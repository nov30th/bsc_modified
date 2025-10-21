// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package contractutils

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// ContractCaller provides utility methods for calling smart contracts
type ContractCaller struct {
	blockchain  *core.BlockChain
	chainConfig *params.ChainConfig
}

// NewContractCaller creates a new contract caller
func NewContractCaller(blockchain *core.BlockChain) *ContractCaller {
	return &ContractCaller{
		blockchain:  blockchain,
		chainConfig: blockchain.Config(),
	}
}

// CallContract executes a contract call using the EVM
func (c *ContractCaller) CallContract(contractAddr common.Address, data []byte, statedb *state.StateDB) ([]byte, error) {
	// Create a minimal block context
	header := c.blockchain.CurrentHeader()
	context := core.NewEVMBlockContext(header, c.blockchain, nil)

	// Create EVM
	evm := vm.NewEVM(context, statedb, c.chainConfig, vm.Config{})

	// Call contract
	caller := vm.AccountRef(common.Address{})
	ret, _, err := evm.Call(caller, contractAddr, data, 100000, uint256.NewInt(0))

	if err != nil {
		return nil, fmt.Errorf("contract call failed: %w", err)
	}

	return ret, nil
}

// CallString calls a contract function that returns a string
func (c *ContractCaller) CallString(contractAddr common.Address, selector []byte, statedb *state.StateDB) (string, error) {
	result, err := c.CallContract(contractAddr, selector, statedb)
	if err != nil {
		return "", err
	}

	// Decode ABI encoded string
	return c.DecodeString(result)
}

// CallUint256 calls a contract function that returns uint256
func (c *ContractCaller) CallUint256(contractAddr common.Address, selector []byte, statedb *state.StateDB) (*big.Int, error) {
	result, err := c.CallContract(contractAddr, selector, statedb)
	if err != nil {
		return nil, err
	}

	if len(result) != 32 {
		return nil, fmt.Errorf("invalid uint256 return length: %d", len(result))
	}

	return new(big.Int).SetBytes(result), nil
}

// CallUint8 calls a contract function that returns uint8
func (c *ContractCaller) CallUint8(contractAddr common.Address, selector []byte, statedb *state.StateDB) (uint8, error) {
	result, err := c.CallContract(contractAddr, selector, statedb)
	if err != nil {
		return 0, err
	}

	if len(result) != 32 {
		return 0, fmt.Errorf("invalid uint8 return length: %d", len(result))
	}

	value := new(big.Int).SetBytes(result)
	if !value.IsUint64() || value.Uint64() > 255 {
		return 0, fmt.Errorf("invalid uint8 value: %s", value.String())
	}

	return uint8(value.Uint64()), nil
}

// CallAddress calls a contract function that returns an address
func (c *ContractCaller) CallAddress(contractAddr common.Address, selector []byte, statedb *state.StateDB) (common.Address, error) {
	result, err := c.CallContract(contractAddr, selector, statedb)
	if err != nil {
		return common.Address{}, err
	}

	if len(result) != 32 {
		return common.Address{}, fmt.Errorf("invalid address return length: %d", len(result))
	}

	return common.BytesToAddress(result), nil
}

// DecodeString decodes an ABI-encoded string
func (c *ContractCaller) DecodeString(data []byte) (string, error) {
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

// GetState returns the current blockchain state
func (c *ContractCaller) GetState() (*state.StateDB, error) {
	return c.blockchain.State()
}

// HasCode checks if an address has contract code
func (c *ContractCaller) HasCode(addr common.Address, statedb *state.StateDB) bool {
	code := statedb.GetCode(addr)
	return len(code) > 0
}
