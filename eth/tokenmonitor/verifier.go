// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package tokenmonitor

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// Verifier handles token contract verification
type Verifier struct {
	blockchain *core.BlockChain
	chainConfig *params.ChainConfig
}

// NewVerifier creates a new token verifier
func NewVerifier(blockchain *core.BlockChain) *Verifier {
	return &Verifier{
		blockchain: blockchain,
		chainConfig: blockchain.Config(),
	}
}

// VerifyAndExtract verifies a contract is a BEP-20 token and extracts metadata
func (v *Verifier) VerifyAndExtract(token *PendingToken) (*TokenMetadata, error) {
	// Get current state
	statedb, err := v.blockchain.State()
	if err != nil {
		return nil, fmt.Errorf("failed to get state: %w", err)
	}

	// 1. Verify core functions
	if !v.isBEP20(token.Address, statedb) {
		return nil, errors.New("not a BEP-20 token")
	}

	// 2. Extract metadata
	metadata := &TokenMetadata{
		Address:     token.Address.Hex(),
		Creator:     token.Creator.Hex(),
		TxHash:      token.TxHash.Hex(),
		BlockNumber: token.BlockNumber,
		Timestamp:   token.Timestamp,
	}

	// 3. Call totalSupply (required)
	if supply, err := v.callTotalSupply(token.Address, statedb); err == nil {
		metadata.TotalSupply = BigIntToString(supply)
	} else {
		return nil, fmt.Errorf("totalSupply failed: %w", err)
	}

	// 4. Call name (optional)
	if name, err := v.callString(token.Address, NameSelector, statedb); err == nil && name != "" {
		metadata.Name = name
		metadata.HasName = true
	}

	// 5. Call symbol (optional)
	if symbol, err := v.callString(token.Address, SymbolSelector, statedb); err == nil && symbol != "" {
		metadata.Symbol = symbol
		metadata.HasSymbol = true
	}

	// 6. Call decimals (optional)
	if decimals, err := v.callDecimals(token.Address, statedb); err == nil {
		metadata.Decimals = &decimals
		metadata.HasDecimals = true
	}

	// 7. Call getOwner (optional, BEP-20 specific)
	if owner, err := v.callGetOwner(token.Address, statedb); err == nil {
		ownerHex := owner.Hex()
		metadata.Owner = ownerHex
		metadata.HasGetOwner = true
	}

	return metadata, nil
}

// isBEP20 checks if a contract is a BEP-20 token by verifying core functions
func (v *Verifier) isBEP20(addr common.Address, statedb *state.StateDB) bool {
	// Check totalSupply()
	if _, err := v.callTotalSupply(addr, statedb); err != nil {
		return false
	}

	// Check balanceOf(address) with zero address
	if _, err := v.callBalanceOf(addr, common.Address{}, statedb); err != nil {
		return false
	}

	return true
}

// callTotalSupply calls the totalSupply() function
func (v *Verifier) callTotalSupply(addr common.Address, statedb *state.StateDB) (*big.Int, error) {
	data := TotalSupplySelector
	result, err := v.callContract(addr, data, statedb)
	if err != nil {
		return nil, err
	}

	if len(result) != 32 {
		return nil, fmt.Errorf("invalid totalSupply return length: %d", len(result))
	}

	return new(big.Int).SetBytes(result), nil
}

// callBalanceOf calls the balanceOf(address) function
func (v *Verifier) callBalanceOf(addr common.Address, account common.Address, statedb *state.StateDB) (*big.Int, error) {
	data := make([]byte, 4+32)
	copy(data[0:4], BalanceOfSelector)
	copy(data[4+12:4+32], account.Bytes()) // Pad address to 32 bytes

	result, err := v.callContract(addr, data, statedb)
	if err != nil {
		return nil, err
	}

	if len(result) != 32 {
		return nil, fmt.Errorf("invalid balanceOf return length: %d", len(result))
	}

	return new(big.Int).SetBytes(result), nil
}

// callString calls a string function (name or symbol)
func (v *Verifier) callString(addr common.Address, selector []byte, statedb *state.StateDB) (string, error) {
	result, err := v.callContract(addr, selector, statedb)
	if err != nil {
		return "", err
	}

	// Decode ABI encoded string
	return v.decodeString(result)
}

// callDecimals calls the decimals() function
func (v *Verifier) callDecimals(addr common.Address, statedb *state.StateDB) (uint8, error) {
	data := DecimalsSelector
	result, err := v.callContract(addr, data, statedb)
	if err != nil {
		return 0, err
	}

	if len(result) != 32 {
		return 0, fmt.Errorf("invalid decimals return length: %d", len(result))
	}

	decimals := new(big.Int).SetBytes(result)
	if !decimals.IsUint64() || decimals.Uint64() > 255 {
		return 0, fmt.Errorf("invalid decimals value: %s", decimals.String())
	}

	return uint8(decimals.Uint64()), nil
}

// callGetOwner calls the getOwner() function (BEP-20 specific)
func (v *Verifier) callGetOwner(addr common.Address, statedb *state.StateDB) (common.Address, error) {
	data := GetOwnerSelector
	result, err := v.callContract(addr, data, statedb)
	if err != nil {
		return common.Address{}, err
	}

	if len(result) != 32 {
		return common.Address{}, fmt.Errorf("invalid getOwner return length: %d", len(result))
	}

	return common.BytesToAddress(result), nil
}

// callContract executes a contract call using the EVM
func (v *Verifier) callContract(contractAddr common.Address, data []byte, statedb *state.StateDB) ([]byte, error) {
	// Create a minimal block context
	header := v.blockchain.CurrentHeader()
	context := core.NewEVMBlockContext(header, v.blockchain, nil)

	// Create EVM
	evm := vm.NewEVM(context, statedb, v.chainConfig, vm.Config{})

	// Call contract
	caller := vm.AccountRef(common.Address{})
	ret, _, err := evm.Call(caller, contractAddr, data, 100000, uint256.NewInt(0))

	if err != nil {
		return nil, fmt.Errorf("contract call failed: %w", err)
	}

	return ret, nil
}

// decodeString decodes an ABI-encoded string
func (v *Verifier) decodeString(data []byte) (string, error) {
	if len(data) < 64 {
		return "", fmt.Errorf("data too short for string")
	}

	// First 32 bytes: offset (should be 32)
	offset := new(big.Int).SetBytes(data[0:32]).Uint64()
	if offset != 32 {
		return "", fmt.Errorf("unexpected string offset: %d", offset)
	}

	// Next 32 bytes: length
	if uint64(len(data)) < offset+32 {
		return "", fmt.Errorf("data too short for string length")
	}

	length := new(big.Int).SetBytes(data[offset : offset+32]).Uint64()
	if length == 0 {
		return "", nil
	}

	// Remaining bytes: actual string data
	start := offset + 32
	if uint64(len(data)) < start+length {
		return "", fmt.Errorf("data too short for string content")
	}

	strData := data[start : start+length]

	// Remove null bytes
	strData = bytes.TrimRight(strData, "\x00")

	return string(strData), nil
}
