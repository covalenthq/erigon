package rlptracer

type globalProxyFrame struct {
	tracer *Tracer
}

func (proxy *globalProxyFrame) SetTracer(tr *Tracer) {
	proxy.tracer = tr
}

func (proxy *globalProxyFrame) ReceiveChildTraces(childTraces Traces) {
	proxy.tracer.receiveChildTracesAtGlobalScope(childTraces)
}

func (proxy *globalProxyFrame) SetParent(newParent stackFrame) {
	panic("tracer attempted to give globalProxyFrame a parent")
}

func (proxy *globalProxyFrame) Parent() stackFrame {
	return nil
}

func (proxy *globalProxyFrame) Commit() stackFrame {
	return nil
}

func (proxy *globalProxyFrame) ID() *frameIdentity {
	panic("tracer attempted to get ID of global scope")
}

func (proxy *globalProxyFrame) Traces() *TraceWrappedVector {
	panic("tracer attempted to get Traces of global scope")
}

func (proxy *globalProxyFrame) PreimageSource() PreimageSource {
	return proxy.tracer.preimageSource
}
