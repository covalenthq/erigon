package rlptracer

import (
	"fmt"
	"math/big"

	"github.com/ledgerwatch/erigon/core/state"
)

// import (
// 	"math/big"

// 	"github.com/ledgerwatch/erigon/core/state"
// )

var bigZero big.Int

func discoverGenesisEffects(stateAfter *state.StateDB) []Effect {
	fmt.Println("implement func discoverGenesisEffects(stateAfter *state.StateDB) []Effect {")
	var effects []Effect

	/*
		dumpIt := stateAfter.TracerDump()
		for dumpIt.Next() {
			effects = append(effects, NewEffectAccountStorageReserved(dumpIt.Address))

			if dumpIt.Balance.Sign() != 0 {
				if e := NewEffectAccountBalanceModified(dumpIt.Address, &bigZero, dumpIt.Balance); e != nil {
					effects = append(effects, e)
				}
			}

			if len(dumpIt.Code) > 0 {
				effects = append(effects, NewEffectAccountBecameInternal(dumpIt.Address, dumpIt.CodeHash))
			} else if dumpIt.Balance.Sign() != 0 {
				effects = append(effects, NewEffectAccountBecameExternal(dumpIt.Address))
			}

			for _, slot := range dumpIt.StorageSlots {
				if e := NewBlockEffectStorageModified(dumpIt.Address, slot.Location, nil, slot.Value); e != nil {
					effects = append(effects, e)
				}
			}
		}
		if dumpIt.Error != nil {
			panic(dumpIt.Error.Error())
		}
	*/

	return effects
}
