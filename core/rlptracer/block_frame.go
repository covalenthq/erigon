package rlptracer

import (
	"fmt"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
)

type BlockFrame struct {
	traces *TraceWrappedVector

	block   *types.Block
	chainID uint64

	invalid bool

	parent         stackFrame
	id             *frameIdentity
	preimageSource PreimageSource

	hasNewCoinbase bool
	coinbasePaid   bool
}

// func NewBlockFrame(block *types.Block, config *params.ChainConfig, stateBefore *state.StateDB) stackFrame {
// 	blockNr := block.NumberU64()
// 	blockSnow := block.SnowflakeID()

// 	newID := &frameIdentity{
// 		blockNumber:    &blockNr,
// 		blockSnowflake: &blockSnow,
// 	}

// 	frame := &BlockFrame{
// 		id:      newID,
// 		traces:  NewTraceWrappedVector(newID),
// 		block:   block,
// 		chainID: config.ChainID.Uint64(),
// 	}

// 	ruleChangeEffects := discoverRuleChanges(block.NumberU64(), config)
// 	frame.traces.AddSideEffectsBefore(ruleChangeEffects)

// 	frame.hasNewCoinbase = stateBefore.Empty(block.Coinbase())

// 	return frame
// }

func (frame *BlockFrame) CoinbaseReceivedValue() {
	frame.coinbasePaid = true
}

func (frame *BlockFrame) AnnotateAfter(stateAfter *state.StateDB) error {
	if *(frame.id.blockNumber) == 0 {
		genesisEffects := discoverGenesisEffects(stateAfter)
		frame.traces.AddSideEffectsAfter(genesisEffects)
	}

	if frame.hasNewCoinbase && frame.coinbasePaid {
		frame.traces.AddSideEffectsAfter([]Effect{
			NewEffectAccountStorageReserved(frame.block.Coinbase()),
			NewEffectAccountBecameExternal(frame.block.Coinbase()),
		})
	}

	return nil
}

func (frame *BlockFrame) SetParent(newParent stackFrame) {
	frame.parent = newParent
	frame.preimageSource = NewEphemeralPreimageSource(newParent.PreimageSource())
}

func (frame *BlockFrame) ReceiveChildTraces(childTraces Traces) {
	if err := childTraces.GetFault(); err != nil {
		if _, ok := err.(*transactionFailure); !ok {
			// any fault other than transactionFailure is consensus-aborting
			panic(fmt.Sprintf("blocks already in chain should never be invalid (err=%s)", err.Error()))
		}
	}

	frame.traces.AddChild(childTraces)
}

func (frame *BlockFrame) Commit() stackFrame {
	if frame.invalid {
		frame.traces.Fault(fmt.Errorf("invalid block"))
	}

	frame.traces.SetRenderContext(&renderContext{
		preimageSource: frame.preimageSource,
	})

	frame.parent.ReceiveChildTraces(frame.traces)
	return frame.parent
}

func (frame *BlockFrame) Parent() stackFrame {
	return frame.parent
}

func (frame *BlockFrame) ID() *frameIdentity {
	return frame.id
}

func (frame *BlockFrame) Traces() *TraceWrappedVector {
	return frame.traces
}

func (frame *BlockFrame) PreimageSource() PreimageSource {
	return frame.preimageSource
}

func (frame *BlockFrame) Coinbase() libcommon.Address {
	return frame.block.Coinbase()
}
