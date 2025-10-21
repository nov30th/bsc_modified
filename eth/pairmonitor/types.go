// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pairmonitor

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// PairMetadata represents the metadata of a PancakeSwap trading pair
type PairMetadata struct {
	// Pair information
	PairAddress  string `json:"pairAddress"`
	Token0       string `json:"token0"`
	Token1       string `json:"token1"`
	PairIndex    uint64 `json:"pairIndex"`

	// Token0 metadata
	Token0Name     string `json:"token0Name,omitempty"`
	Token0Symbol   string `json:"token0Symbol,omitempty"`
	Token0Decimals *uint8 `json:"token0Decimals,omitempty"`

	// Token1 metadata
	Token1Name     string `json:"token1Name,omitempty"`
	Token1Symbol   string `json:"token1Symbol,omitempty"`
	Token1Decimals *uint8 `json:"token1Decimals,omitempty"`

	// Liquidity information
	Reserve0       string `json:"reserve0,omitempty"`        // Token0 reserve amount
	Reserve1       string `json:"reserve1,omitempty"`        // Token1 reserve amount
	TotalSupply    string `json:"totalSupply,omitempty"`     // LP token total supply
	InitialPrice   string `json:"initialPrice,omitempty"`    // Token1/Token0 price

	// Pair token metadata
	PairName       string `json:"pairName,omitempty"`        // LP token name
	PairSymbol     string `json:"pairSymbol,omitempty"`      // LP token symbol

	// Creation information
	Creator     string `json:"creator"`
	TxHash      string `json:"txHash"`
	BlockNumber uint64 `json:"blockNumber"`
	Timestamp   uint64 `json:"timestamp"`

	// Factory information
	FactoryAddress string `json:"factoryAddress"` // Which factory created this pair
}

// PendingPair represents a pair pending verification
type PendingPair struct {
	PairAddress    common.Address
	Token0         common.Address
	Token1         common.Address
	PairIndex      *big.Int
	FactoryAddress common.Address
	BlockNumber    uint64
	TxHash         common.Hash
	Creator        common.Address
	Timestamp      uint64
}

// Publisher is the interface for publishing pair metadata
type Publisher interface {
	Publish(topic string, data []byte) error
	Close() error
}

// PancakeSwap Factory addresses on BSC
var (
	// PancakeSwap V2 Factory
	PancakeV2Factory = common.HexToAddress("0xcA143Ce32Fe78f1f7019d7d551a6402fC5350c73")

	// PancakeSwap V3 Factory (optional, can add later)
	// PancakeV3Factory = common.HexToAddress("0x0BFbCF9fa4f9C56B0F40a671Ad40E0805A091865")
)

// PairCreated event signature: keccak256("PairCreated(address,address,address,uint256)")
var PairCreatedEventSignature = common.HexToHash("0x0d3648bd0f6ba80134a33ba9275ac585d9d315f0ad8355cddefde31afa28d0e9")

// Function selectors for Pair contract
var (
	// Pair contract functions
	GetReservesSelector  = common.Hex2Bytes("0902f1ac") // getReserves()
	Token0Selector       = common.Hex2Bytes("0dfe1681") // token0()
	Token1Selector       = common.Hex2Bytes("d21220a7") // token1()

	// ERC20 standard functions (reuse from token monitoring)
	TotalSupplySelector = common.Hex2Bytes("18160ddd") // totalSupply()
	NameSelector        = common.Hex2Bytes("06fdde03") // name()
	SymbolSelector      = common.Hex2Bytes("95d89b41") // symbol()
	DecimalsSelector    = common.Hex2Bytes("313ce567") // decimals()
)

// Helper function to convert big.Int to string safely
func BigIntToString(b *big.Int) string {
	if b == nil {
		return "0"
	}
	return b.String()
}

// Helper function to calculate price (reserve1/reserve0)
func CalculatePrice(reserve0, reserve1 *big.Int, decimals0, decimals1 uint8) string {
	if reserve0 == nil || reserve1 == nil || reserve0.Cmp(big.NewInt(0)) == 0 {
		return "0"
	}

	// Adjust for decimals difference
	decimalsDiff := int(decimals1) - int(decimals0)

	price := new(big.Float).SetInt(reserve1)
	divisor := new(big.Float).SetInt(reserve0)

	// Apply decimal adjustment
	if decimalsDiff != 0 {
		adjustment := new(big.Float).SetInt(new(big.Int).Exp(
			big.NewInt(10),
			big.NewInt(int64(decimalsDiff)),
			nil,
		))
		if decimalsDiff > 0 {
			price.Quo(price, adjustment)
		} else {
			price.Mul(price, adjustment)
		}
	}

	result := new(big.Float).Quo(price, divisor)
	return result.Text('f', 18)
}
