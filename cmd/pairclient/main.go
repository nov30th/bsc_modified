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

// pairclient is a ZMQ subscriber client for monitoring new PancakeSwap pair creation events
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

// PairMetadata represents the metadata of a PancakeSwap trading pair
type PairMetadata struct {
	PairAddress    string `json:"pairAddress"`
	Token0         string `json:"token0"`
	Token1         string `json:"token1"`
	PairIndex      uint64 `json:"pairIndex"`
	Token0Name     string `json:"token0Name,omitempty"`
	Token0Symbol   string `json:"token0Symbol,omitempty"`
	Token0Decimals *uint8 `json:"token0Decimals,omitempty"`
	Token1Name     string `json:"token1Name,omitempty"`
	Token1Symbol   string `json:"token1Symbol,omitempty"`
	Token1Decimals *uint8 `json:"token1Decimals,omitempty"`
	Reserve0       string `json:"reserve0,omitempty"`
	Reserve1       string `json:"reserve1,omitempty"`
	TotalSupply    string `json:"totalSupply,omitempty"`
	InitialPrice   string `json:"initialPrice,omitempty"`
	PairName       string `json:"pairName,omitempty"`
	PairSymbol     string `json:"pairSymbol,omitempty"`
	Creator        string `json:"creator"`
	TxHash         string `json:"txHash"`
	BlockNumber    uint64 `json:"blockNumber"`
	Timestamp      uint64 `json:"timestamp"`
	FactoryAddress string `json:"factoryAddress"`
}

var (
	endpoint   = flag.String("endpoint", "tcp://localhost:5556", "ZMQ endpoint to subscribe to")
	topic      = flag.String("topic", "bsc.pair.created", "Topic to subscribe to")
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
		fmt.Printf("  BSC PancakeSwap Pair Monitor Client\n")
		fmt.Printf("=================================================\n")
		fmt.Printf("Endpoint: %s\n", *endpoint)
		fmt.Printf("Topic:    %s\n", *topic)
		fmt.Printf("=================================================\n\n")
		fmt.Println("Waiting for pair creation events...\n")
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
				fmt.Printf("Total pairs received: %d\n", count)
				fmt.Printf("Duration: %v\n", duration)
				if duration.Seconds() > 0 {
					fmt.Printf("Rate: %.2f pairs/second\n", float64(count)/duration.Seconds())
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
			var pair PairMetadata
			err = json.Unmarshal(data, &pair)
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
				printPair(&pair, count)
			}
		}
	}
}

func printPair(pair *PairMetadata, count uint64) {
	timestamp := time.Unix(int64(pair.Timestamp), 0)

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Pair #%d\n", count)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Pair Address: %s\n", pair.PairAddress)
	fmt.Printf("Factory:      %s\n", pair.FactoryAddress)
	fmt.Printf("Pair Index:   %d\n", pair.PairIndex)
	fmt.Printf("\n")

	// Token 0
	fmt.Printf("Token0:       %s\n", pair.Token0)
	if pair.Token0Symbol != "" {
		fmt.Printf("  Symbol:     %s\n", pair.Token0Symbol)
	}
	if pair.Token0Name != "" {
		fmt.Printf("  Name:       %s\n", pair.Token0Name)
	}
	if pair.Token0Decimals != nil {
		fmt.Printf("  Decimals:   %d\n", *pair.Token0Decimals)
	}
	if pair.Reserve0 != "" {
		fmt.Printf("  Reserve:    %s\n", pair.Reserve0)
	}
	fmt.Printf("\n")

	// Token 1
	fmt.Printf("Token1:       %s\n", pair.Token1)
	if pair.Token1Symbol != "" {
		fmt.Printf("  Symbol:     %s\n", pair.Token1Symbol)
	}
	if pair.Token1Name != "" {
		fmt.Printf("  Name:       %s\n", pair.Token1Name)
	}
	if pair.Token1Decimals != nil {
		fmt.Printf("  Decimals:   %d\n", *pair.Token1Decimals)
	}
	if pair.Reserve1 != "" {
		fmt.Printf("  Reserve:    %s\n", pair.Reserve1)
	}
	fmt.Printf("\n")

	// Pair info
	if pair.PairName != "" {
		fmt.Printf("Pair Name:    %s\n", pair.PairName)
	}
	if pair.PairSymbol != "" {
		fmt.Printf("Pair Symbol:  %s\n", pair.PairSymbol)
	}
	if pair.TotalSupply != "" {
		fmt.Printf("LP Supply:    %s\n", pair.TotalSupply)
	}
	if pair.InitialPrice != "" {
		fmt.Printf("Price:        %s %s/%s\n", pair.InitialPrice, pair.Token1Symbol, pair.Token0Symbol)
	}
	fmt.Printf("\n")

	fmt.Printf("Creator:      %s\n", pair.Creator)
	fmt.Printf("Block:        %d\n", pair.BlockNumber)
	fmt.Printf("Tx Hash:      %s\n", pair.TxHash)
	fmt.Printf("Time:         %s\n", timestamp.Format("2006-01-02 15:04:05"))

	fmt.Printf("\n")
}
