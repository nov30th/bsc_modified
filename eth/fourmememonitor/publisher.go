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
	"encoding/json"

	"github.com/ethereum/go-ethereum/eth/tokenmonitor"
)

// ZMQPublisherAdapter adapts tokenmonitor.ZMQPublisher to fourmememonitor.Publisher interface
type ZMQPublisherAdapter struct {
	publisher *tokenmonitor.ZMQPublisher
}

// NewZMQPublisherAdapter creates a new ZMQ publisher adapter
func NewZMQPublisherAdapter(publisher *tokenmonitor.ZMQPublisher) *ZMQPublisherAdapter {
	return &ZMQPublisherAdapter{
		publisher: publisher,
	}
}

// Publish publishes Four.meme token metadata to ZMQ
func (p *ZMQPublisherAdapter) Publish(metadata *FourMemeTokenMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	return p.publisher.Publish(FourMemeTokenCreatedTopic, data)
}

// Close closes the ZMQ publisher
func (p *ZMQPublisherAdapter) Close() error {
	return p.publisher.Close()
}
