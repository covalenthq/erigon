package rlptracer

import (
	"math/big"
	"time"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
)

type MessageCallFrame struct {
	traces   *TraceWrappedVector
	reverted bool

	parent         stackFrame
	id             *frameIdentity
	preimageSource PreimageSource

	spawnedAtTime    time.Time
	terminatedAtTime time.Time

	from      libcommon.Address
	to        libcommon.Address
	isClosure bool
	payload   []byte
	output    []byte
	err       error
	gasBefore uint64
	gasAfter  uint64

	valueOffered   *big.Int
	movedRealValue bool
	toEmptyBefore  bool

	interpreterIdSeq uint64
}

func NewMessageCallFrame(msgId uint64, from libcommon.Address, to libcommon.Address, isClosure bool, gasBefore uint64, valueOffered *big.Int, payload []byte) *MessageCallFrame {
	now := time.Now()

	valueOffered = new(big.Int).Set(valueOffered)

	newID := &frameIdentity{
		msgId: &msgId,
	}

	return &MessageCallFrame{
		id:     newID,
		traces: NewTraceWrappedVector(newID),

		spawnedAtTime: now,

		from:         from,
		to:           to,
		isClosure:    isClosure,
		gasBefore:    gasBefore,
		valueOffered: valueOffered,
		payload:      payload,
	}
}

func (frame *MessageCallFrame) SetParent(newParent stackFrame) {
	frame.parent = newParent
	frame.preimageSource = newParent.PreimageSource()

	parentID := newParent.ID()
	frame.id.blockNumber = parentID.blockNumber
	frame.id.blockSnowflake = parentID.blockSnowflake
	frame.id.txOffset = parentID.txOffset
}

func (frame *MessageCallFrame) ReceiveChildTraces(childTraces Traces) {
	if err := childTraces.GetFault(); err != nil {
		frame.reverted = true
	}

	frame.traces.AddChild(childTraces)
}

func (frame *MessageCallFrame) ConsumeOfferedValue(fromBalBefore, toBalBefore *big.Int, toEmptyBefore bool) {
	if frame.valueOffered.Sign() == 0 {
		return
	}

	fromBalAfter := new(big.Int).Sub(fromBalBefore, frame.valueOffered)
	toBalAfter := new(big.Int).Add(toBalBefore, frame.valueOffered)

	frame.traces.AddSideEffectsBefore([]Effect{
		NewEffectAccountBalanceModified(frame.from, fromBalBefore, fromBalAfter),
		NewEffectAccountBalanceModified(frame.to, toBalBefore, toBalAfter),
	})

	frame.toEmptyBefore = toEmptyBefore
}

func (frame *MessageCallFrame) AnnotateAfter(output []byte, err error, gasAfter uint64, programHash libcommon.Hash) error {
	frame.terminatedAtTime = time.Now()
	frame.output = make([]byte, len(output))
	copy(frame.output, output)
	frame.gasAfter = gasAfter
	frame.err = err

	frame.traces.AddSideEffectAfter(NewEffectFrameGasSpent(frame.gasBefore, frame.gasAfter))

	if frame.err == nil {
		if frame.toEmptyBefore {
			frame.traces.AddSideEffectAfter(NewEffectAccountStorageReserved(frame.to))
		}

		if frame.isClosure {
			if len(frame.output) > 0 {
				frame.traces.AddSideEffectAfter(NewEffectAccountRanClosure(frame.to, frame.payload, frame.output))
				frame.traces.AddSideEffectAfter(NewEffectAccountBecameInternal(frame.to, programHash))
			} else {
				frame.traces.AddSideEffectAfter(NewEffectAccountBecameBlackhole(frame.to))
			}
		} else if frame.toEmptyBefore {
			frame.traces.AddSideEffectAfter(NewEffectAccountBecameBonanza(frame.to))
		}
	}

	return nil
}

func (frame *MessageCallFrame) Commit() stackFrame {
	if *(frame.id.msgId) > 0 {
		frame.traces.AddMetadataBefore(NewEffectMessageCallSpawned(frame.parent, frame))
	}

	if frame.reverted {
		frame.traces.Fault(&opReversion{})
	}

	if frame.traces.HasContent() {
		frame.parent.ReceiveChildTraces(frame.traces)
	}
	return frame.parent
}

func (frame *MessageCallFrame) Parent() stackFrame {
	return frame.parent
}

func (frame *MessageCallFrame) ID() *frameIdentity {
	return frame.id
}

func (frame *MessageCallFrame) Traces() *TraceWrappedVector {
	return frame.traces
}

func (frame *MessageCallFrame) PreimageSource() PreimageSource {
	return frame.preimageSource
}

func (frame *MessageCallFrame) NextInterpreterSeq() (res uint64) {
	res = frame.interpreterIdSeq
	frame.interpreterIdSeq++
	return
}
