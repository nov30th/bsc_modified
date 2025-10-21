// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package tokenmonitor

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
	// Topic for new token events
	TokenCreatedTopic = "bsc.token.created"

	// Buffer sizes
	EventChannelSize   = 100
	VerifyQueueSize    = 1000
	PublishQueueSize   = 100
	PublishBatchSize   = 10
	PublishBatchTimeout = 1 * time.Second

	// Verification delay - wait for state to be committed
	VerificationDelay = 500 * time.Millisecond
)

// TokenMonitor monitors blockchain for new token contracts
type TokenMonitor struct {
	blockchain   *core.BlockChain
	publisher    Publisher
	verifier     *Verifier

	eventCh      chan core.NewTokenCreatedEvent
	eventSub     event.Subscription

	verifyQueue  chan *PendingToken
	publishQueue chan *TokenMetadata

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

// NewTokenMonitor creates a new token monitor
func NewTokenMonitor(blockchain *core.BlockChain, publisher Publisher) *TokenMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	// Calculate worker count: CPU cores - 1, minimum 1
	workers := runtime.NumCPU() - 1
	if workers < 1 {
		workers = 1
	}

	return &TokenMonitor{
		blockchain:   blockchain,
		publisher:    publisher,
		verifier:     NewVerifier(blockchain),
		eventCh:      make(chan core.NewTokenCreatedEvent, EventChannelSize),
		verifyQueue:  make(chan *PendingToken, VerifyQueueSize),
		publishQueue: make(chan *TokenMetadata, PublishQueueSize),
		workers:      workers,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the token monitor
func (m *TokenMonitor) Start() error {
	// Subscribe to token created events
	m.eventSub = m.blockchain.SubscribeTokenCreatedEvent(m.eventCh)

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

	log.Info("Token monitor started", "workers", m.workers, "cpus", runtime.NumCPU())
	return nil
}

// Stop stops the token monitor
func (m *TokenMonitor) Stop() {
	log.Info("Stopping token monitor...")
	m.cancel()
	m.eventSub.Unsubscribe()
	m.wg.Wait()

	// Print final stats
	m.stats.RLock()
	log.Info("Token monitor stopped",
		"detected", m.stats.detected,
		"verified", m.stats.verified,
		"published", m.stats.published,
		"failed", m.stats.failed)
	m.stats.RUnlock()
}

// eventLoop receives token creation events
func (m *TokenMonitor) eventLoop() {
	defer m.wg.Done()

	for {
		select {
		case event := <-m.eventCh:
			// Convert to pending token
			pending := &PendingToken{
				Address:     event.ContractAddress,
				BlockNumber: event.BlockNumber,
				TxHash:      event.TxHash,
				Creator:     event.Creator,
				Timestamp:   event.Timestamp,
			}

			// Update stats
			m.stats.Lock()
			m.stats.detected++
			m.stats.Unlock()

			// Add to verify queue
			select {
			case m.verifyQueue <- pending:
				log.Debug("Token queued for verification",
					"address", event.ContractAddress.Hex(),
					"block", event.BlockNumber)
			default:
				log.Warn("Verify queue full, dropping token",
					"address", event.ContractAddress.Hex())
				m.stats.Lock()
				m.stats.failed++
				m.stats.Unlock()
			}

		case err := <-m.eventSub.Err():
			log.Error("Token event subscription error", "err", err)
			return

		case <-m.ctx.Done():
			return
		}
	}
}

// verifyWorker verifies token contracts and extracts metadata
func (m *TokenMonitor) verifyWorker(id int) {
	defer m.wg.Done()

	for {
		select {
		case token := <-m.verifyQueue:
			// Wait for state to be committed (important for newly created contracts)
			time.Sleep(VerificationDelay)

			// Verify and extract metadata
			metadata, err := m.verifier.VerifyAndExtract(token)
			if err != nil {
				// Record error for debugging (keep last 5 errors)
				errMsg := fmt.Sprintf("block=%d addr=%s err=%v", token.BlockNumber, token.Address.Hex(), err)

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
					log.Debug("Token verification failed (sample)", "worker", id, "total_failed", m.stats.failed, "err", err)
				}
				continue
			}

			// Update stats
			m.stats.Lock()
			m.stats.verified++
			m.stats.Unlock()

			log.Info("Token verified",
				"worker", id,
				"address", metadata.Address,
				"symbol", metadata.Symbol,
				"name", metadata.Name,
				"decimals", metadata.Decimals)

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

// publishWorker publishes verified tokens to ZMQ
func (m *TokenMonitor) publishWorker() {
	defer m.wg.Done()

	batch := make([]*TokenMetadata, 0, PublishBatchSize)
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

// publishBatch publishes a batch of token metadata
func (m *TokenMonitor) publishBatch(batch []*TokenMetadata) {
	for _, metadata := range batch {
		// Serialize to JSON
		data, err := json.Marshal(metadata)
		if err != nil {
			log.Error("Failed to marshal token metadata", "err", err)
			m.stats.Lock()
			m.stats.failed++
			m.stats.Unlock()
			continue
		}

		// Publish to ZMQ
		err = m.publisher.Publish(TokenCreatedTopic, data)
		if err != nil {
			log.Error("Failed to publish token", "err", err)
			m.stats.Lock()
			m.stats.failed++
			m.stats.Unlock()
			continue
		}

		// Update stats
		m.stats.Lock()
		m.stats.published++
		m.stats.Unlock()

		log.Debug("Token published",
			"address", metadata.Address,
			"symbol", metadata.Symbol)
	}
}

// statsReporter periodically reports statistics
func (m *TokenMonitor) statsReporter() {
	defer m.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.stats.RLock()
			log.Info("Token monitor stats",
				"detected", m.stats.detected,
				"verified", m.stats.verified,
				"published", m.stats.published,
				"failed", m.stats.failed,
				"verify_queue", len(m.verifyQueue),
				"publish_queue", len(m.publishQueue))

			// Log last few errors if there are failures
			if m.stats.failed > 0 && len(m.stats.lastErrors) > 0 {
				log.Info("Recent verification failures (last 5):")
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
func (m *TokenMonitor) GetStats() map[string]uint64 {
	m.stats.RLock()
	defer m.stats.RUnlock()

	return map[string]uint64{
		"detected":  m.stats.detected,
		"verified":  m.stats.verified,
		"published": m.stats.published,
		"failed":    m.stats.failed,
	}
}
