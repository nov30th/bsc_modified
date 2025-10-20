// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package tokenmonitor

import (
	"fmt"
	"sync"

	zmq "github.com/pebbe/zmq4"

	"github.com/ethereum/go-ethereum/log"
)

// ZMQPublisher implements the Publisher interface using ZeroMQ
type ZMQPublisher struct {
	socket   *zmq.Socket
	endpoint string
	mu       sync.Mutex
	closed   bool
}

// NewZMQPublisher creates a new ZMQ publisher
func NewZMQPublisher(endpoint string) (*ZMQPublisher, error) {
	socket, err := zmq.NewSocket(zmq.PUB)
	if err != nil {
		return nil, fmt.Errorf("failed to create ZMQ socket: %w", err)
	}

	// Set socket options
	socket.SetLinger(0)                // Don't wait for unsent messages on close
	socket.SetSndhwm(1000)             // Send high water mark (max queued messages)
	socket.SetSndtimeo(100)            // Send timeout 100ms
	socket.SetIpv6(false)              // Disable IPv6

	err = socket.Bind(endpoint)
	if err != nil {
		socket.Close()
		return nil, fmt.Errorf("failed to bind ZMQ socket to %s: %w", endpoint, err)
	}

	log.Info("ZMQ publisher started", "endpoint", endpoint)

	return &ZMQPublisher{
		socket:   socket,
		endpoint: endpoint,
		closed:   false,
	}, nil
}

// Publish publishes a message to the specified topic
func (p *ZMQPublisher) Publish(topic string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("publisher is closed")
	}

	// ZMQ multipart message: [topic][data]
	_, err := p.socket.SendBytes([]byte(topic), zmq.SNDMORE)
	if err != nil {
		return fmt.Errorf("failed to send topic: %w", err)
	}

	_, err = p.socket.SendBytes(data, 0)
	if err != nil {
		return fmt.Errorf("failed to send data: %w", err)
	}

	return nil
}

// Close closes the publisher
func (p *ZMQPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	err := p.socket.Close()
	if err != nil {
		return fmt.Errorf("failed to close ZMQ socket: %w", err)
	}

	log.Info("ZMQ publisher stopped", "endpoint", p.endpoint)
	return nil
}

// IsClosed returns whether the publisher is closed
func (p *ZMQPublisher) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}
