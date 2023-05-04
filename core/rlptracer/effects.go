package rlptracer

import (
	"bytes"
	"encoding/binary"
	"math/big"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/vm"
)

type EffectWithReason struct {
	e      Effect
	reason string
}

func WithReason(e Effect, reason string) *EffectWithReason {
	if e == nil {
		return nil
	}
	return &EffectWithReason{
		e:      e,
		reason: reason,
	}
}

func (ewr *EffectWithReason) Render(ctx *renderContext) EffectMsg {
	msg := ewr.e.Render(ctx)
	return append(msg, EffectMsg{"why", ewr.reason})
}

type EffectAccountStorageReserved struct {
	newAccount libcommon.Address
}

func NewEffectAccountStorageReserved(newAccount libcommon.Address) *EffectAccountStorageReserved {
	return &EffectAccountStorageReserved{
		newAccount: newAccount,
	}
}

func (e *EffectAccountStorageReserved) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"alloc", e.newAccount}
}

type EffectAccountStorageReleased struct {
	addr libcommon.Address
}

func NewEffectAccountStorageReleased(addr libcommon.Address) *EffectAccountStorageReleased {
	return &EffectAccountStorageReleased{
		addr: addr,
	}
}

func (e *EffectAccountStorageReleased) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"free", e.addr}
}

type EffectAccountNewStage struct {
	stage       state.AccountLifecycleStage
	addr        libcommon.Address
	programHash libcommon.Hash
}

func NewEffectAccountBecameEmpty(addr libcommon.Address) *EffectAccountNewStage {
	return &EffectAccountNewStage{
		stage: state.AccountStageEmpty,
		addr:  addr,
	}
}

func NewEffectAccountBecameBonanza(addr libcommon.Address) *EffectAccountNewStage {
	return &EffectAccountNewStage{
		stage: state.AccountStageBonanza,
		addr:  addr,
	}
}

func NewEffectAccountBecameExternal(addr libcommon.Address) *EffectAccountNewStage {
	return &EffectAccountNewStage{
		stage: state.AccountStageExternal,
		addr:  addr,
	}
}

func NewEffectAccountBecameInternal(addr libcommon.Address, programHash libcommon.Hash) *EffectAccountNewStage {
	return &EffectAccountNewStage{
		stage:       state.AccountStageInternal,
		addr:        addr,
		programHash: programHash,
	}
}

func NewEffectAccountBecameBlackhole(addr libcommon.Address) *EffectAccountNewStage {
	return &EffectAccountNewStage{
		stage: state.AccountStageBlackhole,
		addr:  addr,
	}
}

func (e *EffectAccountNewStage) Render(ctx *renderContext) EffectMsg {
	switch e.stage {
	case state.AccountStageEmpty:
		return EffectMsg{"type", e.addr, "0"}
	case state.AccountStageBonanza:
		return EffectMsg{"type", e.addr, "b"}
	case state.AccountStageExternal:
		return EffectMsg{"type", e.addr, "e"}
	case state.AccountStageInternal:
		return EffectMsg{"type", e.addr, "i", e.programHash}
	case state.AccountStageBlackhole:
		return EffectMsg{"type", e.addr, "x"}
	}
	panic("unknown account lifecycle stage")
}

type OpEffectLogEmitted struct {
	topics []libcommon.Hash
	data   []byte
}

func NewOpEffectLogEmitted(topics []libcommon.Hash, data []byte) *OpEffectLogEmitted {
	topicsBuf := make([]libcommon.Hash, len(topics))
	copy(topicsBuf, topics)

	dataBuf := make([]byte, len(data))
	copy(dataBuf, data)

	return &OpEffectLogEmitted{
		topics: topicsBuf,
		data:   dataBuf,
	}
}

func (e *OpEffectLogEmitted) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"log", e.topics, e.data}
}

type EffectAccountBalanceModified struct {
	addr     libcommon.Address
	oldValue *big.Int
	newValue *big.Int
}

func NewEffectAccountBalanceModified(addr libcommon.Address, oldValue *big.Int, newValue *big.Int) *EffectAccountBalanceModified {
	if oldValue.Cmp(newValue) == 0 {
		return nil
	}

	return &EffectAccountBalanceModified{
		addr:     addr,
		oldValue: new(big.Int).Set(oldValue),
		newValue: new(big.Int).Set(newValue),
	}
}

func (e *EffectAccountBalanceModified) Render(ctx *renderContext) EffectMsg {
	// FIX: cannot emit negative numbers in RLP
	// delta := new(big.Int).Sub(e.newValue, e.oldValue)
	return EffectMsg{"balance", e.addr, e.oldValue, e.newValue}
}

type EffectStorageRead struct {
	addr  []byte
	key   []byte
	value []byte
}

func NewEffectStorageRead(acct libcommon.Address, loc libcommon.Hash, value libcommon.Hash) *EffectStorageRead {
	return &EffectStorageRead{
		addr:  acct.Bytes(),
		key:   loc.Bytes(),
		value: value.Bytes(),
	}
}

func (e *EffectStorageRead) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"sload", e.addr, e.key, e.value}
}

type EffectStorageModified struct {
	fromOp   bool
	addr     []byte
	key      []byte
	oldValue []byte
	newValue []byte
}

func NewBlockEffectStorageModified(addr libcommon.Address, loc []byte, oldBytes []byte, newBytes []byte) *EffectStorageModified {
	if bytes.Equal(oldBytes, newBytes) {
		return nil
	}

	return &EffectStorageModified{
		addr:     addr.Bytes(),
		key:      loc,
		oldValue: oldBytes,
		newValue: newBytes,
	}
}

func NewOpEffectStorageModified(acct libcommon.Address, loc libcommon.Hash, oldValue libcommon.Hash, newValue libcommon.Hash) *EffectStorageModified {
	oldBytes := oldValue.Bytes()
	newBytes := newValue.Bytes()

	if bytes.Equal(oldBytes, newBytes) {
		return nil
	}

	return &EffectStorageModified{
		fromOp:   true,
		addr:     acct.Bytes(),
		key:      loc.Bytes(),
		oldValue: oldBytes,
		newValue: newBytes,
	}
}

func (e *EffectStorageModified) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"sstore", e.addr, e.key, e.oldValue, e.newValue}
}

type EffectMessageCallSpawned struct {
	enclosingID *frameIdentity
	newID       *frameIdentity
}

func NewEffectMessageCallSpawned(parent stackFrame, child *MessageCallFrame) *EffectMessageCallSpawned {
	return &EffectMessageCallSpawned{
		enclosingID: parent.ID(),
		newID:       child.id,
	}
}

func (e *EffectMessageCallSpawned) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"spawn", *(e.enclosingID.msgId), *(e.newID.msgId)}
}

func (e *EffectMessageCallSpawned) Emitter() *frameIdentity {
	return e.enclosingID
}

type EffectAccountSentMessage struct {
	sender    libcommon.Address
	recipient libcommon.Address
	data      []byte
}

func NewEffectAccountSentMessage(sender libcommon.Address, recipient libcommon.Address, data []byte) *EffectAccountSentMessage {
	return &EffectAccountSentMessage{
		sender:    sender,
		recipient: recipient,
		data:      common.CopyBytes(data),
	}
}

func (e *EffectAccountSentMessage) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"push", e.sender, e.recipient, e.data}
}

type EffectAccountRanClosure struct {
	addr libcommon.Address
	code []byte
}

func NewEffectAccountRanClosure(addr libcommon.Address, code []byte, output []byte) *EffectAccountRanClosure {
	return &EffectAccountRanClosure{
		addr: addr,
		code: common.CopyBytes(code),
	}
}

func (e *EffectAccountRanClosure) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"ranClosure", e.addr, e.code}
}

type EffectEntrypointUsed struct {
	programHash   libcommon.Hash
	mangledSymbol []byte
	payloadLen    int
}

func NewEffectEntrypointUsed(programHash libcommon.Hash, mangledSymbol []byte, payloadLen int) *EffectEntrypointUsed {
	return &EffectEntrypointUsed{
		programHash:   programHash,
		mangledSymbol: mangledSymbol,
		payloadLen:    payloadLen,
	}
}

func (e *EffectEntrypointUsed) Render(ctx *renderContext) EffectMsg {
	symbolPart := EffectMsg{}
	if len(e.mangledSymbol) == 4 {
		imageComposite := make([]byte, 8)
		copy(imageComposite[0:4], e.mangledSymbol)
		binary.BigEndian.PutUint32(imageComposite[4:8], uint32(e.payloadLen))

		if demangledSymbol, ok := ctx.preimageSource.GetPreimage(hashFnKeccak256Trunc4, imageComposite); ok {
			symbolPart = EffectMsg{"t", demangledSymbol}
		} else {
			symbolPart = EffectMsg{"4", e.mangledSymbol}
		}
	}
	return EffectMsg{"asExtCall", e.programHash, symbolPart, uint64(e.payloadLen)}
}

type EffectNoEntrypointUsed struct {
	programHash libcommon.Hash
}

func NewEffectNoEntrypointUsed(programHash libcommon.Hash) *EffectNoEntrypointUsed {
	return &EffectNoEntrypointUsed{
		programHash: programHash,
	}
}

func (e *EffectNoEntrypointUsed) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"asASM", e.programHash}
}

type EffectValueTransferred struct {
	fromAccount libcommon.Address
	toAccount   libcommon.Address
	value       *big.Int
}

func (e *EffectValueTransferred) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"send", e.fromAccount, e.toAccount, e.value}
}

func NewEffectOpGasSpent(op vm.OpCode, gasBefore uint64, gasAfter uint64) *EffectOpGasSpent {
	return &EffectOpGasSpent{
		op:        op,
		gasBefore: gasBefore,
		gasAfter:  gasAfter,
	}
}

type EffectOpGasSpent struct {
	op        vm.OpCode
	gasBefore uint64
	gasAfter  uint64
}

func (e *EffectOpGasSpent) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"gasop", e.op.String(), e.gasBefore, e.gasAfter}
}

func NewEffectFrameGasSpent(gasBefore uint64, gasAfter uint64) *EffectFrameGasSpent {
	return &EffectFrameGasSpent{
		gasBefore: gasBefore,
		gasAfter:  gasAfter,
	}
}

type EffectFrameGasSpent struct {
	gasBefore uint64
	gasAfter  uint64
}

func (e *EffectFrameGasSpent) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"gasf", e.gasBefore, e.gasAfter}
}
