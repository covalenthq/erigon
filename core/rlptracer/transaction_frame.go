package rlptracer

import (
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/core"
)

type TransactionFrame struct {
	traces *TraceWrappedVector

	offsetWithinBlock int
	originMsg         core.Message

	parent         stackFrame
	id             *frameIdentity
	preimageSource PreimageSource

	returnData       []byte
	usedGas          uint64
	executionFailed  bool
	consensusErr     error
	consensusAborted bool

	msgIdSeq uint64
}

func NewTransactionFrame(offsetWithinBlock int, originMsg core.Message) stackFrame {
	txOffset := uint64(offsetWithinBlock)

	newID := &frameIdentity{
		txOffset: &txOffset,
	}

	return &TransactionFrame{
		id:                newID,
		traces:            NewTraceWrappedVector(newID),
		offsetWithinBlock: offsetWithinBlock,
		originMsg:         originMsg,
	}
}

func (frame *TransactionFrame) SetParent(newParent stackFrame) {
	frame.parent = newParent
	frame.preimageSource = newParent.PreimageSource()

	parentID := newParent.ID()
	frame.id.blockNumber = parentID.blockNumber
	frame.id.blockSnowflake = parentID.blockSnowflake
}

func (frame *TransactionFrame) ReceiveChildTraces(childTraces Traces) {
	// execution failures don't cause the tx itself to fault;
	// a tx can "succeed with an error result."

	// frame.consensusAborted is only set (in AnnotateAfter) by the interpreter,
	// if it decides that the error emitted was a consensus error.

	// txs that abort consensus create invalid blocks, so we should never see
	// these in an imported chain.

	frame.traces.AddChild(childTraces)
}

func (frame *TransactionFrame) AnnotateAfter(ret []byte, usedGas uint64, executionFailed bool, consensusErr error) error {
	// TODO: dump receipt
	frame.returnData = make([]byte, len(ret))
	copy(frame.returnData, ret)
	frame.usedGas = usedGas
	frame.executionFailed = executionFailed
	frame.consensusErr = consensusErr
	frame.consensusAborted = (consensusErr != nil)
	return nil
}

func (frame *TransactionFrame) Commit() stackFrame {
	if frame.consensusAborted {
		frame.traces.Fault(frame.consensusErr)
	} else if frame.executionFailed {
		frame.traces.FaultRetainingOwnSideEffects(&transactionFailure{})
	}

	frame.parent.ReceiveChildTraces(frame.traces)
	return frame.parent
}

func (frame *TransactionFrame) Parent() stackFrame {
	return frame.parent
}

func (frame *TransactionFrame) ID() *frameIdentity {
	return frame.id
}

func (frame *TransactionFrame) Traces() *TraceWrappedVector {
	return frame.traces
}

func (frame *TransactionFrame) PreimageSource() PreimageSource {
	return frame.preimageSource
}

func (frame *TransactionFrame) Coinbase() libcommon.Address {
	return frame.parent.(*BlockFrame).Coinbase()
}

func (frame *TransactionFrame) NextMessageSeq() (res uint64) {
	res = frame.msgIdSeq
	frame.msgIdSeq++
	return
}
