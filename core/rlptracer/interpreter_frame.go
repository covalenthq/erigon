package rlptracer

import (
	libcommon "github.com/ledgerwatch/erigon-lib/common"
)

type InterpreterFrame struct {
	traces *TraceWrappedVector

	interpreterId uint64
	reverted      bool

	parent         stackFrame
	id             *frameIdentity
	preimageSource PreimageSource

	codeHash libcommon.Hash
	readOnly bool
	input    []byte
	gas      uint64

	output      []byte
	err         error
	gasRefunded uint64

	opClockSeq    uint64
	pastJumpTable bool
}

func NewInterpreterFrame(interpreterId uint64, codeHash libcommon.Hash, input []byte, gas uint64, readOnly bool) *InterpreterFrame {
	inputBuf := make([]byte, len(input))
	copy(inputBuf, input)

	newID := &frameIdentity{
		interpreterId: &interpreterId,
	}

	return &InterpreterFrame{
		id:     newID,
		traces: NewTraceWrappedVector(newID),

		interpreterId: interpreterId,

		codeHash: codeHash,
		readOnly: readOnly,
		input:    inputBuf,
		gas:      gas,
	}
}

func (frame *InterpreterFrame) SetParent(newParent stackFrame) {
	frame.parent = newParent
	frame.preimageSource = newParent.PreimageSource()

	parentID := newParent.ID()
	frame.id.blockNumber = parentID.blockNumber
	frame.id.blockSnowflake = parentID.blockSnowflake
	frame.id.txOffset = parentID.txOffset
	frame.id.msgId = parentID.msgId
}

func (frame *InterpreterFrame) ReceiveChildTraces(childTraces Traces) {
	if err := childTraces.GetFault(); err != nil {
		frame.reverted = true
	}

	frame.traces.AddChild(childTraces)
}

func (frame *InterpreterFrame) AdvanceOpClock() {
	frame.opClockSeq++
}

func (frame *InterpreterFrame) GetOpClock() uint64 {
	return frame.opClockSeq
}

func (frame *InterpreterFrame) CaptureConditionalJump() {
	if frame.pastJumpTable {
		return
	}
	var (
		payloadSig []byte
		params     []byte
	)
	if len(frame.input) >= 4 {
		payloadSig = frame.input[0:4]
		params = frame.input[4:]
	} else {
		params = frame.input
	}
	frame.traces.AddMetadataBefore(NewEffectEntrypointUsed(frame.codeHash, payloadSig, len(params)))
	frame.pastJumpTable = true
}

func (frame *InterpreterFrame) AnnotateAfter(output []byte, err error, gasRefunded uint64) error {
	frame.output = make([]byte, len(output))
	copy(frame.output, output)
	frame.err = err
	frame.gasRefunded = gasRefunded
	return nil
}

func (frame *InterpreterFrame) Commit() stackFrame {
	if !frame.pastJumpTable {
		frame.traces.AddMetadataBefore(NewEffectNoEntrypointUsed(frame.codeHash))
	}
	if frame.reverted {
		frame.traces.Fault(&opReversion{})
	}
	if frame.traces.HasContent() {
		frame.parent.ReceiveChildTraces(frame.traces)
	}
	return frame.parent
}

func (frame *InterpreterFrame) Parent() stackFrame {
	return frame.parent
}

func (frame *InterpreterFrame) ID() *frameIdentity {
	return frame.id
}

func (frame *InterpreterFrame) Traces() *TraceWrappedVector {
	return frame.traces
}

func (frame *InterpreterFrame) PreimageSource() PreimageSource {
	return frame.preimageSource
}
