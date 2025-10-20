// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package tokenmonitor

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// TokenMetadata represents the metadata of a BEP-20 token
type TokenMetadata struct {
	// Basic information
	Address     string `json:"address"`
	TotalSupply string `json:"totalSupply"` // String to avoid big number overflow

	// Optional information (may not be present)
	Name     string `json:"name,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Decimals *uint8 `json:"decimals,omitempty"`
	Owner    string `json:"owner,omitempty"`

	// Deployment information
	Creator     string `json:"creator"`
	TxHash      string `json:"txHash"`
	BlockNumber uint64 `json:"blockNumber"`
	Timestamp   uint64 `json:"timestamp"`

	// Feature flags
	HasName     bool `json:"hasName"`
	HasSymbol   bool `json:"hasSymbol"`
	HasDecimals bool `json:"hasDecimals"`
	HasGetOwner bool `json:"hasGetOwner"`

	// Initial distribution (optional)
	InitialHolders []HolderInfo `json:"initialHolders,omitempty"`
}

// HolderInfo represents a token holder
type HolderInfo struct {
	Address string `json:"address"`
	Balance string `json:"balance"`
}

// PendingToken represents a token contract pending verification
type PendingToken struct {
	Address     common.Address
	BlockNumber uint64
	TxHash      common.Hash
	Creator     common.Address
	Timestamp   uint64
}

// Publisher is the interface for publishing token metadata
type Publisher interface {
	Publish(topic string, data []byte) error
	Close() error
}

// Function selectors (keccak256 hash of function signature, first 4 bytes)
var (
	// ERC-20/BEP-20 standard functions
	TotalSupplySelector = common.Hex2Bytes("18160ddd") // totalSupply()
	BalanceOfSelector   = common.Hex2Bytes("70a08231") // balanceOf(address)
	TransferSelector    = common.Hex2Bytes("a9059cbb") // transfer(address,uint256)
	NameSelector        = common.Hex2Bytes("06fdde03") // name()
	SymbolSelector      = common.Hex2Bytes("95d89b41") // symbol()
	DecimalsSelector    = common.Hex2Bytes("313ce567") // decimals()

	// BEP-20 specific
	GetOwnerSelector = common.Hex2Bytes("893d20e8") // getOwner()

	// Transfer event signature: keccak256("Transfer(address,address,uint256)")
	TransferEventSignature = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
)

// Helper function to convert big.Int to string safely
func BigIntToString(b *big.Int) string {
	if b == nil {
		return "0"
	}
	return b.String()
}
