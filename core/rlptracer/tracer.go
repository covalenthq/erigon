package rlptracer

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"github.com/ledgerwatch/erigon/core"
	"github.com/ledgerwatch/erigon/eth/tracers/logger"
	"github.com/ledgerwatch/log/v3"
)

// import (
// 	"context"
// 	"fmt"
// 	"io"
// 	"math/big"
// 	"reflect"

// 	"github.com/ledgerwatch/erigon/common"
// 	"github.com/ledgerwatch/erigon/core"
// 	"github.com/ledgerwatch/erigon/core/state"
// 	"github.com/ledgerwatch/erigon/core/types"
// 	"github.com/ledgerwatch/erigon/core/vm"
// 	"github.com/ledgerwatch/erigon/log"
// 	"github.com/ledgerwatch/erigon/params"
// )

type Tracer struct {
	cfg       *logger.LogConfig
	activeMsg core.Message
	// activeState    *state.StateDB
	stackTop       stackFrame
	preimageSource PreimageSource

	master *Tracer

	fromBlockNr uint64
	toBlockNr   uint64

	writer                *countingWriter
	renderAndReorderInput chan Traces
	renderAndReorderDone  chan struct{}
}

// // NewTracer creates a new EVM tracer that prints execution steps as RLP
// // records into the provided stream.
func NewTracer(cfg *logger.LogConfig, writer io.Writer) *Tracer {
	bottomFrame := &globalProxyFrame{}

	tr := &Tracer{
		cfg:                   cfg,
		stackTop:              bottomFrame,
		preimageSource:        NewEphemeralPreimageSource(nil),
		writer:                &countingWriter{writer: writer},
		renderAndReorderInput: make(chan Traces, 1000),
		renderAndReorderDone:  make(chan struct{}),
	}

	bottomFrame.SetTracer(tr)

	return tr
}

// func (tr *Tracer) NewWorker() (*Tracer, error) {
// 	workerTr := &Tracer{
// 		master:         tr,
// 		cfg:            tr.cfg,
// 		stackTop:       tr.stackTop,
// 		preimageSource: tr.preimageSource,
// 	}

// 	return workerTr, nil
// }

func (tr *Tracer) CaptureTraceBegin(ctx context.Context, fromNr uint64, toNr uint64) error {
	tr.fromBlockNr = fromNr
	tr.toBlockNr = toNr

	go tr.renderAndReorderLoop(ctx)

	return nil
}

// func (tr *Tracer) CaptureTraceEnd() error {
// 	close(tr.renderAndReorderInput)
// 	<-tr.renderAndReorderDone

// 	return nil
// }

// func (tr *Tracer) CaptureBlockBegin(block *types.Block, config *params.ChainConfig, stateBefore *state.StateDB) error {
// 	frame := NewBlockFrame(block, config, stateBefore)
// 	tr.pushFrame(frame)
// 	return nil
// }

// func (tr *Tracer) CaptureBlockEnd(stateAfter *state.StateDB) error {
// 	blockFrame := tr.mergeUntilFrame(reflect.TypeOf((*BlockFrame)(nil))).(*BlockFrame)
// 	defer tr.mergeStackDown()

// 	return blockFrame.AnnotateAfter(stateAfter)
// }

// func (tr *Tracer) CaptureTxBegin(block_offset int, msg interface{}, stateBefore *state.StateDB) error {
// 	msgR := msg.(core.Message)

// 	frame := NewTransactionFrame(block_offset, msgR)
// 	tr.pushFrame(frame)
// 	tr.activeMsg = msgR
// 	tr.activeState = stateBefore
// 	return nil
// }

// func (tr *Tracer) CaptureTxEnd(ret []byte, usedGas uint64, failed bool, err error) error {
// 	txFrame := tr.mergeUntilFrame(reflect.TypeOf((*TransactionFrame)(nil))).(*TransactionFrame)
// 	defer tr.mergeStackDown()

// 	tr.activeState = nil
// 	tr.activeMsg = nil

// 	return txFrame.AnnotateAfter(ret, usedGas, failed, err)
// }

// func (tr *Tracer) CaptureTxSenderBuyGas(origin common.Address, oldBalance *big.Int, newBalance *big.Int) error {
// 	effect := NewEffectAccountBalanceModified(origin, oldBalance, newBalance)
// 	if effect != nil {
// 		txFrame := tr.stackTop.(*TransactionFrame)
// 		txFrame.Traces().AddSideEffectAfter(effect)
// 	}
// 	return nil
// }

// func (tr *Tracer) CaptureTxComputerReward(coinbase common.Address, oldBalance *big.Int, newBalance *big.Int) error {
// 	effect := NewEffectAccountBalanceModified(coinbase, oldBalance, newBalance)
// 	if effect != nil {
// 		txFrame := tr.stackTop.(*TransactionFrame)
// 		txFrame.Traces().AddSideEffectAfter(effect)

// 		blkFrame := txFrame.parent.(*BlockFrame)
// 		blkFrame.CoinbaseReceivedValue()
// 	}
// 	return nil
// }

// func (tr *Tracer) CaptureMessageCallBeginFromClosure(caller common.Address, deployTarget common.Address, gasBefore uint64, valueOffered *big.Int, payload []byte) error {
// 	return tr.captureMessageCallBegin(true, caller, deployTarget, gasBefore, valueOffered, payload)
// }

// func (tr *Tracer) CaptureMessageCallBeginFromModule(caller common.Address, contract common.Address, gasBefore uint64, valueOffered *big.Int, payload []byte) error {
// 	return tr.captureMessageCallBegin(false, caller, contract, gasBefore, valueOffered, payload)
// }

// func (tr *Tracer) captureMessageCallBegin(isClosure bool, from common.Address, to common.Address, gasBefore uint64, valueOffered *big.Int, payload []byte) error {
// 	frame := NewMessageCallFrame(
// 		tr.nextMessageCallSeq(),
// 		from,
// 		to,
// 		isClosure,
// 		gasBefore,
// 		valueOffered,
// 		payload,
// 	)
// 	tr.pushFrame(frame)

// 	return nil
// }

// func (tr *Tracer) CaptureMessageCallWillConsumeOfferedValue() error {
// 	msgFrame := tr.findFrame(reflect.TypeOf((*MessageCallFrame)(nil))).(*MessageCallFrame)
// 	if msgFrame == nil {
// 		panic("tracer could not find enclosing MessageCallFrame to consume offered value")
// 	}

// 	msgFrame.ConsumeOfferedValue(
// 		tr.activeState.GetBalancePlusHolds(msgFrame.from),
// 		tr.activeState.GetBalancePlusHolds(msgFrame.to),
// 		tr.activeState.Empty(msgFrame.to),
// 	)

// 	return nil
// }

// func (tr *Tracer) CaptureMessageCallEnd(output []byte, err error, gasAfter uint64) error {
// 	msgFrame := tr.mergeUntilFrame(reflect.TypeOf((*MessageCallFrame)(nil))).(*MessageCallFrame)
// 	defer tr.mergeStackDown()

// 	var codeHash common.Hash
// 	if msgFrame.isClosure {
// 		// optimization over hashing output
// 		codeHash = tr.activeState.GetCodeHash(msgFrame.to)
// 	}

// 	return msgFrame.AnnotateAfter(output, err, gasAfter, codeHash)
// }

// func (tr *Tracer) CaptureInterpreterBegin(contract *vm.Contract, input []byte, readOnly bool) error {
// 	frame := NewInterpreterFrame(
// 		tr.nextInterpreterSeq(),
// 		contract.CodeHash,
// 		input,
// 		contract.Gas,
// 		readOnly,
// 	)
// 	tr.pushFrame(frame)
// 	return nil
// }

// func (tr *Tracer) CaptureInterpreterEnd(output []byte, err error) error {
// 	interpreterFrame := tr.mergeUntilFrame(reflect.TypeOf((*InterpreterFrame)(nil))).(*InterpreterFrame)
// 	defer tr.mergeStackDown()
// 	return interpreterFrame.AnnotateAfter(output, err, tr.activeState.GetRefund())
// }

// func (tr *Tracer) CaptureInterpreterOpBegin(op vm.OpCode, gasBefore uint64) error {
// 	frame := NewOpExecutionFrame(gasBefore, op)
// 	tr.pushFrame(frame)
// 	return nil
// }

// func (tr *Tracer) CaptureInterpreterOpEnd(pc uint64, res []byte, err error, gasAfter uint64) error {
// 	frame := tr.mergeUntilFrame(reflect.TypeOf((*OpExecutionFrame)(nil))).(*OpExecutionFrame)
// 	defer tr.mergeStackDown()

// 	return frame.AnnotateAfter(pc, res, err, gasAfter)
// }

// func (tr *Tracer) CaptureInterpreterClockTick() error {
// 	interpreterFrame := tr.findFrame(reflect.TypeOf((*InterpreterFrame)(nil))).(*InterpreterFrame)
// 	if interpreterFrame == nil {
// 		panic("tracer could not find enclosing InterpreterFrame to advance opClock")
// 	}

// 	interpreterFrame.AdvanceOpClock()
// 	return nil
// }

// func (tr *Tracer) CaptureAccountSendMessage(sender, recipient common.Address, data []byte) error {
// 	effect := NewEffectAccountSentMessage(sender, recipient, data)
// 	tr.stackTop.Traces().AddSideEffectAfter(effect)
// 	return nil
// }

// func (tr *Tracer) CaptureAccountReserveStorage(newAccount common.Address) error {
// 	effect := NewEffectAccountStorageReserved(newAccount)
// 	tr.stackTop.Traces().AddSideEffectBefore(effect)
// 	return nil
// }

// func (tr *Tracer) CaptureAccountReleaseStorage(addr common.Address) error {
// 	effect := NewEffectAccountStorageReleased(addr)
// 	tr.stackTop.Traces().AddSideEffectBefore(effect)
// 	return nil
// }

// func (tr *Tracer) CaptureAccountClaimBonanza(account common.Address, weiClaimed *big.Int) error {
// 	// if weiClaimed.Sign() == 0 {
// 	// 	return nil
// 	// }

// 	// accountBalAfter := tr.activeState.GetBalance(account)

// 	// traces := tr.stackTop.Traces()
// 	// traces.AddSideEffectAfter(NewEffectAccountBalanceUnmasked(account, new(big.Int), accountBalAfter))
// 	return nil
// }

// func (tr *Tracer) CaptureValueTransfer(sender, receiver common.Address, weiSent *big.Int) error {
// 	if weiSent.Sign() == 0 {
// 		return nil
// 	}

// 	traces := tr.stackTop.Traces()

// 	senderBalAfter := tr.activeState.GetBalancePlusHolds(sender)
// 	senderBalBefore := new(big.Int).Add(senderBalAfter, weiSent)
// 	traces.AddSideEffectAfter(NewEffectAccountBalanceModified(sender, senderBalBefore, senderBalAfter))

// 	receiverBalAfter := tr.activeState.GetBalancePlusHolds(receiver)
// 	receiverBalBefore := new(big.Int).Sub(receiverBalAfter, weiSent)
// 	traces.AddSideEffectAfter(NewEffectAccountBalanceModified(receiver, receiverBalBefore, receiverBalAfter))

// 	return nil
// }

// func (tr *Tracer) CaptureAccountBecomeExternal(addr common.Address) error {
// 	effect := NewEffectAccountBecameExternal(addr)
// 	tr.stackTop.Traces().AddSideEffectBefore(effect)
// 	return nil
// }

// func (tr *Tracer) CaptureAccountBecomeInternal(addr common.Address, prog common.Hash) error {
// 	effect := NewEffectAccountBecameInternal(addr, prog)
// 	tr.stackTop.Traces().AddSideEffectBefore(effect)
// 	return nil
// }

// func (tr *Tracer) CaptureAccountBecomeBlackhole(addr common.Address) error {
// 	effect := NewEffectAccountBecameBlackhole(addr)
// 	tr.stackTop.Traces().AddSideEffectBefore(effect)
// 	return nil
// }

// func (tr *Tracer) CaptureSuicide(dead, inheritor common.Address, weiInherited *big.Int) error {
// 	traces := tr.stackTop.Traces()

// 	if weiInherited.Sign() != 0 {
// 		traces.AddSideEffectAfter(NewEffectAccountBalanceModified(dead, weiInherited, &bigZero))

// 		inheritorBalAfter := tr.activeState.GetBalancePlusHolds(inheritor)
// 		inheritorBalBefore := new(big.Int).Sub(inheritorBalAfter, weiInherited)
// 		traces.AddSideEffectAfter(NewEffectAccountBalanceModified(inheritor, inheritorBalBefore, inheritorBalAfter))

// 		if tr.activeState.GetLifecycleStage(inheritor) == state.AccountStageBonanza && inheritorBalBefore.Sign() == 0 {
// 			traces.AddSideEffectAfter(NewEffectAccountBecameBonanza(inheritor))
// 		}
// 	}

// 	traces.AddSideEffectAfter(NewEffectAccountBecameEmpty(dead))
// 	traces.AddSideEffectAfter(NewEffectAccountStorageReleased(dead))

// 	return nil
// }

// func (tr *Tracer) CaptureConditionalJump() error {
// 	interpreterFrame := tr.findFrame(reflect.TypeOf((*InterpreterFrame)(nil))).(*InterpreterFrame)
// 	if interpreterFrame == nil {
// 		panic("tracer could not find enclosing InterpreterFrame to notify of conditional jump")
// 	}
// 	interpreterFrame.CaptureConditionalJump()
// 	return nil
// }

// func (tr *Tracer) CaptureLog(topics []common.Hash, data []byte) error {
// 	effect := NewOpEffectLogEmitted(topics, data)
// 	tr.stackTop.Traces().AddSideEffectAfter(effect)
// 	return nil
// }

// func (tr *Tracer) CaptureStorageWrite(acct common.Address, loc common.Hash, oldValue common.Hash, newValue common.Hash) error {
// 	effect := NewOpEffectStorageModified(acct, loc, oldValue, newValue)
// 	if effect != nil {
// 		tr.stackTop.Traces().AddSideEffectAfter(effect)
// 	}
// 	return nil
// }

// func (tr *Tracer) CaptureStorageRead(acct common.Address, loc common.Hash, value common.Hash) error {
// 	effect := NewEffectStorageRead(acct, loc, value)
// 	if effect != nil {
// 		tr.stackTop.Traces().AddSideEffectAfter(effect)
// 	}
// 	return nil
// }

// func (tr *Tracer) CaptureBlockUncleReward(coinbase common.Address, oldBalance *big.Int, newBalance *big.Int) error {
// 	effect := NewEffectAccountBalanceModified(coinbase, oldBalance, newBalance)
// 	if effect != nil {
// 		blkFrame := tr.stackTop.(*BlockFrame)
// 		blkFrame.Traces().AddSideEffectAfter(effect)
// 		blkFrame.CoinbaseReceivedValue()
// 	}
// 	return nil
// }

// func (tr *Tracer) CaptureBlockMinerReward(coinbase common.Address, oldBalance *big.Int, newBalance *big.Int) error {
// 	effect := NewEffectAccountBalanceModified(coinbase, oldBalance, newBalance)
// 	if effect != nil {
// 		blkFrame := tr.stackTop.(*BlockFrame)
// 		blkFrame.Traces().AddSideEffectAfter(effect)
// 		blkFrame.CoinbaseReceivedValue()
// 	}
// 	return nil
// }

// func (tr *Tracer) RegisterHashFnMapping(hashFn byte, input []byte, output []byte) {
// 	if len(input) == 0 || len(output) == 0 {
// 		return
// 	}

// 	inputBuf := make([]byte, len(input))
// 	copy(inputBuf, input)

// 	outputBuf := make([]byte, len(output))
// 	copy(outputBuf, output)

// 	// fmt.Printf("Registered preimage: %s(%#x) <- %#x\n", hashFunctionToString[HashFunction(hashFn)], outputBuf, inputBuf)

// 	tr.stackTop.PreimageSource().PutPreimage(HashFunction(hashFn), inputBuf, outputBuf)
// }

// func (tr *Tracer) pushFrame(newFrame stackFrame) stackFrame {
// 	newFrame.SetParent(tr.stackTop)
// 	tr.stackTop = newFrame
// 	return newFrame
// }

// func (tr *Tracer) mergeStackDown() error {
// 	if tr.stackTop == nil {
// 		panic("attempted to pop trace stack of completed trace")
// 	}

// 	tr.stackTop = tr.stackTop.Commit()

// 	return nil
// }

func (tr *Tracer) receiveChildTracesAtGlobalScope(childTraces Traces) {
	if err := childTraces.GetFault(); err != nil {
		// TODO: faults can reach here if tracer is used single-threaded without workers
		panic("tracer fault reached global scope")
	}

	tr.renderAndReorderInput <- childTraces
}

func (tr *Tracer) nextMessageCallSeq() uint64 {
	txFrame := tr.findFrame(reflect.TypeOf((*TransactionFrame)(nil))).(*TransactionFrame)
	if txFrame == nil {
		panic("tracer could not find enclosing TransactionFrame to allocate pid")
	}
	return txFrame.NextMessageSeq()
}

func (tr *Tracer) nextInterpreterSeq() uint64 {
	msgFrame := tr.findFrame(reflect.TypeOf((*MessageCallFrame)(nil))).(*MessageCallFrame)
	if msgFrame == nil {
		panic("tracer could not find enclosing MessageCallFrame to allocate interpreterId")
	}
	return msgFrame.NextInterpreterSeq()
}

func (tr *Tracer) coinbaseReceivedValue() {
	blockFrame := tr.findFrame(reflect.TypeOf((*BlockFrame)(nil))).(*BlockFrame)
	if blockFrame == nil {
		panic("tracer could not find enclosing BlockFrame to notify coinbase of payment")
	}
	blockFrame.CoinbaseReceivedValue()
}

func (tr *Tracer) findFrame(desiredType reflect.Type) stackFrame {
	pos := tr.stackTop
	for pos != nil {
		typeAtPos := reflect.TypeOf(pos)
		if typeAtPos == desiredType {
			return pos
		}

		pos = pos.Parent()
	}
	return nil
}

func (tr *Tracer) mergeUntilFrame(desiredType reflect.Type) stackFrame {
	fmt.Println("implement func (tr *Tracer) mergeUntilFrame(desiredType reflect.Type) stackFrame {")
	// for tr.stackTop != nil {
	// 	typeAtPos := reflect.TypeOf(tr.stackTop)
	// 	if typeAtPos == desiredType {
	// 		return tr.stackTop
	// 	}

	// 	tr.mergeStackDown()
	// }

	panic(fmt.Sprintf("could not find a %v frame in tracer stack", desiredType))
}

func (tr *Tracer) renderAndReorderLoop(ctx context.Context) {
	ord := map[uint64]Traces{}
	waitingForBlockNr := tr.fromBlockNr

	for childTraces := range tr.renderAndReorderInput {
		blockNr := childTraces.Emitter().blockNumber
		if blockNr == nil {
			panic("tracer received child traces at global scope without identity")
		}
		ord[*blockNr] = childTraces

		var (
			tracesToEmit Traces
			ok           bool
		)
		for {
			tracesToEmit, ok = ord[waitingForBlockNr]
			if !ok {
				break
			}

			tr.writer.ResetCount()

			tracesToEmit.RenderTo(tr.writer, &renderContext{})
			delete(ord, waitingForBlockNr)
			waitingForBlockNr++
		}
	}

	select {
	case <-ctx.Done():
		// tracing was cancelled; don't worry about missing blocks

	default:
		if waitingForBlockNr <= tr.toBlockNr {
			log.Warn("RLP tracer didn't receive all blocks promised")
		}
	}

	close(tr.renderAndReorderDone)
}
