package rlptracer

import (
	"io"

	"github.com/ledgerwatch/erigon/rlp"
)

type Traces interface {
	Emitter() *frameIdentity
	HasContent() bool
	GetFault() error
	RenderTo(io.Writer, *renderContext) (int64, error)
}

type TraceNullLeaf struct{}

func (leaf *TraceNullLeaf) Emitter() *frameIdentity {
	return nil
}

func (leaf *TraceNullLeaf) HasContent() bool {
	return false
}

func (leaf *TraceNullLeaf) GetFault() error {
	return nil
}

func (leaf *TraceNullLeaf) RenderTo(w io.Writer, rndrCtx *renderContext) (n int64, err error) {
	return
}

type TraceSingleLeaf struct {
	emitter *frameIdentity
	effect  Effect
}

func NewTraceSingleLeaf(id *frameIdentity, e Effect) *TraceSingleLeaf {
	return &TraceSingleLeaf{
		emitter: id,
		effect:  e,
	}
}

func (leaf *TraceSingleLeaf) Emitter() *frameIdentity {
	return leaf.emitter
}

func (leaf *TraceSingleLeaf) HasContent() bool {
	return true
}

func (leaf *TraceSingleLeaf) GetFault() error {
	return nil
}

func (leaf *TraceSingleLeaf) RenderTo(w io.Writer, rndrCtx *renderContext) (n int64, err error) {
	if leaf.effect == nil {
		return
	}

	var (
		event  EffectMsg
		eventB []byte
		wroteN int
	)

	emitter := leaf.emitter
	if ewe, ok := leaf.effect.(EffectWithEmitter); ok {
		emitter = ewe.Emitter()
	}

	var emitSeq uint64
	if cw, ok := w.(*countingWriter); ok {
		emitSeq = cw.TimesCalled
	}

	event = append([]interface{}{"effect", emitter.Render(rndrCtx), emitSeq}, leaf.effect.Render(rndrCtx)...)
	eventB, err = rlp.EncodeToBytes(event)
	if err != nil {
		return
	}
	wroteN, err = w.Write(eventB)
	n = int64(wroteN)
	return
}

type TraceFault struct {
	emitter *frameIdentity
	err     error
}

func NewTraceFault(id *frameIdentity, err error) *TraceFault {
	return &TraceFault{
		emitter: id,
		err:     err,
	}
}

func (fault *TraceFault) Emitter() *frameIdentity {
	return fault.emitter
}

func (leaf *TraceFault) HasContent() bool {
	return true
}

func (fault *TraceFault) GetFault() error {
	return fault.err
}

func (fault *TraceFault) RenderTo(w io.Writer, rndrCtx *renderContext) (n int64, err error) {
	if fault.err == nil {
		return
	}

	var (
		event  EffectMsg
		eventB []byte
		wroteN int
	)

	var emitSeq uint64
	if cw, ok := w.(*countingWriter); ok {
		emitSeq = cw.TimesCalled
	}

	event = []interface{}{"fault", fault.emitter.Render(rndrCtx), emitSeq, fault.err.Error()}
	eventB, err = rlp.EncodeToBytes(event)
	if err != nil {
		return
	}
	wroteN, err = w.Write(eventB)
	n = int64(wroteN)
	return
}

type TraceMultiLeaf struct {
	emitter *frameIdentity
	effects []Effect
}

func NewTraceMultiLeaf(id *frameIdentity, es []Effect) *TraceMultiLeaf {
	return &TraceMultiLeaf{
		emitter: id,
		effects: es,
	}
}

func (leaf *TraceMultiLeaf) Emitter() *frameIdentity {
	return leaf.emitter
}

func (leaf *TraceMultiLeaf) HasContent() bool {
	return len(leaf.effects) > 0
}

func (leaf *TraceMultiLeaf) GetFault() error {
	return nil
}

func (leaf *TraceMultiLeaf) Append(e Effect) {
	leaf.effects = append(leaf.effects, e)
}

func (leaf *TraceMultiLeaf) AppendMany(es []Effect) {
	leaf.effects = append(leaf.effects, es...)
}

func (leaf *TraceMultiLeaf) RenderTo(w io.Writer, rndrCtx *renderContext) (n int64, err error) {
	var (
		event   EffectMsg
		eventB  []byte
		wroteN  int
		emitter *frameIdentity
		emitSeq uint64
	)

	for _, e := range leaf.effects {
		if ewe, ok := e.(EffectWithEmitter); ok {
			emitter = ewe.Emitter()
		} else {
			emitter = leaf.emitter
		}

		if cw, ok := w.(*countingWriter); ok {
			emitSeq = cw.TimesCalled
		}

		event = append([]interface{}{"effect", emitter.Render(rndrCtx), emitSeq}, e.Render(rndrCtx)...)
		eventB, err = rlp.EncodeToBytes(event)
		if err != nil {
			return
		}
		wroteN, err = w.Write(eventB)
		n += int64(wroteN)
		if err != nil {
			return
		}
	}

	return
}

type TraceVector struct {
	elements []Traces
}

func NewTraceVector() *TraceVector {
	return &TraceVector{}
}

func (vector *TraceVector) Emitter() *frameIdentity {
	if len(vector.elements) == 1 {
		return vector.elements[0].Emitter()
	}
	return nil
}

func (vector *TraceVector) HasContent() bool {
	return len(vector.elements) > 0
}

func (vector *TraceVector) GetFault() error {
	// for _, childTraces := range(vector.elements) {
	//   if err = childTraces.Faulted(); err != nil {
	//     return
	//   }
	// }
	return nil
}

func (vector *TraceVector) Append(el Traces) {
	vector.elements = append(vector.elements, el)
}

func (vector *TraceVector) RenderTo(w io.Writer, rndrCtx *renderContext) (n int64, err error) {
	var wroteN int64

	for _, el := range vector.elements {
		wroteN, err = el.RenderTo(w, rndrCtx)
		n += wroteN
		if err != nil {
			return
		}
	}

	return
}

type TraceWrappedVector struct {
	emitter       *frameIdentity
	renderContext *renderContext

	header         *TraceMultiLeaf
	bodyBeforePart *TraceMultiLeaf
	body           Traces
	bodyAfterPart  *TraceMultiLeaf
	footer         *TraceMultiLeaf
}

func NewTraceWrappedVector(identity *frameIdentity) *TraceWrappedVector {
	return &TraceWrappedVector{
		emitter:        identity,
		header:         NewTraceMultiLeaf(identity, nil),
		bodyBeforePart: NewTraceMultiLeaf(identity, nil),
		body:           NewTraceVector(),
		bodyAfterPart:  NewTraceMultiLeaf(identity, nil),
		footer:         NewTraceMultiLeaf(identity, nil),
	}
}

func (wrapper *TraceWrappedVector) Emitter() *frameIdentity {
	return wrapper.emitter
}

func (wrapper *TraceWrappedVector) HasContent() bool {
	if wrapper.bodyBeforePart != nil && wrapper.bodyBeforePart.HasContent() {
		return true
	}
	if wrapper.body.HasContent() {
		return true
	}
	return wrapper.bodyAfterPart != nil && wrapper.bodyAfterPart.HasContent()
}

func (wrapper *TraceWrappedVector) GetFault() error {
	return wrapper.body.GetFault()
}

func (wrapper *TraceWrappedVector) AddMetadataBefore(e Effect) {
	wrapper.header.Append(e)
}

func (wrapper *TraceWrappedVector) AddMetadataAfter(e Effect) {
	wrapper.footer.Append(e)
}

func (wrapper *TraceWrappedVector) AddSideEffectBefore(e Effect) {
	if wrapper.bodyBeforePart != nil {
		wrapper.bodyBeforePart.Append(e)
	}
}

func (wrapper *TraceWrappedVector) AddSideEffectsBefore(es []Effect) {
	if wrapper.bodyBeforePart != nil {
		wrapper.bodyBeforePart.AppendMany(es)
	}
}

func (wrapper *TraceWrappedVector) AddSideEffectAfter(e Effect) {
	if wrapper.bodyAfterPart != nil {
		wrapper.bodyAfterPart.Append(e)
	}
}

func (wrapper *TraceWrappedVector) AddSideEffectsAfter(es []Effect) {
	if wrapper.bodyAfterPart != nil {
		wrapper.bodyAfterPart.AppendMany(es)
	}
}

func (wrapper *TraceWrappedVector) AddChild(childTraces Traces) {
	if vector, ok := wrapper.body.(*TraceVector); ok {
		vector.Append(childTraces)
	}
}

func (wrapper *TraceWrappedVector) DropChildren() {
	wrapper.body = &TraceNullLeaf{}
}

func (wrapper *TraceWrappedVector) Fault(err error) {
	wrapper.bodyBeforePart = nil
	wrapper.body = NewTraceFault(wrapper.emitter, err)
	wrapper.bodyAfterPart = nil
}

func (wrapper *TraceWrappedVector) FaultRetainingOwnSideEffects(err error) {
	wrapper.body = NewTraceFault(wrapper.emitter, err)
}

func (wrapper *TraceWrappedVector) SetRenderContext(ctx *renderContext) {
	wrapper.renderContext = ctx
}

func (wrapper *TraceWrappedVector) RenderTo(w io.Writer, parentRndrCtx *renderContext) (n int64, err error) {
	var wroteN int64

	rndrCtx := parentRndrCtx
	if wrapper.renderContext != nil {
		rndrCtx = wrapper.renderContext
	}

	wroteN, err = wrapper.header.RenderTo(w, rndrCtx)
	n += wroteN
	if err != nil {
		return
	}

	if wrapper.bodyBeforePart != nil {
		wroteN, err = wrapper.bodyBeforePart.RenderTo(w, rndrCtx)
		n += wroteN
		if err != nil {
			return
		}
	}

	wroteN, err = wrapper.body.RenderTo(w, rndrCtx)
	n += wroteN
	if err != nil {
		return
	}

	if wrapper.bodyAfterPart != nil {
		wroteN, err = wrapper.bodyAfterPart.RenderTo(w, rndrCtx)
		n += wroteN
		if err != nil {
			return
		}
	}

	wroteN, err = wrapper.footer.RenderTo(w, rndrCtx)
	n += wroteN
	return
}
