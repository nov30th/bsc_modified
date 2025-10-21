// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pairmonitor

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/eth/contractutils"
	"github.com/ethereum/go-ethereum/log"
)

// Verifier handles pair contract verification
type Verifier struct {
	caller *contractutils.ContractCaller
}

// NewVerifier creates a new pair verifier
func NewVerifier(blockchain *core.BlockChain) *Verifier {
	return &Verifier{
		caller: contractutils.NewContractCaller(blockchain),
	}
}

// VerifyAndExtract verifies a pair contract and extracts metadata
func (v *Verifier) VerifyAndExtract(pair *PendingPair) (*PairMetadata, error) {
	// Get current state
	statedb, err := v.caller.GetState()
	if err != nil {
		return nil, fmt.Errorf("failed to get state: %w", err)
	}

	// Check if pair contract has code
	if !v.caller.HasCode(pair.PairAddress, statedb) {
		return nil, fmt.Errorf("pair contract has no code in current state")
	}

	metadata := &PairMetadata{
		PairAddress:    pair.PairAddress.Hex(),
		Token0:         pair.Token0.Hex(),
		Token1:         pair.Token1.Hex(),
		PairIndex:      pair.PairIndex.Uint64(),
		Creator:        pair.Creator.Hex(),
		TxHash:         pair.TxHash.Hex(),
		BlockNumber:    pair.BlockNumber,
		Timestamp:      pair.Timestamp,
		FactoryAddress: pair.FactoryAddress.Hex(),
	}

	// Get reserves from pair contract
	reserves, err := v.getReserves(pair.PairAddress, statedb)
	if err != nil {
		log.Debug("Failed to get reserves", "pair", pair.PairAddress.Hex(), "err", err)
	} else {
		metadata.Reserve0 = BigIntToString(reserves[0])
		metadata.Reserve1 = BigIntToString(reserves[1])
	}

	// Get total supply of LP token
	totalSupply, err := v.caller.CallUint256(pair.PairAddress, TotalSupplySelector, statedb)
	if err == nil {
		metadata.TotalSupply = BigIntToString(totalSupply)
	}

	// Get pair token name and symbol
	if name, err := v.caller.CallString(pair.PairAddress, NameSelector, statedb); err == nil && name != "" {
		metadata.PairName = name
	}
	if symbol, err := v.caller.CallString(pair.PairAddress, SymbolSelector, statedb); err == nil && symbol != "" {
		metadata.PairSymbol = symbol
	}

	// Get token0 metadata
	token0Decimals := uint8(18) // default
	if name, err := v.caller.CallString(pair.Token0, NameSelector, statedb); err == nil && name != "" {
		metadata.Token0Name = name
	}
	if symbol, err := v.caller.CallString(pair.Token0, SymbolSelector, statedb); err == nil && symbol != "" {
		metadata.Token0Symbol = symbol
	}
	if decimals, err := v.caller.CallUint8(pair.Token0, DecimalsSelector, statedb); err == nil {
		metadata.Token0Decimals = &decimals
		token0Decimals = decimals
	}

	// Get token1 metadata
	token1Decimals := uint8(18) // default
	if name, err := v.caller.CallString(pair.Token1, NameSelector, statedb); err == nil && name != "" {
		metadata.Token1Name = name
	}
	if symbol, err := v.caller.CallString(pair.Token1, SymbolSelector, statedb); err == nil && symbol != "" {
		metadata.Token1Symbol = symbol
	}
	if decimals, err := v.caller.CallUint8(pair.Token1, DecimalsSelector, statedb); err == nil {
		metadata.Token1Decimals = &decimals
		token1Decimals = decimals
	}

	// Calculate initial price if reserves available
	if reserves != nil && len(reserves) >= 2 {
		metadata.InitialPrice = CalculatePrice(reserves[0], reserves[1], token0Decimals, token1Decimals)
	}

	return metadata, nil
}

// getReserves calls the getReserves() function of the pair contract
// Returns [reserve0, reserve1, blockTimestampLast]
func (v *Verifier) getReserves(pairAddr common.Address, statedb *state.StateDB) ([]*big.Int, error) {
	result, err := v.caller.CallContract(pairAddr, GetReservesSelector, statedb)
	if err != nil {
		return nil, err
	}

	// getReserves returns (uint112 reserve0, uint112 reserve1, uint32 blockTimestampLast)
	// Encoded as 3 * 32 bytes in ABI
	if len(result) < 96 {
		return nil, fmt.Errorf("invalid getReserves return length: %d", len(result))
	}

	reserve0 := new(big.Int).SetBytes(result[0:32])
	reserve1 := new(big.Int).SetBytes(result[32:64])
	// blockTimestampLast := binary.BigEndian.Uint32(result[92:96])

	return []*big.Int{reserve0, reserve1}, nil
}
