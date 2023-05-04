package rlptracer

import (
	"fmt"
	"math/big"

	"github.com/ledgerwatch/erigon-lib/chain"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
)

type forkPoint struct {
	pos        *big.Int
	ruleChange Effect
}

func discoverRuleChanges(blockNr uint64, config *chain.Config) []Effect {
	fmt.Println("implement func discoverRuleChanges(blockNr uint64, config *chain.Config) []Effect {")
	// var effects []Effect

	// forkPoints := []forkPoint{
	// 	forkPoint{config.HomesteadBlock, &EffectHomesteadRuleChange{}},
	// 	forkPoint{config.DAOForkBlock, &EffectDAOForkRuleChange{support: config.DAOForkSupport}},
	// 	forkPoint{config.EIP150Block, &EffectEIP150RuleChange{hash: config.EIP150Hash}},
	// 	forkPoint{config.EIP155Block, &EffectEIP155RuleChange{}},
	// 	forkPoint{config.EIP158Block, &EffectEIP158RuleChange{}},
	// 	forkPoint{config.ByzantiumBlock, &EffectByzantiumRuleChange{}},
	// 	forkPoint{config.ConstantinopleBlock, &EffectConstantinopleRuleChange{}},
	// 	forkPoint{config.PetersburgBlock, &EffectPetersburgRuleChange{}},
	// 	forkPoint{config.EWASMBlock, &EffectEWASMRuleChange{}},
	// }

	// for _, forkPoint := range forkPoints {
	// 	if forkPoint.pos == nil {
	// 		continue
	// 	}

	// 	if blockNr == forkPoint.pos.Uint64() {
	// 		effects = append(effects, forkPoint.ruleChange)
	// 	}
	// }

	// return effects
	return nil
}

type EffectHomesteadRuleChange struct{}

func (e *EffectHomesteadRuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"homestead"}
}

type EffectDAOForkRuleChange struct {
	support bool
}

func (e *EffectDAOForkRuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"DAOFork", e.support}
}

type EffectEIP150RuleChange struct {
	hash libcommon.Hash
}

func (e *EffectEIP150RuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"EIP150", e.hash}
}

type EffectEIP155RuleChange struct{}

func (e *EffectEIP155RuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"EIP155"}
}

type EffectEIP158RuleChange struct{}

func (e *EffectEIP158RuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"EIP158"}
}

type EffectByzantiumRuleChange struct{}

func (e *EffectByzantiumRuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"Byzantium"}
}

type EffectConstantinopleRuleChange struct{}

func (e *EffectConstantinopleRuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"Constantinople"}
}

type EffectPetersburgRuleChange struct{}

func (e *EffectPetersburgRuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"Petersburg"}
}

type EffectEWASMRuleChange struct{}

func (e *EffectEWASMRuleChange) Render(ctx *renderContext) EffectMsg {
	return EffectMsg{"EWASM"}
}
