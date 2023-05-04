package state

import (
	"fmt"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
)

type AccountLifecycleStage byte

const (
	AccountStageEmpty AccountLifecycleStage = iota
	AccountStageBonanza
	AccountStageExternal
	AccountStageInternal
	AccountStageBlackhole
)

func (self *StateDB) GetLifecycleStage(addr libcommon.Address) AccountLifecycleStage {
	so := self.getStateObject(addr)
	if so == nil {
		return AccountStageEmpty
	}
	return so.lifecycleStage()
}

func (s *stateObject) lifecycleStage() AccountLifecycleStage {
	fmt.Println("implement func (s *stateObject) lifecycleStage() AccountLifecycleStage { ")
	// // TODO: detect blackholed accounts

	// if !bytes.Equal(s.data.CodeHash, EmptyCodeHash) {
	// 	return AccountStageInternal
	// }
	// if s.data.Nonce != 0 {
	// 	return AccountStageExternal
	// }
	// if s.data.Balance.Sign() != 0 {
	// 	return AccountStageBonanza
	// }
	return AccountStageEmpty

}

type AccountPopulatedFields byte

const (
	NoncePopulated AccountPopulatedFields = 1 << iota
	BalancePopulated
	CodePopulated
	IncarnationPopulated
)

// func (self *StateDB) GetPopulatedFields(addr libcommon.Address) AccountPopulatedFields {
// 	so := self.getStateObject(addr)
// 	if so == nil {
// 		return 0
// 	}
// 	return so.populatedFields()
// }

// func (s *stateObject) populatedFields() (pop AccountPopulatedFields) {
// 	if s.data.Incarnation != 0 {
// 		pop |= IncarnationPopulated
// 	}
// 	if s.data.Nonce != 0 {
// 		pop |= NoncePopulated
// 	}
// 	if s.data.Balance.Sign() != 0 {
// 		pop |= BalancePopulated
// 	}
// 	if !bytes.Equal(s.data.CodeHash, EmptyCodeHash) {
// 		pop |= CodePopulated
// 	}
// 	return
// }

// func (self *StateDB) CreateHoldOnBalance(addr libcommon.Address, holdAmount *big.Int) {
// 	if so := self.getStateObject(addr); so != nil {
// 		so.createHold(holdAmount)
// 	}
// }

// func (self *StateDB) ChargeHoldsToBalance(addr libcommon.Address) {
// 	if so := self.getStateObject(addr); so != nil {
// 		so.chargeHold()
// 	}
// }

// func (self *StateDB) ReleaseHolds(addr libcommon.Address) {
// 	if so := self.getStateObject(addr); so != nil {
// 		so.releaseHold()
// 	}
// }

// func (self *StateDB) GetHolds(addr libcommon.Address) *big.Int {
// 	if so := self.getStateObject(addr); so != nil {
// 		return so.holds()
// 	} else {
// 		return common.Big0
// 	}
// }

// func (self *StateDB) GetBalancePlusHolds(addr libcommon.Address) *big.Int {
// 	if so := self.getStateObject(addr); so != nil {
// 		return so.balancePlusHolds()
// 	} else {
// 		return common.Big0
// 	}
// }

// func (s *stateObject) holds() *big.Int {
// 	if s.heldBalance == nil {
// 		return common.Big0
// 	}
// 	return s.heldBalance
// }

// func (s *stateObject) balancePlusHolds() *big.Int {
// 	if s.heldBalance == nil {
// 		return s.data.Balance
// 	}
// 	return new(big.Int).Add(s.data.Balance, s.heldBalance)
// }

// func (s *stateObject) createHold(amount *big.Int) {
// 	if s.heldBalance == nil {
// 		s.heldBalance = new(big.Int)
// 	}
// 	s.heldBalance.Add(s.heldBalance, amount)
// 	s.SubBalance(amount)
// }

// func (s *stateObject) chargeHold() {
// 	s.heldBalance = nil
// }

// func (s *stateObject) releaseHold() {
// 	if s.heldBalance != nil {
// 		s.AddBalance(s.heldBalance)
// 		s.heldBalance = nil
// 	}
// }

// type BlockEventLogger interface{
// 	CaptureBlockUncleReward(coinbase libcommon.Address, balBefore, balAfter *big.Int) error
// 	CaptureBlockMinerReward(coinbase libcommon.Address, balBefore, balAfter *big.Int) error
// 	CaptureTxSenderBuyGas(origin libcommon.Address, oldBalance *big.Int, newBalance *big.Int) error
// 	CaptureTxComputerReward(coinbase libcommon.Address, oldBalance *big.Int, newBalance *big.Int) error
// }

// type StateDBTracing struct {
// 	*StateDB
// 	eventLogger    BlockEventLogger
// }

// func (self *StateDB) TracingExtensions(eventLogger BlockEventLogger) *StateDBTracing {
// 	return &StateDBTracing{
// 		StateDB: self,
// 		eventLogger: eventLogger,
// 	}
// }

// func (self *StateDB) AddBlockUncleBalance(coinbase libcommon.Address, amount *big.Int) {
// 	self.AddBalance(coinbase, amount)
// }

// func (self *StateDB) AddBlockMinerBalance(coinbase libcommon.Address, amount *big.Int) {
// 	self.AddBalance(coinbase, amount)
// }

// func (self *StateDBTracing) AddBlockUncleBalance(coinbase libcommon.Address, amount *big.Int) {
// 	balBefore := self.GetBalance(coinbase)
// 	self.AddBalance(coinbase, amount)
// 	balAfter := self.GetBalance(coinbase)
// 	self.eventLogger.CaptureBlockUncleReward(coinbase, balBefore, balAfter)
// }

// func (self *StateDBTracing) AddBlockMinerBalance(coinbase libcommon.Address, amount *big.Int) {
// 	balBefore := self.GetBalance(coinbase)
// 	self.AddBalance(coinbase, amount)
// 	balAfter := self.GetBalance(coinbase)
// 	self.eventLogger.CaptureBlockMinerReward(coinbase, balBefore, balAfter)
// }

// func (self *StateDB) BuyGasFinal(origin libcommon.Address, computer libcommon.Address, amount *big.Int) {
// 	self.ReleaseHolds(origin)
// 	self.SubBalance(origin, amount)
// 	self.AddBalance(computer, amount)
// }

// func (self *StateDBTracing) BuyGasFinal(buyer libcommon.Address, seller libcommon.Address, realAmount *big.Int) {
// 	buyerBalBefore := self.GetBalancePlusHolds(buyer)
// 	self.ReleaseHolds(buyer)
// 	self.SubBalance(buyer, realAmount)
// 	buyerBalAfter := self.GetBalance(buyer)

// 	chargedAmount := new(big.Int).Sub(buyerBalBefore, buyerBalAfter)

// 	sellerBalBefore := self.GetBalance(seller)
// 	self.AddBalance(seller, chargedAmount)
// 	sellerBalAfter := self.GetBalance(seller)

// 	self.eventLogger.CaptureTxSenderBuyGas(buyer, buyerBalBefore, buyerBalAfter)
// 	self.eventLogger.CaptureTxComputerReward(seller, sellerBalBefore, sellerBalAfter)
// }

// func (self *StateDB) BuyAllGasFinal(origin libcommon.Address, computer libcommon.Address) {
// 	held := self.GetHolds(origin)
// 	self.ChargeHoldsToBalance(origin)
// 	self.AddBalance(computer, held)
// }

// func (self *StateDBTracing) BuyAllGasFinal(buyer libcommon.Address, seller libcommon.Address) {
// 	buyerBalBefore := self.GetBalancePlusHolds(buyer)
// 	self.ChargeHoldsToBalance(buyer)
// 	buyerBalAfter := self.GetBalance(buyer)

// 	heldAmount := new(big.Int).Sub(buyerBalBefore, buyerBalAfter)

// 	sellerBalBefore := self.GetBalance(seller)
// 	self.AddBalance(seller, heldAmount)
// 	sellerBalAfter := self.GetBalance(seller)

// 	self.eventLogger.CaptureTxSenderBuyGas(buyer, buyerBalBefore, buyerBalAfter)
// 	self.eventLogger.CaptureTxComputerReward(seller, sellerBalBefore, sellerBalAfter)
// }
