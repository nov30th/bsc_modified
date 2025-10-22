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
	"fmt"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/contractutils"
)

// Verifier verifies Four.meme tokens and extracts their metadata
type Verifier struct {
	caller *contractutils.ContractCaller
}

// NewVerifier creates a new Four.meme token verifier
func NewVerifier(blockchain *core.BlockChain) *Verifier {
	return &Verifier{
		caller: contractutils.NewContractCaller(blockchain),
	}
}

// VerifyAndExtract verifies a Four.meme token contract and extracts its metadata
func (v *Verifier) VerifyAndExtract(token *PendingFourMemeToken) (*FourMemeTokenMetadata, error) {
	// Get current state
	statedb, err := v.caller.GetState()
	if err != nil {
		return nil, fmt.Errorf("failed to get state: %w", err)
	}

	// Check if contract exists
	if !v.caller.HasCode(token.TokenAddress, statedb) {
		return nil, fmt.Errorf("token contract has no code")
	}

	// Extract token metadata
	metadata := &FourMemeTokenMetadata{
		Address:        token.TokenAddress.Hex(),
		Creator:        token.Creator.Hex(),
		TxHash:         token.TxHash.Hex(),
		BlockNumber:    token.BlockNumber,
		Timestamp:      token.Timestamp,
		FactoryAddress: token.FactoryAddress.Hex(),
		InitialSupply:  token.InitialSupply.String(),
		Platform:       "four.meme",
	}

	// Try to get name
	if name, err := v.caller.CallString(token.TokenAddress, NameSelector, statedb); err == nil && name != "" {
		metadata.Name = name
		metadata.HasName = true
	}

	// Try to get symbol
	if symbol, err := v.caller.CallString(token.TokenAddress, SymbolSelector, statedb); err == nil && symbol != "" {
		metadata.Symbol = symbol
		metadata.HasSymbol = true
	}

	// Try to get decimals
	if decimals, err := v.caller.CallUint8(token.TokenAddress, DecimalsSelector, statedb); err == nil {
		metadata.Decimals = &decimals
		metadata.HasDecimals = true
	}

	// Get total supply (required for ERC-20)
	totalSupply, err := v.caller.CallUint256(token.TokenAddress, TotalSupplySelector, statedb)
	if err != nil {
		return nil, fmt.Errorf("failed to get totalSupply: %w", err)
	}
	metadata.TotalSupply = totalSupply.String()

	return metadata, nil
}
