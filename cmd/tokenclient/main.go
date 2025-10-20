// Copyright 2025 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// tokenclient is a ZMQ subscriber client for monitoring new token creation events
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	zmq "github.com/pebbe/zmq4"
)

// TokenMetadata represents the metadata of a BEP-20 token
type TokenMetadata struct {
	Address        string       `json:"address"`
	TotalSupply    string       `json:"totalSupply"`
	Name           string       `json:"name,omitempty"`
	Symbol         string       `json:"symbol,omitempty"`
	Decimals       *uint8       `json:"decimals,omitempty"`
	Owner          string       `json:"owner,omitempty"`
	Creator        string       `json:"creator"`
	TxHash         string       `json:"txHash"`
	BlockNumber    uint64       `json:"blockNumber"`
	Timestamp      uint64       `json:"timestamp"`
	HasName        bool         `json:"hasName"`
	HasSymbol      bool         `json:"hasSymbol"`
	HasDecimals    bool         `json:"hasDecimals"`
	HasGetOwner    bool         `json:"hasGetOwner"`
	InitialHolders []HolderInfo `json:"initialHolders,omitempty"`
}

type HolderInfo struct {
	Address string `json:"address"`
	Balance string `json:"balance"`
}

var (
	endpoint   = flag.String("endpoint", "tcp://localhost:5555", "ZMQ endpoint to subscribe to")
	topic      = flag.String("topic", "bsc.token.created", "Topic to subscribe to")
	jsonOutput = flag.Bool("json", false, "Output only JSON (no formatting)")
	verbose    = flag.Bool("v", false, "Verbose output")
)

func main() {
	flag.Parse()

	// Create ZMQ subscriber
	subscriber, err := zmq.NewSocket(zmq.SUB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create ZMQ socket: %v\n", err)
		os.Exit(1)
	}
	defer subscriber.Close()

	// Connect to publisher
	err = subscriber.Connect(*endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to %s: %v\n", *endpoint, err)
		os.Exit(1)
	}

	// Subscribe to topic
	err = subscriber.SetSubscribe(*topic)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe to topic %s: %v\n", *topic, err)
		os.Exit(1)
	}

	if !*jsonOutput {
		fmt.Printf("=================================================\n")
		fmt.Printf("  BSC Token Monitor Client\n")
		fmt.Printf("=================================================\n")
		fmt.Printf("Endpoint: %s\n", *endpoint)
		fmt.Printf("Topic:    %s\n", *topic)
		fmt.Printf("=================================================\n\n")
		fmt.Println("Waiting for token creation events...\n")
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Statistics
	count := uint64(0)
	startTime := time.Now()

	// Main event loop
	for {
		select {
		case <-sigChan:
			if !*jsonOutput {
				fmt.Printf("\n\nShutting down...\n")
				duration := time.Since(startTime)
				fmt.Printf("Total tokens received: %d\n", count)
				fmt.Printf("Duration: %v\n", duration)
				if duration.Seconds() > 0 {
					fmt.Printf("Rate: %.2f tokens/second\n", float64(count)/duration.Seconds())
				}
			}
			return

		default:
			// Receive topic
			_, err := subscriber.RecvBytes(zmq.DONTWAIT)
			if err != nil {
				if zmq.AsErrno(err) == zmq.Errno(syscall.EAGAIN) {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				fmt.Fprintf(os.Stderr, "Error receiving topic: %v\n", err)
				continue
			}

			// Receive data
			data, err := subscriber.RecvBytes(0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error receiving data: %v\n", err)
				continue
			}

			// Parse JSON
			var token TokenMetadata
			err = json.Unmarshal(data, &token)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
				if *verbose {
					fmt.Fprintf(os.Stderr, "Raw data: %s\n", string(data))
				}
				continue
			}

			count++

			if *jsonOutput {
				// Output only JSON
				fmt.Println(string(data))
			} else {
				// Pretty print
				printToken(&token, count)
			}
		}
	}
}

func printToken(token *TokenMetadata, count uint64) {
	timestamp := time.Unix(int64(token.Timestamp), 0)

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Token #%d\n", count)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Address:      %s\n", token.Address)

	if token.HasName && token.Name != "" {
		fmt.Printf("Name:         %s\n", token.Name)
	} else {
		fmt.Printf("Name:         <not available>\n")
	}

	if token.HasSymbol && token.Symbol != "" {
		fmt.Printf("Symbol:       %s\n", token.Symbol)
	} else {
		fmt.Printf("Symbol:       <not available>\n")
	}

	if token.HasDecimals && token.Decimals != nil {
		fmt.Printf("Decimals:     %d\n", *token.Decimals)
	} else {
		fmt.Printf("Decimals:     <not available>\n")
	}

	fmt.Printf("Total Supply: %s\n", token.TotalSupply)
	fmt.Printf("Creator:      %s\n", token.Creator)

	if token.HasGetOwner && token.Owner != "" {
		fmt.Printf("Owner:        %s\n", token.Owner)
	}

	fmt.Printf("Block:        %d\n", token.BlockNumber)
	fmt.Printf("Tx Hash:      %s\n", token.TxHash)
	fmt.Printf("Time:         %s\n", timestamp.Format("2006-01-02 15:04:05"))

	if *verbose {
		fmt.Printf("\nFeatures:\n")
		fmt.Printf("  - name():      %v\n", token.HasName)
		fmt.Printf("  - symbol():    %v\n", token.HasSymbol)
		fmt.Printf("  - decimals():  %v\n", token.HasDecimals)
		fmt.Printf("  - getOwner():  %v\n", token.HasGetOwner)

		if len(token.InitialHolders) > 0 {
			fmt.Printf("\nInitial Holders (%d):\n", len(token.InitialHolders))
			for i, holder := range token.InitialHolders {
				if i >= 5 {
					fmt.Printf("  ... and %d more\n", len(token.InitialHolders)-5)
					break
				}
				fmt.Printf("  %s: %s\n", holder.Address, holder.Balance)
			}
		}
	}

	fmt.Printf("\n")
}
