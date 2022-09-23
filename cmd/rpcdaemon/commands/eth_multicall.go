package commands

import (
  "bytes"
  "context"
  "fmt"
  "sync"
  "time"

  "golang.org/x/crypto/sha3"
  "github.com/holiman/uint256"
  common2 "github.com/ledgerwatch/erigon-lib/common"
  "github.com/ledgerwatch/erigon/common"
  "github.com/ledgerwatch/erigon/common/hexutil"
  "github.com/ledgerwatch/erigon/common/math"
  "github.com/ledgerwatch/erigon/core"
  "github.com/ledgerwatch/erigon/core/state"
  "github.com/ledgerwatch/erigon/core/vm"
  "github.com/ledgerwatch/erigon/crypto"
  "github.com/ledgerwatch/erigon/internal/ethapi"
  "github.com/ledgerwatch/erigon/rpc"
  "github.com/ledgerwatch/erigon/turbo/rpchelper"
  "github.com/ledgerwatch/erigon/turbo/transactions"
  "github.com/ledgerwatch/log/v3"
)

type MulticallRunlist map[rpc.BlockNumber]MulticallPerBlockRunlist
type MulticallPerBlockRunlist map[common.Address][]hexutil.Bytes

type MulticallResult map[uint64]MulticallPerBlockResult
type MulticallPerBlockResult map[common.Address][]*MulticallExecutionResult

type MulticallExecutionResult struct {
  UsedGas    uint64
  ReturnData hexutil.Bytes `json:",omitempty"`
  Err        string        `json:",omitempty"`
}

// hasherPool holds LegacyKeccak hashers.
var hasherPool = sync.Pool{
  New: func() interface{} {
    return sha3.NewLegacyKeccak256()
  },
}

func computeWithCachedBalanceSlot(stateReader state.StateReader, contractAddr common.Address, holderAddr []byte) ([]byte, bool) {
  baseSlot, ok := vm.ContractBalanceOfSlotCache.Load(contractAddr)
  if !ok || baseSlot == nil {
    return nil, false
  }
  baseSlotH := baseSlot.(common.Hash)

  var empty []byte

  contractAcc, err := stateReader.ReadAccountData(contractAddr)
  if err != nil {
    return nil, false
  }
  if contractAcc == nil {
    return empty, true
  }

  locBuf := make([]byte, 0, 64)

  locBuf = append(locBuf, holderAddr...)
  locBuf = append(locBuf, baseSlotH.Bytes()...)

  var hashedLoc common.Hash

  hasher := hasherPool.Get().(crypto.KeccakState)
  defer hasherPool.Put(hasher)
  hasher.Reset()
  hasher.Write(locBuf)
  if _, err := hasher.Read(hashedLoc[:]); err != nil {
    panic(err)
  }

  res, err := stateReader.ReadAccountStorage(contractAddr, contractAcc.Incarnation, &hashedLoc)
  if err != nil {
    return nil, false
  }
  return common.LeftPadBytes(res, 32), true
}

// Call implements eth_call. Executes a new message call immediately without creating a transaction on the block chain.
func (api *APIImpl) Multicall(ctx context.Context, commonCallArgs ethapi.CallArgs, contractsWithPayloadsByBlock MulticallRunlist, overrides *map[common.Address]ethapi.Account) (MulticallResult, error) {
  startTime := time.Now()

  // result stores
  execResults := make(MulticallResult)

  dbtx, err := api.db.BeginRo(ctx)
  if err != nil {
    return nil, err
  }
  defer dbtx.Rollback()

  chainConfig, err := api.chainConfig(dbtx)
  if err != nil {
    return nil, err
  }

  noopWriter := state.NewNoopWriter()
  vmConfig := vm.Config{}

  // Setup context so it may be cancelled the call has completed
  // or, in case of unmetered gas, setup a context with a timeout.
  var cancel context.CancelFunc
  var execCtx context.Context
  if api.evmCallTimeout > 0 {
    execCtx, cancel = context.WithTimeout(ctx, api.evmCallTimeout)
  } else {
    execCtx, cancel = context.WithCancel(ctx)
  }
  // Make sure the context is cancelled when the call has completed
  // this makes sure resources are cleaned up.
  defer cancel()

  var execSeq int
  var numAccelerated int

  for number, contractsWithPayloads := range contractsWithPayloadsByBlock {
    blockExecResults := make(MulticallPerBlockResult)

    blockNrOrHash := rpc.BlockNumberOrHashWithNumber(number)

    blockNumber, _, _, err := rpchelper.GetCanonicalBlockNumber(blockNrOrHash, dbtx, api.filters)
    if err != nil {
      return nil, err
    }
    block, err := api.BaseAPI.blockByNumberWithSenders(dbtx, blockNumber)
    if err != nil {
      return nil, err
    }

    blockHeader := block.Header()

    var baseFee *uint256.Int
    if blockHeader.BaseFee != nil {
      var overflow bool
      baseFee, overflow = uint256.FromBig(blockHeader.BaseFee)
      if overflow {
        return nil, fmt.Errorf("header.BaseFee uint256 overflow")
      }
    }

    var stateReader state.StateReader
    stateReader, err = rpchelper.CreateStateReader(ctx, dbtx, blockNrOrHash, api.filters, api.stateCache, api.historyV2(dbtx), api._agg)
    if err != nil {
      return nil, err
    }

    callArgsBuf := commonCallArgs
    callArgsBuf.MaxFeePerGas = (*hexutil.Big)(blockHeader.BaseFee)

    if callArgsBuf.Gas == nil || uint64(*callArgsBuf.Gas) == 0 {
      callArgsBuf.Gas = (*hexutil.Uint64)(&api.GasCap)
    }

    ibs := state.New(stateReader)

    for contractAddr, payloads := range contractsWithPayloads {
      callArgsBuf.To = &contractAddr

      contractExecResults := make([]*MulticallExecutionResult, 0, len(payloads))

      for _, payload := range payloads {
        if err := common2.Stopped(execCtx.Done()); err != nil {
          return nil, err
        }

        // var accelCrosscheckResult []byte

        if bytes.HasPrefix(payload, vm.BALANCEOF_SELECTOR) && len(payload) == 36 {
          if result, ok := computeWithCachedBalanceSlot(stateReader, contractAddr, payload[4:36]); ok {
            // accelCrosscheckResult = result
            mcExecResult := &MulticallExecutionResult{
              UsedGas:    0,
              ReturnData: result,
            }

            contractExecResults = append(contractExecResults, mcExecResult)
            numAccelerated++
            execSeq++
            continue
          }
        }

        callArgsBuf.Data = (*hexutil.Bytes)(&payload)

        msg, err := callArgsBuf.ToMessage(api.GasCap, baseFee)
        if err != nil {
          return nil, err
        }

        blockCtx, txCtx := transactions.GetEvmContext(msg, blockHeader, blockNrOrHash.RequireCanonical, dbtx, api._blockReader)
        blockCtx.GasLimit = math.MaxUint64
        blockCtx.MaxGasLimit = true

        evm := vm.NewEVM(blockCtx, txCtx, ibs, chainConfig, vmConfig)
        gp := new(core.GasPool).AddGas(msg.Gas())

        // Clone the state cache before applying the changes, clone is discarded
        ibs.Prepare(common.Hash{}, blockHeader.Hash(), execSeq)

        execResult, applyMsgErr := core.ApplyMessage(evm, msg, gp, true /* refunds */, true /* gasBailout */)

        var effectiveErrDesc string
        if applyMsgErr != nil {
          effectiveErrDesc = applyMsgErr.Error()
        } else if execResult.Err != nil {
          effectiveErrDesc = ethapi.NewRevertError(execResult).Error()
        }

        // if accelCrosscheckResult != nil && len(execResult.ReturnData) > 0 && !bytes.Equal(execResult.ReturnData, accelCrosscheckResult) {
        //   panic(fmt.Sprintf(
        //     "balanceOf accel failure: block=%d contract=%v holder=%v slowPath=%v slowPathErr=%s fastPath=%v",
        //     blockNumber,
        //     contractAddr,
        //     common.BytesToAddress(payload[4:36]),
        //     new(uint256.Int).SetBytes(execResult.ReturnData),
        //     effectiveErrDesc,
        //     new(uint256.Int).SetBytes(accelCrosscheckResult),
        //   ))
        // }

        chainRules := evm.ChainRules()

        if err = ibs.FinalizeTx(chainRules, noopWriter); err != nil {
          return nil, err
        }
        if err = ibs.CommitBlock(chainRules, noopWriter); err != nil {
          return nil, err
        }

        mcExecResult := &MulticallExecutionResult{
          UsedGas: execResult.UsedGas,
          Err:     effectiveErrDesc,
        }

        if len(execResult.ReturnData) > 0 {
          mcExecResult.ReturnData = common.CopyBytes(execResult.ReturnData)
        }

        contractExecResults = append(contractExecResults, mcExecResult)
        execSeq++
      }

      blockExecResults[contractAddr] = contractExecResults
    }

    execResults[blockNumber] = blockExecResults
  }

  log.Info("Executed eth_multicall", "fast_path", numAccelerated, "slow_path", (execSeq - numAccelerated), "runtime_usec", time.Since(startTime).Microseconds())

  return execResults, nil
}

