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
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Four.meme Factory address on BSC
var FourMemeFactory = common.HexToAddress("0x5c952063c7fc8610FFDB798152D69F0B9550762b")

// Topic for Four.meme token creation events
const FourMemeTokenCreatedTopic = "bsc.fourmeme.token.created"

// ERC-20 function selectors
var (
	TotalSupplySelector = common.Hex2Bytes("18160ddd") // totalSupply()
	NameSelector        = common.Hex2Bytes("06fdde03") // name()
	SymbolSelector      = common.Hex2Bytes("95d89b41") // symbol()
	DecimalsSelector    = common.Hex2Bytes("313ce567") // decimals()
)

// FourMemeTokenMetadata represents the complete metadata of a Four.meme token
type FourMemeTokenMetadata struct {
	// Basic Information
	Address        string `json:"address"`        // Token contract address
	Creator        string `json:"creator"`        // User who created the token
	TxHash         string `json:"txHash"`         // Creation transaction hash
	BlockNumber    uint64 `json:"blockNumber"`    // Block number
	Timestamp      uint64 `json:"timestamp"`      // Block timestamp
	FactoryAddress string `json:"factoryAddress"` // Four.meme Factory address

	// Token Metadata (from ERC-20 calls)
	Name        string `json:"name,omitempty"`        // Token name
	Symbol      string `json:"symbol,omitempty"`      // Token symbol
	Decimals    *uint8 `json:"decimals,omitempty"`    // Token decimals
	TotalSupply string `json:"totalSupply"`           // Total supply in wei
	InitialSupply string `json:"initialSupply"`       // Initial minted supply

	// Four.meme Specific
	Platform string `json:"platform"` // "four.meme"

	// Metadata flags
	HasName     bool `json:"hasName"`     // Whether name() is implemented
	HasSymbol   bool `json:"hasSymbol"`   // Whether symbol() is implemented
	HasDecimals bool `json:"hasDecimals"` // Whether decimals() is implemented
}

// PendingFourMemeToken represents a token waiting for verification
type PendingFourMemeToken struct {
	TokenAddress   common.Address
	Creator        common.Address
	InitialSupply  *big.Int
	BlockNumber    uint64
	TxHash         common.Hash
	Timestamp      uint64
	FactoryAddress common.Address
}

// Publisher interface for publishing verified token metadata
type Publisher interface {
	Publish(metadata *FourMemeTokenMetadata) error
	Close() error
}
