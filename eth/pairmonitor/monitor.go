// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pairmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
)

const (
	// Topic for pair created events
	PairCreatedTopic = "bsc.pair.created"

	// Buffer sizes
	EventChannelSize   = 100
	VerifyQueueSize    = 1000
	PublishQueueSize   = 100
	PublishBatchSize   = 10
	PublishBatchTimeout = 1 * time.Second

	// Verification delay - wait for state to be committed
	VerificationDelay = 500 * time.Millisecond
)

// PairMonitor monitors blockchain for new trading pairs
type PairMonitor struct {
	blockchain   *core.BlockChain
	publisher    Publisher
	verifier     *Verifier

	eventCh      chan core.NewPairCreatedEvent
	eventSub     event.Subscription

	verifyQueue  chan *PendingPair
	publishQueue chan *PairMetadata

	workers      int
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc

	// Statistics
	stats struct {
		sync.RWMutex
		detected     uint64
		verified     uint64
		published    uint64
		failed       uint64
		lastErrors   []string  // Keep last 5 error messages
		lastFailedAt time.Time // Last failure timestamp
	}
}

// NewPairMonitor creates a new pair monitor
func NewPairMonitor(blockchain *core.BlockChain, publisher Publisher) *PairMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	// Calculate worker count: CPU cores - 1, minimum 1
	workers := runtime.NumCPU() - 1
	if workers < 1 {
		workers = 1
	}

	return &PairMonitor{
		blockchain:   blockchain,
		publisher:    publisher,
		verifier:     NewVerifier(blockchain),
		eventCh:      make(chan core.NewPairCreatedEvent, EventChannelSize),
		verifyQueue:  make(chan *PendingPair, VerifyQueueSize),
		publishQueue: make(chan *PairMetadata, PublishQueueSize),
		workers:      workers,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the pair monitor
func (m *PairMonitor) Start() error {
	// Subscribe to pair created events
	m.eventSub = m.blockchain.SubscribePairCreatedEvent(m.eventCh)

	// Start event receiver
	m.wg.Add(1)
	go m.eventLoop()

	// Start verify workers
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.verifyWorker(i)
	}

	// Start publish worker
	m.wg.Add(1)
	go m.publishWorker()

	// Start stats reporter
	m.wg.Add(1)
	go m.statsReporter()

	log.Info("Pair monitor started", "workers", m.workers, "cpus", runtime.NumCPU())
	return nil
}

// Stop stops the pair monitor
func (m *PairMonitor) Stop() {
	log.Info("Stopping pair monitor...")
	m.cancel()
	m.eventSub.Unsubscribe()
	m.wg.Wait()

	// Print final stats
	m.stats.RLock()
	log.Info("Pair monitor stopped",
		"detected", m.stats.detected,
		"verified", m.stats.verified,
		"published", m.stats.published,
		"failed", m.stats.failed)
	m.stats.RUnlock()
}

// eventLoop receives pair creation events
func (m *PairMonitor) eventLoop() {
	defer m.wg.Done()

	for {
		select {
		case event := <-m.eventCh:
			// Convert to pending pair
			pending := &PendingPair{
				PairAddress:    event.PairAddress,
				Token0:         event.Token0,
				Token1:         event.Token1,
				PairIndex:      event.PairIndex,
				FactoryAddress: event.FactoryAddress,
				BlockNumber:    event.BlockNumber,
				TxHash:         event.TxHash,
				Creator:        event.Creator,
				Timestamp:      event.Timestamp,
			}

			// Update stats
			m.stats.Lock()
			m.stats.detected++
			m.stats.Unlock()

			// Add to verify queue
			select {
			case m.verifyQueue <- pending:
				log.Debug("Pair queued for verification",
					"pair", event.PairAddress.Hex(),
					"token0", event.Token0.Hex(),
					"token1", event.Token1.Hex(),
					"block", event.BlockNumber)
			default:
				log.Warn("Verify queue full, dropping pair",
					"pair", event.PairAddress.Hex())
				m.stats.Lock()
				m.stats.failed++
				m.stats.Unlock()
			}

		case err := <-m.eventSub.Err():
			log.Error("Pair event subscription error", "err", err)
			return

		case <-m.ctx.Done():
			return
		}
	}
}

// verifyWorker verifies pair contracts and extracts metadata
func (m *PairMonitor) verifyWorker(id int) {
	defer m.wg.Done()

	for {
		select {
		case pair := <-m.verifyQueue:
			// Wait for state to be committed (important for newly created pairs)
			time.Sleep(VerificationDelay)

			// Verify and extract metadata
			metadata, err := m.verifier.VerifyAndExtract(pair)
			if err != nil {
				// Record error for debugging (keep last 5 errors)
				errMsg := fmt.Sprintf("block=%d pair=%s err=%v", pair.BlockNumber, pair.PairAddress.Hex(), err)

				m.stats.Lock()
				m.stats.failed++
				m.stats.lastFailedAt = time.Now()

				// Keep only last 5 errors
				m.stats.lastErrors = append(m.stats.lastErrors, errMsg)
				if len(m.stats.lastErrors) > 5 {
					m.stats.lastErrors = m.stats.lastErrors[1:]
				}
				m.stats.Unlock()

				// Only log every 100th failure to reduce noise
				if m.stats.failed % 100 == 0 {
					log.Debug("Pair verification failed (sample)", "worker", id, "total_failed", m.stats.failed, "err", err)
				}
				continue
			}

			// Update stats
			m.stats.Lock()
			m.stats.verified++
			m.stats.Unlock()

			log.Info("Pair verified",
				"worker", id,
				"pair", metadata.PairAddress,
				"token0", metadata.Token0Symbol,
				"token1", metadata.Token1Symbol,
				"factory", metadata.FactoryAddress)

			// Add to publish queue
			select {
			case m.publishQueue <- metadata:
			default:
				log.Warn("Publish queue full", "worker", id)
				m.stats.Lock()
				m.stats.failed++
				m.stats.Unlock()
			}

		case <-m.ctx.Done():
			return
		}
	}
}

// publishWorker publishes verified pairs to ZMQ
func (m *PairMonitor) publishWorker() {
	defer m.wg.Done()

	batch := make([]*PairMetadata, 0, PublishBatchSize)
	ticker := time.NewTicker(PublishBatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case metadata := <-m.publishQueue:
			batch = append(batch, metadata)

			// Publish batch when full
			if len(batch) >= PublishBatchSize {
				m.publishBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Publish batch on timeout
			if len(batch) > 0 {
				m.publishBatch(batch)
				batch = batch[:0]
			}

		case <-m.ctx.Done():
			// Publish remaining on shutdown
			if len(batch) > 0 {
				m.publishBatch(batch)
			}
			return
		}
	}
}

// publishBatch publishes a batch of pair metadata
func (m *PairMonitor) publishBatch(batch []*PairMetadata) {
	for _, metadata := range batch {
		// Serialize to JSON
		data, err := json.Marshal(metadata)
		if err != nil {
			log.Error("Failed to marshal pair metadata", "err", err)
			m.stats.Lock()
			m.stats.failed++
			m.stats.Unlock()
			continue
		}

		// Publish to ZMQ
		err = m.publisher.Publish(PairCreatedTopic, data)
		if err != nil {
			log.Error("Failed to publish pair", "err", err)
			m.stats.Lock()
			m.stats.failed++
			m.stats.Unlock()
			continue
		}

		// Update stats
		m.stats.Lock()
		m.stats.published++
		m.stats.Unlock()

		log.Debug("Pair published",
			"pair", metadata.PairAddress,
			"token0", metadata.Token0Symbol,
			"token1", metadata.Token1Symbol)
	}
}

// statsReporter periodically reports statistics
func (m *PairMonitor) statsReporter() {
	defer m.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.stats.RLock()
			log.Info("Pair monitor stats",
				"detected", m.stats.detected,
				"verified", m.stats.verified,
				"published", m.stats.published,
				"failed", m.stats.failed,
				"verify_queue", len(m.verifyQueue),
				"publish_queue", len(m.publishQueue))

			// Log last few errors if there are failures
			if m.stats.failed > 0 && len(m.stats.lastErrors) > 0 {
				log.Info("Recent pair verification failures (last 5):")
				for i, errMsg := range m.stats.lastErrors {
					log.Info(fmt.Sprintf("  [%d] %s", i+1, errMsg))
				}
			}
			m.stats.RUnlock()

		case <-m.ctx.Done():
			return
		}
	}
}

// GetStats returns current statistics
func (m *PairMonitor) GetStats() map[string]uint64 {
	m.stats.RLock()
	defer m.stats.RUnlock()

	return map[string]uint64{
		"detected":  m.stats.detected,
		"verified":  m.stats.verified,
		"published": m.stats.published,
		"failed":    m.stats.failed,
	}
}
