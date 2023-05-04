package rlptracer

import (
	"github.com/ledgerwatch/erigon/core/vm"
)

type OpExecutionFrame struct {
	traces *TraceWrappedVector

	parent         stackFrame
	id             *frameIdentity
	preimageSource PreimageSource

	beganExecutionAtClock uint64
	depth                 int
	pc                    uint64
	op                    vm.OpCode
	cost                  uint64
	gasBefore             uint64

	gasAfter uint64

	result []byte
	err    error
}

func NewOpExecutionFrame(gasBefore uint64, op vm.OpCode) stackFrame {
	newID := &frameIdentity{}

	newFrame := &OpExecutionFrame{
		id:     newID,
		traces: NewTraceWrappedVector(newID),

		gasBefore: gasBefore,
		op:        op,
	}

	return newFrame
}

func (opExec *OpExecutionFrame) SetParent(newParent stackFrame) {
	opExec.parent = newParent
	opExec.preimageSource = newParent.PreimageSource()

	opExec.beganExecutionAtClock = newParent.(*InterpreterFrame).GetOpClock()

	parentID := newParent.ID()
	opExec.id.blockNumber = parentID.blockNumber
	opExec.id.blockSnowflake = parentID.blockSnowflake
	opExec.id.txOffset = parentID.txOffset
	opExec.id.msgId = parentID.msgId
	opExec.id.interpreterId = parentID.interpreterId
	opExec.id.opClock = &opExec.beganExecutionAtClock
}

func (opExec *OpExecutionFrame) ReceiveChildTraces(childTraces Traces) {
	// faults from enclosed frames stop here, as ops that spawn frames, like CALL,
	// translate faults in said frames to a return value for their caller

	// independently, the frame will find out whether it should fault upwards
	// in AnnotateAfter, when it receives frame.err

	opExec.traces.AddChild(childTraces)
}

func (opExec *OpExecutionFrame) AnnotateBefore(gasBefore uint64, op vm.OpCode) error {
	opExec.gasBefore = gasBefore
	opExec.op = op

	return nil
}

func (opExec *OpExecutionFrame) AnnotateAfter(pc uint64, res []byte, err error, gasAfter uint64) error {
	opExec.pc = pc
	opExec.gasAfter = gasAfter
	opExec.result = make([]byte, len(res))
	copy(opExec.result, res)
	opExec.err = err

	return nil
}

func (opExec *OpExecutionFrame) Commit() stackFrame {
	opExec.traces.AddSideEffectAfter(NewEffectOpGasSpent(opExec.op, opExec.gasBefore, opExec.gasAfter))

	if opExec.err != nil {
		opExec.traces.Fault(opExec.err)
	}
	if opExec.traces.HasContent() {
		opExec.parent.ReceiveChildTraces(opExec.traces)
	}
	return opExec.parent
}

func (opExec *OpExecutionFrame) Parent() stackFrame {
	return opExec.parent
}

func (opExec *OpExecutionFrame) ID() *frameIdentity {
	return opExec.id
}

func (opExec *OpExecutionFrame) Traces() *TraceWrappedVector {
	return opExec.traces
}

func (opExec *OpExecutionFrame) PreimageSource() PreimageSource {
	return opExec.preimageSource
}
