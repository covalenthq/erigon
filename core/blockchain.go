// Copyright 2014 The go-ethereum Authors
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

// Package core implements the Ethereum consensus protocol.
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ledgerwatch/erigon/core/systemcontracts"
	"github.com/ledgerwatch/erigon/rlp"
	"golang.org/x/crypto/sha3"
	"golang.org/x/exp/slices"

	metrics2 "github.com/VictoriaMetrics/metrics"
	"github.com/ledgerwatch/log/v3"

	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/common/math"
	"github.com/ledgerwatch/erigon/common/mclock"
	"github.com/ledgerwatch/erigon/common/u256"
	"github.com/ledgerwatch/erigon/consensus"
	"github.com/ledgerwatch/erigon/consensus/misc"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/params"
)

var (
	blockExecutionTimer = metrics2.GetOrCreateSummary("chain_execution_seconds")
)

type SyncMode string

const (
	TriesInMemory = 128
)

// statsReportLimit is the time limit during import and export after which we
// always print out progress. This avoids the user wondering what's going on.
const statsReportLimit = 8 * time.Second

type EphemeralExecResult struct {
	StateRoot         common.Hash           `json:"stateRoot"`
	TxRoot            common.Hash           `json:"txRoot"`
	ReceiptRoot       common.Hash           `json:"receiptRoot"`
	LogsHash          common.Hash           `json:"logsHash"`
	Bloom             types.Bloom           `json:"logsBloom"        gencodec:"required"`
	Receipts          types.Receipts        `json:"receipts"`
	Rejected          []*rejectedTx         `json:"rejected,omitempty"`
	Difficulty        *math.HexOrDecimal256 `json:"currentDifficulty" gencodec:"required"`
	GasUsed           math.HexOrDecimal64   `json:"gasUsed"`
	ReceiptForStorage *types.ReceiptForStorage
}

// report prints statistics if some number of blocks have been processed
// or more than a few seconds have passed since the last message.
func (st *InsertStats) Report(logPrefix string, chain []*types.Block, index int, toCommit bool) {
	// Fetch the timings for the batch
	var (
		now     = mclock.Now()
		elapsed = time.Duration(now) - time.Duration(st.StartTime)
	)
	// If we're at the last block of the batch or report period reached, log
	if index == len(chain)-1 || elapsed >= statsReportLimit || toCommit {
		// Count the number of transactions in this segment
		var txs int
		for _, block := range chain[st.lastIndex : index+1] {
			txs += len(block.Transactions())
		}
		end := chain[index]
		context := []interface{}{
			"blocks", st.Processed, "txs", txs,
			"elapsed", common.PrettyDuration(elapsed),
			"number", end.Number(), "hash", end.Hash(),
		}
		if timestamp := time.Unix(int64(end.Time()), 0); time.Since(timestamp) > time.Minute {
			context = append(context, []interface{}{"age", common.PrettyAge(timestamp)}...)
		}
		if st.queued > 0 {
			context = append(context, []interface{}{"queued", st.queued}...)
		}
		if st.ignored > 0 {
			context = append(context, []interface{}{"ignored", st.ignored}...)
		}
		log.Info(fmt.Sprintf("[%s] Imported new chain segment", logPrefix), context...)
		*st = InsertStats{StartTime: now, lastIndex: index + 1}
	}
}

// ExecuteBlockEphemerally runs a block from provided stateReader and
// writes the result to the provided stateWriter
func ExecuteBlockEphemerallyForBSC(
	chainConfig *params.ChainConfig,
	vmConfig *vm.Config,
	blockHashFunc func(n uint64) common.Hash,
	//getHeader func(hash common.Hash, number uint64) *types.Header,
	engine consensus.Engine,
	block *types.Block,
	stateReader state.StateReader,
	stateWriter state.WriterWithChangeSets,
	epochReader consensus.EpochReader,
	chainReader consensus.ChainHeaderReader,
	contractHasTEVM func(codeHash common.Hash) (bool, error),
) (types.Receipts, error) {
	defer blockExecutionTimer.UpdateDuration(time.Now())
	block.Uncles()
	ibs := state.New(stateReader)
	header := block.Header()
	var receipts types.Receipts
	usedGas := new(uint64)
	gp := new(GasPool)
	gp.AddGas(block.GasLimit())

	if !vmConfig.ReadOnly {
		if err := InitializeBlockExecution(engine, chainReader, epochReader, block.Header(), block.Transactions(), block.Uncles(), chainConfig, ibs); err != nil {
			return nil, err
		}
	}

	if chainConfig.DAOForkSupport && chainConfig.DAOForkBlock != nil && chainConfig.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(ibs)
	}
	systemcontracts.UpgradeBuildInSystemContract(chainConfig, header.Number, ibs)
	noop := state.NewNoopWriter()
	posa, isPoSA := engine.(consensus.PoSA)
	//fmt.Printf("====txs processing start: %d====\n", block.NumberU64())
	for i, tx := range block.Transactions() {
		if isPoSA {
			if isSystemTx, err := posa.IsSystemTransaction(tx, block.Header()); err != nil {
				return nil, err
			} else if isSystemTx {
				continue
			}
		}
		ibs.Prepare(tx.Hash(), block.Hash(), i)
		writeTrace := false
		if vmConfig.Debug && vmConfig.Tracer == nil {
			vmConfig.Tracer = vm.NewStructLogger(&vm.LogConfig{})
			writeTrace = true
		}

		receipt, _, err := ApplyTransaction(chainConfig, blockHashFunc, engine, nil, gp, ibs, noop, header, tx, usedGas, *vmConfig, contractHasTEVM)
		if writeTrace {
			w, err1 := os.Create(fmt.Sprintf("txtrace_%x.txt", tx.Hash()))
			if err1 != nil {
				panic(err1)
			}
			encoder := json.NewEncoder(w)
			logs := FormatLogs(vmConfig.Tracer.(*vm.StructLogger).StructLogs())
			if err2 := encoder.Encode(logs); err2 != nil {
				panic(err2)
			}
			if err2 := w.Close(); err2 != nil {
				panic(err2)
			}
			vmConfig.Tracer = nil
		}
		if err != nil {
			return nil, fmt.Errorf("could not apply tx %d from block %d [%v]: %w", i, block.NumberU64(), tx.Hash().Hex(), err)
		}
		if !vmConfig.NoReceipts {
			receipts = append(receipts, receipt)
		}
	}

	var newBlock *types.Block
	if !vmConfig.ReadOnly {
		// We're doing this hack for BSC to avoid changing consensus interfaces a lot. BSC modifies txs and receipts by appending
		// system transactions, and they increase used gas and write cumulative gas to system receipts, that's why we need
		// to deduct system gas before. This line is equal to "blockGas-systemGas", but since we don't know how much gas is
		// used by system transactions we just override. Of course, we write used by block gas back. It also always true
		// that used gas by block is always equal to original's block header gas, and it's checked by receipts root verification
		// otherwise it causes block verification error.
		header.GasUsed = *usedGas
		syscall := func(contract common.Address, data []byte) ([]byte, error) {
			return SysCallContract(contract, data, *chainConfig, ibs, header, engine)
		}
		outTxs, outReceipts, err := engine.Finalize(chainConfig, header, ibs, block.Transactions(), block.Uncles(), receipts, epochReader, chainReader, syscall)
		if err != nil {
			return nil, err
		}
		*usedGas = header.GasUsed

		// We need repack this block because transactions and receipts might be changed by consensus, and
		// it won't pass receipts hash or bloom verification
		newBlock = types.NewBlock(block.Header(), outTxs, block.Uncles(), outReceipts)
		// Update receipts
		if !vmConfig.NoReceipts {
			receipts = outReceipts
		}
	} else {
		newBlock = block
	}

	if chainConfig.IsByzantium(header.Number.Uint64()) && !vmConfig.NoReceipts {
		if newBlock.ReceiptHash() != block.ReceiptHash() {
			return nil, fmt.Errorf("mismatched receipt headers for block %d (%s != %s)", block.NumberU64(), newBlock.ReceiptHash().Hex(), block.Header().ReceiptHash.Hex())
		}
	}
	if newBlock.GasUsed() != header.GasUsed {
		return nil, fmt.Errorf("gas used by execution: %d, in header: %d", *usedGas, header.GasUsed)
	}
	if !vmConfig.NoReceipts {
		if newBlock.Bloom() != header.Bloom {
			return nil, fmt.Errorf("bloom computed by execution: %x, in header: %x", newBlock.Bloom(), header.Bloom)
		}
	}

	if err := ibs.CommitBlock(chainConfig.Rules(header.Number.Uint64()), stateWriter); err != nil {
		return nil, fmt.Errorf("committing block %d failed: %w", header.Number.Uint64(), err)
	} else if err := stateWriter.WriteChangeSets(); err != nil {
		return nil, fmt.Errorf("writing changesets for block %d failed: %w", header.Number.Uint64(), err)
	}

	return receipts, nil
}

type rejectedTx struct {
	Index int    `json:"index"`
	Err   string `json:"error"`
}

// ExecuteBlockEphemerally runs a block from provided stateReader and
// writes the result to the provided stateWriter
func ExecuteBlockEphemerally(
	chainConfig *params.ChainConfig,
	vmConfig *vm.Config, // configuration options for the interpreter
	blockHashFunc func(n uint64) common.Hash,
	//getHeader func(hash common.Hash, number uint64) *types.Header, // input: block header hash, block_height
	engine consensus.Engine,
	block *types.Block, // read from db
	stateReader state.StateReader,
	stateWriter state.WriterWithChangeSets, // writes are done by block finalization -> ibs.CommitBlock which updates the world state + changesets
	epochReader consensus.EpochReader,
	chainReader consensus.ChainHeaderReader,
	contractHasTEVM func(codeHash common.Hash) (bool, error), // transpiled evm (splitting evm codes into even lower level instructions)
	statelessExec bool, // for usage of this API via cli tools wherein some of the validations need to be relaxed.
) (*EphemeralExecResult, error) {
	//moskud: reads block from stateReader, runs it and writes the result to stateWriter
	// InitializeBlockExecution: pretty much a no-op (suppose to set the epoch etc.)
	// DaoHardFork state changes (modifies the state database - refunds to certain accounts)
	// For each transaction:
	//    - apply the transaction
	//    - write the traces (bool); append receipt (bool)
	// validations
	// create bloom filter (bool)
	// finalize block execution (bool) - reward to miner + commit the in-memory changes resulting from the block
	// some special handling for Bor consensus

	defer blockExecutionTimer.UpdateDuration(time.Now())
	block.Uncles()
	ibs := state.New(stateReader) // responsible for caching & managing state changes occurring during block execution
	header := block.Header()
	var receipts types.Receipts
	usedGas := new(uint64)
	gp := new(GasPool)
	gp.AddGas(block.GasLimit())

	var (
		rejectedTxs []*rejectedTx
		includedTxs types.Transactions
	)

	if !vmConfig.ReadOnly {
		// moskud: perform block finalization
		// for most consensus engines, Initialize is a no op (lol)
		if err := InitializeBlockExecution(engine, chainReader, epochReader, block.Header(), block.Transactions(), block.Uncles(), chainConfig, ibs); err != nil {
			return nil, err
		}
	}

	if chainConfig.DAOForkSupport && chainConfig.DAOForkBlock != nil && chainConfig.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(ibs)
	}
	noop := state.NewNoopWriter()
	//fmt.Printf("====txs processing start: %d====\n", block.NumberU64())
	for i, tx := range block.Transactions() {
		ibs.Prepare(tx.Hash(), block.Hash(), i) // moskud: IntraBlockState - set the current tx hash, block hash and tx_num
		writeTrace := false
		if vmConfig.Debug && vmConfig.Tracer == nil {
			vmConfig.Tracer = vm.NewStructLogger(&vm.LogConfig{})
			writeTrace = true
		}

		receipt, _, err := ApplyTransaction(chainConfig, blockHashFunc, engine, nil, gp, ibs, noop, header, tx, usedGas, *vmConfig, contractHasTEVM)
		if writeTrace {
			w, err1 := os.Create(fmt.Sprintf("txtrace_%x.txt", tx.Hash()))
			if err1 != nil {
				panic(err1)
			}
			encoder := json.NewEncoder(w)
			logs := FormatLogs(vmConfig.Tracer.(*vm.StructLogger).StructLogs())
			if err2 := encoder.Encode(logs); err2 != nil {
				panic(err2)
			}
			if err2 := w.Close(); err2 != nil {
				panic(err2)
			}
			vmConfig.Tracer = nil
		}
		if err != nil {
			rejectedTxs = append(rejectedTxs, &rejectedTx{i, err.Error()})
			// return nil, nil, fmt.Errorf("could not apply tx %d from block %d [%v]: %w", i, block.NumberU64(), tx.Hash().Hex(), err)
		} else {
			includedTxs = append(includedTxs, tx)
			if !vmConfig.NoReceipts {
				receipts = append(receipts, receipt)
			}
		}
	}

	var bloom types.Bloom
	var receiptSha common.Hash
	var err error

	if chainConfig.IsByzantium(header.Number.Uint64()) && !vmConfig.NoReceipts {
		receiptSha = types.DeriveSha(receipts)
		if !statelessExec && receiptSha != block.ReceiptHash() {
			return nil, fmt.Errorf("mismatched receipt headers for block %d", block.NumberU64())
		}
	}

	if !statelessExec && *usedGas != header.GasUsed {
		return nil, fmt.Errorf("gas used by execution: %d, in header: %d", *usedGas, header.GasUsed)
	}
	if !vmConfig.NoReceipts {
		bloom = types.CreateBloom(receipts)
		if !statelessExec && bloom != header.Bloom {
			return nil, fmt.Errorf("bloom computed by execution: %x, in header: %x", bloom, header.Bloom)
		}
	}
	if !vmConfig.ReadOnly {
		txs := block.Transactions()
		if _, err = FinalizeBlockExecution(engine, stateReader, block.Header(), txs, block.Uncles(), stateWriter, chainConfig, ibs, receipts, epochReader, chainReader, false); err != nil {
			return nil, err // todo: something that bothers me is these returns. It used to return when ever a trnasaction failed. So, maybe some of these returns are like that; can be replace with something else. Like we did with rejected transactions.
		}
	}

	var logs []*types.Log
	for _, receipt := range receipts {
		logs = append(logs, receipt.Logs...)
	}

	blockLogs := ibs.Logs()
	var stateSyncReceipt *types.ReceiptForStorage
	if chainConfig.Consensus == params.BorConsensus && len(blockLogs) > 0 {
		var stateSyncLogs []*types.Log
		slices.SortStableFunc(blockLogs, func(i, j *types.Log) bool { return i.Index < j.Index })

		if len(blockLogs) > len(logs) {
			stateSyncLogs = blockLogs[len(logs):] // get state-sync logs from `state.Logs()`

			types.DeriveFieldsForBorLogs(stateSyncLogs, block.Hash(), block.NumberU64(), uint(len(receipts)), uint(len(logs)))

			stateSyncReceipt = &types.ReceiptForStorage{
				Status: types.ReceiptStatusSuccessful, // make receipt status successful
				Logs:   stateSyncLogs,
			}
		}
	}

	execRs := &EphemeralExecResult{
		TxRoot:            types.DeriveSha(includedTxs),
		ReceiptRoot:       receiptSha,
		Bloom:             bloom,
		LogsHash:          rlpHash(blockLogs),
		Receipts:          receipts,
		GasUsed:           math.HexOrDecimal64(*usedGas),
		Rejected:          rejectedTxs,
		ReceiptForStorage: stateSyncReceipt,
	}

	// return receipts, stateSyncReceipt, execRs, nil
	return execRs, nil
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, x) //nolint:errcheck
	hw.Sum(h[:0])
	return h
}

// SysCallContract moskud: calls the contract with the given message (data) and returns the data which the contract execution returned.
func SysCallContract(contract common.Address, data []byte, chainConfig params.ChainConfig, ibs *state.IntraBlockState, header *types.Header, engine consensus.Engine) (result []byte, err error) {
	// moskud:
	// apply dao fork statedb changes
	// construct a message from data passed
	// create a new evm and execute the message (contract) call, returning what the contract function returned.

	gp := new(GasPool).AddGas(50_000_000)

	if chainConfig.DAOForkSupport && chainConfig.DAOForkBlock != nil && chainConfig.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(ibs)
	}

	msg := types.NewMessage( // Message is a fully derived transaction and implements core.Message
		state.SystemAddress,
		&contract,
		0, u256.Num0,
		50_000_000, u256.Num0,
		nil, nil,
		data, nil, false,
	)
	vmConfig := vm.Config{NoReceipts: true}
	// Create a new context to be used in the EVM environment
	isBor := chainConfig.Bor != nil
	var author *common.Address
	if isBor {
		author = &header.Coinbase
	} else {
		author = &state.SystemAddress
	}
	blockContext := NewEVMBlockContext(header, GetHashFn(header, nil), engine, author, nil)
	evm := vm.NewEVM(blockContext, NewEVMTxContext(msg), ibs, &chainConfig, vmConfig)
	if isBor {
		ret, _, err := evm.Call(
			vm.AccountRef(msg.From()),
			*msg.To(),
			msg.Data(),
			msg.Gas(),
			msg.Value(),
			false,
		)
		if err != nil {
			return nil, nil
		}
		return ret, nil
	}
	// ApplyMessage computes the new state by applying the given message
	// against the old state within the environment.
	res, err := ApplyMessage(evm, msg, gp, true /* refunds */, false /* gasBailout */)
	if err != nil {
		return nil, err
	}
	return res.ReturnData, nil
}

// from the null sender, with 50M gas.
func SysCallContractTx(contract common.Address, data []byte) (tx types.Transaction, err error) {
	//nonce := ibs.GetNonce(SystemAddress)
	tx = types.NewTransaction(0, contract, u256.Num0, 50_000_000, u256.Num0, data)
	return tx.FakeSign(state.SystemAddress)
}

func CallContract(contract common.Address, data []byte, chainConfig params.ChainConfig, ibs *state.IntraBlockState, header *types.Header, engine consensus.Engine) (result []byte, err error) {
	gp := new(GasPool)
	gp.AddGas(50_000_000)
	var gasUsed uint64

	if chainConfig.DAOForkSupport && chainConfig.DAOForkBlock != nil && chainConfig.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(ibs)
	}
	noop := state.NewNoopWriter()
	tx, err := CallContractTx(contract, data, ibs)
	if err != nil {
		return nil, fmt.Errorf("SysCallContract: %w ", err)
	}
	vmConfig := vm.Config{NoReceipts: true}
	_, result, err = ApplyTransaction(&chainConfig, GetHashFn(header, nil), engine, &state.SystemAddress, gp, ibs, noop, header, tx, &gasUsed, vmConfig, nil)
	if err != nil {
		return result, fmt.Errorf("SysCallContract: %w ", err)
	}
	return result, nil
}

// from the null sender, with 50M gas.
func CallContractTx(contract common.Address, data []byte, ibs *state.IntraBlockState) (tx types.Transaction, err error) {
	from := common.Address{}
	nonce := ibs.GetNonce(from)
	tx = types.NewTransaction(nonce, contract, u256.Num0, 50_000_000, u256.Num0, data)
	return tx.FakeSign(from)
}

// moskud: Finalize i.e. allocate the block rewards to the miner (& optionally assemble the resulting block)
// commit block : finalize the state
// some special handling for Sokol network (chain ID 77) -- don't care
// write the change sets (account and storage diffs) into db
func FinalizeBlockExecution(engine consensus.Engine, stateReader state.StateReader, header *types.Header,
	txs types.Transactions, uncles []*types.Header, stateWriter state.WriterWithChangeSets, cc *params.ChainConfig, ibs *state.IntraBlockState,
	receipts types.Receipts, e consensus.EpochReader, headerReader consensus.ChainHeaderReader, isMining bool,
) (newBlock *types.Block, err error) {

	syscall := func(contract common.Address, data []byte) ([]byte, error) {
		return SysCallContract(contract, data, *cc, ibs, header, engine)
	}

	if isMining {
		newBlock, _, _, err = engine.FinalizeAndAssemble(cc, header, ibs, txs, uncles, receipts, e, headerReader, syscall, nil)
	} else {
		_, _, err = engine.Finalize(cc, header, ibs, txs, uncles, receipts, e, headerReader, syscall) //moskud: accumulate rewards and increment (change ibs) miner's balance
	}
	if err != nil {
		return nil, err
	}

	var originalSystemAcc *accounts.Account
	if cc.ChainID.Uint64() == 77 { // hack for Sokol - don't understand why eip158 is enabled, but OE still save SystemAddress with nonce=0
		n := ibs.GetNonce(state.SystemAddress) //hack - because syscall must use ApplyMessage instead of ApplyTx (and don't create tx at all). But CallContract must create tx.
		if n > 0 {
			var err error
			originalSystemAcc, err = stateReader.ReadAccountData(state.SystemAddress)
			if err != nil {
				return nil, err
			}
		}
	}

	// moskud: write account data, code, storage into the stateWriter
	// also clears out the journal & refund
	if err := ibs.CommitBlock(cc.Rules(header.Number.Uint64()), stateWriter); err != nil {
		return nil, fmt.Errorf("committing block %d failed: %w", header.Number.Uint64(), err)
	}

	if originalSystemAcc != nil { // hack for Sokol - don't understand why eip158 is enabled, but OE still save SystemAddress with nonce=0
		acc := accounts.NewAccount()
		acc.Nonce = 0
		if err := stateWriter.UpdateAccountData(state.SystemAddress, originalSystemAcc, &acc); err != nil {
			return nil, err
		}
	}

	// moskud: writing account & storage change sets into db
	if err := stateWriter.WriteChangeSets(); err != nil {
		return nil, fmt.Errorf("writing changesets for block %d failed: %w", header.Number.Uint64(), err)
	}
	return newBlock, nil
}

func InitializeBlockExecution(engine consensus.Engine, chain consensus.ChainHeaderReader, epochReader consensus.EpochReader, header *types.Header, txs types.Transactions, uncles []*types.Header, cc *params.ChainConfig, ibs *state.IntraBlockState) error {
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)

	// moskud:
	// Initialize runs any pre-transaction state modifications (e.g. epoch start)
	// not sure where the block finalization state transition (like reward distribution) is done here.
	engine.Initialize(cc, chain, epochReader, header, txs, uncles, func(contract common.Address, data []byte) ([]byte, error) {
		return SysCallContract(contract, data, *cc, ibs, header, engine)
	})
	return nil
}
