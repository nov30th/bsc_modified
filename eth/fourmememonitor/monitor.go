// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package fourmememonitor

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
)

const (
	// VerificationDelay is the delay before verifying a token to ensure state is committed
	VerificationDelay = 500 * time.Millisecond
)

// FourMemeMonitor monitors and publishes Four.meme token creation events
type FourMemeMonitor struct {
	blockchain *core.BlockChain
	publisher  Publisher
	verifier   *Verifier

	eventChan chan core.NewFourMemeTokenCreatedEvent
	eventSub  event.Subscription

	verifyQueue   chan *PendingFourMemeToken
	publishQueue  chan *FourMemeTokenMetadata
	wg            sync.WaitGroup
	quit          chan struct{}
	workers       int

	// Statistics
	tokensDetected  atomic.Uint64
	tokensVerified  atomic.Uint64
	tokensPublished atomic.Uint64
	tokensFailed    atomic.Uint64
}

// NewFourMemeMonitor creates a new Four.meme token monitor
func NewFourMemeMonitor(blockchain *core.BlockChain, publisher Publisher) *FourMemeMonitor {
	workers := runtime.NumCPU() - 1
	if workers < 1 {
		workers = 1
	}

	return &FourMemeMonitor{
		blockchain:   blockchain,
		publisher:    publisher,
		verifier:     NewVerifier(blockchain),
		eventChan:    make(chan core.NewFourMemeTokenCreatedEvent, 100),
		verifyQueue:  make(chan *PendingFourMemeToken, 1000),
		publishQueue: make(chan *FourMemeTokenMetadata, 1000),
		quit:         make(chan struct{}),
		workers:      workers,
	}
}

// Start starts the Four.meme token monitor
func (m *FourMemeMonitor) Start() error {
	log.Info("Starting Four.meme token monitor", "workers", m.workers, "cpus", runtime.NumCPU())

	// Subscribe to Four.meme token creation events
	m.eventSub = m.blockchain.SubscribeFourMemeTokenCreatedEvent(m.eventChan)

	// Start event loop
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

	return nil
}

// Stop stops the Four.meme token monitor
func (m *FourMemeMonitor) Stop() {
	log.Info("Stopping Four.meme token monitor...")

	if m.eventSub != nil {
		m.eventSub.Unsubscribe()
	}

	close(m.quit)
	m.wg.Wait()

	log.Info("Four.meme token monitor stopped",
		"detected", m.tokensDetected.Load(),
		"verified", m.tokensVerified.Load(),
		"published", m.tokensPublished.Load(),
		"failed", m.tokensFailed.Load())
}

// eventLoop receives Four.meme token creation events and queues them for verification
func (m *FourMemeMonitor) eventLoop() {
	defer m.wg.Done()

	for {
		select {
		case event := <-m.eventChan:
			m.tokensDetected.Add(1)

			pending := &PendingFourMemeToken{
				TokenAddress:   event.TokenAddress,
				Creator:        event.Creator,
				InitialSupply:  event.InitialSupply,
				BlockNumber:    event.BlockNumber,
				TxHash:         event.TxHash,
				Timestamp:      event.Timestamp,
				FactoryAddress: event.FactoryAddress,
			}

			select {
			case m.verifyQueue <- pending:
			case <-m.quit:
				return
			}

		case <-m.quit:
			return
		}
	}
}

// verifyWorker verifies pending tokens and queues them for publishing
func (m *FourMemeMonitor) verifyWorker(id int) {
	defer m.wg.Done()

	for {
		select {
		case token := <-m.verifyQueue:
			// Wait for state to be committed
			time.Sleep(VerificationDelay)

			// Verify and extract metadata
			metadata, err := m.verifier.VerifyAndExtract(token)
			if err != nil {
				m.tokensFailed.Add(1)
				log.Debug("Failed to verify Four.meme token",
					"address", token.TokenAddress.Hex(),
					"error", err)
				continue
			}

			m.tokensVerified.Add(1)

			// Queue for publishing
			select {
			case m.publishQueue <- metadata:
			case <-m.quit:
				return
			}

		case <-m.quit:
			return
		}
	}
}

// publishWorker publishes verified token metadata to ZMQ
func (m *FourMemeMonitor) publishWorker() {
	defer m.wg.Done()

	for {
		select {
		case metadata := <-m.publishQueue:
			if err := m.publisher.Publish(metadata); err != nil {
				log.Error("Failed to publish Four.meme token metadata",
					"address", metadata.Address,
					"error", err)
				continue
			}

			m.tokensPublished.Add(1)
			log.Info("Four.meme token published",
				"address", metadata.Address,
				"symbol", metadata.Symbol,
				"name", metadata.Name,
				"creator", metadata.Creator)

		case <-m.quit:
			return
		}
	}
}

// statsReporter periodically reports statistics
func (m *FourMemeMonitor) statsReporter() {
	defer m.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Info("Four.meme token monitor stats",
				"detected", m.tokensDetected.Load(),
				"verified", m.tokensVerified.Load(),
				"published", m.tokensPublished.Load(),
				"failed", m.tokensFailed.Load())

		case <-m.quit:
			return
		}
	}
}
