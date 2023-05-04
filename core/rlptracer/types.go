package rlptracer

import "io"

type stackFrame interface {
	ReceiveChildTraces(childTraces Traces)
	Parent() stackFrame
	SetParent(stackFrame)
	Commit() stackFrame

	ID() *frameIdentity
	Traces() *TraceWrappedVector
	PreimageSource() PreimageSource
}

type renderContext struct {
	preimageSource PreimageSource
}

type EffectMsg []interface{}

type Effect interface {
	Render(*renderContext) EffectMsg
}

type EffectWithEmitter interface {
	Effect
	Emitter() *frameIdentity
}

type frameIdentity struct {
	blockNumber    *uint64
	blockSnowflake *uint64
	txOffset       *uint64
	msgId          *uint64
	interpreterId  *uint64
	opClock        *uint64
}

func (orig *frameIdentity) Copy() *frameIdentity {
	newFrame := *orig
	return &newFrame
}

func (id *frameIdentity) Render(*renderContext) EffectMsg {
	out := []interface{}{
		id.blockSnowflake,
		id.txOffset,
		id.msgId,
		id.interpreterId,
		id.opClock,
	}

	switch {
	case id.blockSnowflake == nil:
		return out[:0]
	case id.txOffset == nil:
		return out[:1]
	case id.msgId == nil:
		return out[:2]
	case id.interpreterId == nil:
		return out[:3]
	case id.opClock == nil:
		return out[:4]
	default:
		return out[:]
	}
}

type transactionFailure struct{}

func (e *transactionFailure) Error() string {
	return "failed"
}

type opReversion struct{}

func (e *opReversion) Error() string {
	return "reverted"
}

type countingWriter struct {
	writer      io.Writer
	TimesCalled uint64
}

func (cw *countingWriter) ResetCount() {
	cw.TimesCalled = 0
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
	cw.TimesCalled++
	return cw.writer.Write(p)
}
