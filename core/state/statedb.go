// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package state provides a caching layer atop the Ethereum state trie.
package state

import (
	"bytes"

	//"errors"
	"fmt"

	"github.com/holiman/uint256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"

	// "github.com/ethereum/go-ethereum/log"

	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/crypto"

	"github.com/petar/GoLLRB/llrb"
)

// type revision struct {
// 	id           int
// 	journalIndex int
// }

type proofList [][]byte

func (n *proofList) Put(key []byte, value []byte) error {
	*n = append(*n, value)
	return nil
}

// StateDBs within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	stateReader StateReader
	// codeReader  CodeReader

	// directCodeWriter rawdb.DatabaseWriter

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateObjects      map[libcommon.Address]*stateObject
	stateObjectsDirty map[libcommon.Address]struct{}

	nilAccounts map[libcommon.Address]struct{} // Remember non-existent account to avoid reading them again

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// The refund counter, also used by state transitioning.
	refund uint64

	thash, bhash       libcommon.Hash
	blockHeight        uint64
	txIndex            int
	deleteEmptyObjects bool

	logs    map[libcommon.Hash][]*types.Log
	logSize uint

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        *journal
	validRevisions []revision
	nextRevisionId int
}

// Create a new state from a given trie
// func New(stateReader StateReader, codeReader CodeReader) *StateDB {
// 	return &StateDB{
// 		stateReader:       stateReader,
// 		codeReader:        codeReader,
// 		stateObjects:      make(map[libcommon.Address]*stateObject),
// 		stateObjectsDirty: make(map[libcommon.Address]struct{}),
// 		nilAccounts:       make(map[libcommon.Address]struct{}),
// 		logs:              make(map[libcommon.Hash][]*types.Log),
// 		journal:           newJournal(),
// 	}
// }

// setError remembers the first non-nil error it is called with.
func (self *StateDB) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *StateDB) Error() error {
	return self.dbErr
}

// Reset clears out all ephemeral state objects from the state db, but keeps
// the underlying state trie to avoid reloading data for the next operations.
func (self *StateDB) Reset() error {
	self.stateObjects = make(map[libcommon.Address]*stateObject)
	self.stateObjectsDirty = make(map[libcommon.Address]struct{})
	self.thash = libcommon.Hash{}
	self.bhash = libcommon.Hash{}
	self.blockHeight = 0
	self.txIndex = 0
	self.logs = make(map[libcommon.Hash][]*types.Log)
	self.logSize = 0
	self.clearJournalAndRefund()
	return nil
}

// func (self *StateDB) SetDirectCodeWriter(wr rawdb.DatabaseWriter) {
// 	self.directCodeWriter = wr
// }

func (self *StateDB) AddLog(log *types.Log) {
	self.journal.append(addLogChange{txhash: self.thash})

	log.TxHash = self.thash
	log.BlockHash = self.bhash
	log.TxIndex = uint(self.txIndex)
	log.Index = self.logSize
	self.logs[self.thash] = append(self.logs[self.thash], log)
	self.logSize++
}

func (self *StateDB) GetLogs(hash libcommon.Hash) []*types.Log {
	return self.logs[hash]
}

func (self *StateDB) Logs() []*types.Log {
	var logs []*types.Log
	for _, lgs := range self.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

// AddRefund adds gas to the refund counter
func (self *StateDB) AddRefund(gas uint64) {
	self.journal.append(refundChange{prev: self.refund})
	self.refund += gas
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (self *StateDB) SubRefund(gas uint64) {
	self.journal.append(refundChange{prev: self.refund})
	if gas > self.refund {
		panic("Refund counter below zero")
	}
	self.refund -= gas
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for selfdestructed accounts, until the tx is checkpointed.
// func (self *StateDB) Exist(addr libcommon.Address) bool {
// 	so := self.getStateObject(addr)
// 	result := so != nil && so.data.Initialised
// 	// fmt.Printf("StateDB.Exist addr=%s result=%b\n", hex.EncodeToString(addr[:]), result)
// 	return result
// }

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (self *StateDB) Empty(addr libcommon.Address) bool {
	so := self.getStateObject(addr)
	result := so == nil || so.empty()
	// fmt.Printf("StateDB.Empty addr=%s result=%b\n", hex.EncodeToString(addr[:]), result)
	return result
}

// Retrieve the balance from the given address or 0 if object not found
// func (self *StateDB) GetBalance(addr libcommon.Address) *uint256.Int {
// 	stateObject := self.getStateObject(addr)
// 	if stateObject != nil {
// 		return stateObject.Balance()
// 	}
// 	return common.Big0
// }

func (self *StateDB) GetNonce(addr libcommon.Address) uint64 {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

func (self *StateDB) GetCode(addr libcommon.Address) []byte {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code()
	}
	return nil
}

// func (self *StateDB) GetCodeSize(addr libcommon.Address) int {
// 	stateObject := self.getStateObject(addr)
// 	if stateObject == nil {
// 		return 0
// 	}
// 	if stateObject.code != nil {
// 		return len(stateObject.code)
// 	}
// 	len, err := self.codeReader.GetCodeSize(common.BytesToHash(stateObject.CodeHash()))
// 	if err != nil {
// 		self.setError(err)
// 	}
// 	return len
// }

func (self *StateDB) GetCodeHash(addr libcommon.Address) libcommon.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return libcommon.Hash{}
	}
	return libcommon.BytesToHash(stateObject.CodeHash())
}

// GetState retrieves a value from the given account's storage trie.
func (self *StateDB) GetState(addr libcommon.Address, hash libcommon.Hash) libcommon.Hash {
	fmt.Println("implement func (self *StateDB) GetState(addr libcommon.Address, hash libcommon.Hash) libcommon.Hash {")
	// stateObject := self.getStateObject(addr)
	// if stateObject != nil {
	// 	result := new(uint256.Int)
	// 	stateObject.GetState(hash, result)
	// 	return result
	// }
	return libcommon.Hash{}
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (self *StateDB) GetCommittedState(addr libcommon.Address, hash libcommon.Hash) libcommon.Hash {
	fmt.Println("implement func (self *StateDB) GetCommittedState(addr libcommon.Address, hash libcommon.Hash) libcommon.Hash { ")
	// stateObject := self.getStateObject(addr)
	// if stateObject != nil {
	// 	return stateObject.GetCommittedState(hash)
	// }
	return libcommon.Hash{}
}

func (self *StateDB) Hasselfdestructed(addr libcommon.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.selfdestructed
	}
	return false
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (self *StateDB) AddBalance(addr libcommon.Address, amount *uint256.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		// log.Info("AddBalance", "block", self.blockHeight, "tx", self.txIndex, "addr", addr, "amount", amount)
		stateObject.AddBalance(amount)
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (self *StateDB) SubBalance(addr libcommon.Address, amount *uint256.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		// log.Info("SubBalance", "block", self.blockHeight, "tx", self.txIndex, "addr", addr, "amount", amount)
		stateObject.SubBalance(amount)
	}
}

func (self *StateDB) SetBalance(addr libcommon.Address, amount *uint256.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount)
	}
}

func (self *StateDB) SetNonce(addr libcommon.Address, nonce uint64) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

func (self *StateDB) SetCode(addr libcommon.Address, code []byte) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetCode(crypto.Keccak256Hash(code), code)
	}
}

func (self *StateDB) SetState(addr libcommon.Address, key, value libcommon.Hash) {
	fmt.Println("implement func (self *StateDB) SetState(addr libcommon.Address, key, value libcommon.Hash) {")
	// stateObject := self.GetOrNewStateObject(addr)
	// if stateObject != nil {
	// 	stateObject.SetState(key, value)
	// }
}

// Suicide marks the given account as selfdestructed.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
// func (self *StateDB) Suicide(addr libcommon.Address) bool {
// 	stateObject := self.getStateObject(addr)
// 	if stateObject == nil {
// 		return false
// 	}
// 	self.journal.append(suicideChange{
// 		account:     &addr,
// 		prev:        stateObject.selfdestructed,
// 		prevbalance: new(big.Int).Set(stateObject.Balance()),
// 	})
// 	stateObject.markselfdestructed()
// 	stateObject.data.Balance = new(big.Int)

// 	return true
// }

// Retrieve a state object given my the address. Returns nil if not found.
func (self *StateDB) getStateObject(addr libcommon.Address) (stateObject *stateObject) {
	fmt.Println("implement func (self *StateDB) getStateObject(addr libcommon.Address) (stateObject *stateObject) {")
	if obj, ok := self.stateObjects[addr]; ok {
		return obj
	}

	account, err := self.stateReader.ReadAccountData(addr)
	if err != nil {
		self.setError(err)
		return nil
	}

	if account == nil {
		self.stateObjects[addr] = nil
		return nil
	}

	// // Insert into the live set.
	// original := *account // Copy
	// obj := newObject(self, addr, *account, original)
	// self.setStateObject(obj)
	// return obj
	return nil
}

type AccountItem struct {
	SecKey  []byte
	Balance *uint256.Int
}

func (a *AccountItem) Less(b llrb.Item) bool {
	bi := b.(*AccountItem)
	c := a.Balance.Cmp(bi.Balance)
	if c == 0 {
		return bytes.Compare(a.SecKey, bi.SecKey) < 0
	} else {
		return c < 0
	}
}

func (self *StateDB) setStateObject(object *stateObject) {
	self.stateObjects[object.Address()] = object
}

// Retrieve a state object or create a new state object if nil.
func (self *StateDB) GetOrNewStateObject(addr libcommon.Address) *stateObject {
	stateObject := self.getStateObject(addr)
	if stateObject == nil || stateObject.selfdestructed || !stateObject.data.Initialised {
		stateObject = self.createOrResetObject(addr, stateObject)
	}
	return stateObject
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (self *StateDB) createOrResetObject(addr libcommon.Address, prevObj *stateObject) (newObj *stateObject) {
	fmt.Println("implement func (self *StateDB) createOrResetObject(addr libcommon.Address, prevObj *stateObject) (newObj *stateObject) {")
	// var account accounts.Account
	// var original accounts.Account
	// if prevObj != nil {
	// 	account = prevObj.data
	// 	original = prevObj.original
	// } else {
	// 	// ensure objects, when first manipulated, attain a nonzero incarnation,
	// 	// and are marked as Initialised
	// 	account.Incarnation = 1
	// 	account.Initialised = true
	// }

	// newObj = newObject(self, addr, account, original)

	// if prevObj != nil {
	// 	newObj.Reset()
	// 	self.journal.append(resetObjectChange{prev: prevObj})
	// } else {
	// 	self.journal.append(createObjectChange{account: &addr})
	// }

	// self.setStateObject(newObj)
	// return newObj
	return nil
}

// CreateAccount explicitly creates a state object. If a state
// object with the address already exists the balance is overwritten.
func (self *StateDB) CreateAccount(addr libcommon.Address) {
	prevObj := self.getStateObject(addr)
	self.createOrResetObject(addr, prevObj)
}

// CreateAccountSubsumingPrevious explicitly creates a state object. If a state
// object with the address already exists the balance is carried over to the new account.
//
// CreateAccountSubsumingPrevious is called during the EVM CREATE operation.
// The situation might arise that a contract does the following:
//
//  1. sends funds to sha(account ++ (nonce + 1))
//  2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (self *StateDB) CreateAccountSubsumingPrevious(addr libcommon.Address) {
	fmt.Println("implement func (self *StateDB) CreateAccountSubsumingPrevious(addr libcommon.Address) {")
	// prevObj := self.getStateObject(addr)
	// newObj := self.createOrResetObject(addr, prevObj)
	// if prevObj != nil && !prevObj.selfdestructed {
	// 	newObj.setBalance(prevObj.data.Balance)
	// }
}

// Copy creates a deep, independent copy of the state.
// Snapshots of the copied state cannot be applied to the copy.
func (self *StateDB) Copy() *StateDB {
	fmt.Println("implement func (self *StateDB) Copy() *StateDB {")
	// Copy all the basic fields, initialize the memory ones
	state := &StateDB{
		stateReader:       self.stateReader,
		stateObjects:      make(map[libcommon.Address]*stateObject, len(self.journal.dirties)),
		stateObjectsDirty: make(map[libcommon.Address]struct{}, len(self.journal.dirties)),
		nilAccounts:       make(map[libcommon.Address]struct{}),
		refund:            self.refund,
		logs:              make(map[libcommon.Hash][]*types.Log, len(self.logs)),
		logSize:           self.logSize,
		journal:           newJournal(),

		thash:       self.thash,
		bhash:       self.bhash,
		blockHeight: self.blockHeight,
		txIndex:     self.txIndex,
	}
	// // Copy the dirty states and logs
	// for addr := range self.journal.dirties {
	// 	// As documented [here](https://github.com/ethereum/go-ethereum/pull/16485#issuecomment-380438527),
	// 	// and in the Finalise-method, there is a case where an object is in the journal but not
	// 	// in the stateObjects: OOG after touch on ripeMD prior to Byzantium. Thus, we need to check for
	// 	// nil
	// 	if object, exist := self.stateObjects[addr]; exist {
	// 		state.stateObjects[addr] = object.deepCopy(state)
	// 		state.stateObjectsDirty[addr] = struct{}{}
	// 	}
	// }
	// // Above, we don't copy the actual journal. This means that if the copy is copied, the
	// // loop above will be a no-op, since the copy's journal is empty.
	// // Thus, here we iterate over stateObjects, to enable copies of copies
	// for addr := range self.stateObjectsDirty {
	// 	if _, exist := state.stateObjects[addr]; !exist {
	// 		state.stateObjects[addr] = self.stateObjects[addr].deepCopy(state)
	// 		state.stateObjectsDirty[addr] = struct{}{}
	// 	}
	// }
	// for hash, logs := range self.logs {
	// 	cpy := make([]*types.Log, len(logs))
	// 	for i, l := range logs {
	// 		cpy[i] = new(types.Log)
	// 		*cpy[i] = *l
	// 	}
	// 	state.logs[hash] = cpy
	// }
	return state
}

// Snapshot returns an identifier for the current revision of the state.
func (self *StateDB) Snapshot() int {
	id := self.nextRevisionId
	self.nextRevisionId++
	self.validRevisions = append(self.validRevisions, revision{id, self.journal.length()})
	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (self *StateDB) RevertToSnapshot(revid int) {
	fmt.Println("implement func (self *StateDB) RevertToSnapshot(revid int) {")
	// // Find the snapshot in the stack of valid snapshots.
	// idx := sort.Search(len(self.validRevisions), func(i int) bool {
	// 	return self.validRevisions[i].id >= revid
	// })
	// if idx == len(self.validRevisions) || self.validRevisions[idx].id != revid {
	// 	panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	// }
	// snapshot := self.validRevisions[idx].journalIndex

	// // Replay the journal to undo changes and remove invalidated snapshots
	// self.journal.revert(self, snapshot)
	// self.validRevisions = self.validRevisions[:idx]
}

// GetRefund returns the current value of the refund counter.
func (self *StateDB) GetRefund() uint64 {
	return self.refund
}

func (s *StateDB) Checkpoint() error {
	fmt.Println("implement func (s *StateDB) Checkpoint() error {")
	// for addr := range s.journal.dirties {
	// 	stateObject, exist := s.stateObjects[addr]
	// 	if !exist {
	// 		// ripeMD is 'touched' at block 1714175, in tx 0x1237f737031e40bcde4a8b7e717b2d15e3ecadfe49bb1bbc71ee9deb09c6fcf2
	// 		// That tx goes out of gas, and although the notion of 'touched' does not exist there, the
	// 		// touch-event will still be recorded in the journal. Since ripeMD is a special snowflake,
	// 		// it will persist in the journal even though the journal is reverted. In this special circumstance,
	// 		// it may exist in `s.journal.dirties` but not in `s.stateObjects`.
	// 		// Thus, we can safely ignore it here
	// 		continue
	// 	}

	// 	if stateObject.selfdestructed || (s.deleteEmptyObjects && stateObject.empty()) {
	// 		stateObject.Reset()
	// 	}

	// 	s.stateObjectsDirty[addr] = struct{}{}
	// }

	// // log.Info("statedb/Checkpoint", "block", s.blockHeight, "tx", s.txIndex, "journal", s.journal.length(), "dirty", len(s.stateObjectsDirty))

	// // Invalidate journal because reverting across transactions is not allowed.
	// s.clearJournalAndRefund()

	return nil
}

// Finalise finalises the state by removing the self destructed objects
// and clears the journal as well as the refunds.
// func (s *StateDB) Commit(stateWriter StateWriter, codeWriter CodeWriter) error {
// 	if s.dbErr != nil {
// 		return s.dbErr
// 	}

// 	if len(s.journal.dirties) > 0 {
// 		s.Checkpoint()
// 	}

// 	for addr, stateObject := range s.stateObjects {
// 		_, isDirty := s.stateObjectsDirty[addr]
// 		if isDirty {
// 			// log.Info("statedb/Commit", "block", s.blockHeight, "tx", s.txIndex, "addr", addr, "state", "dirty")

// 			if err := stateObject.Commit(stateWriter, codeWriter); err != nil {
// 				return err
// 			}
// 		} else {
// 			// log.Info("statedb/Commit", "block", s.blockHeight, "tx", s.txIndex, "addr", addr, "state", "clean")
// 		}
// 	}

// 	s.stateObjectsDirty = make(map[libcommon.Address]struct{})

// 	// Invalidate journal because reverting across transactions is not allowed.
// 	s.clearJournalAndRefund()

// 	return nil
// }

// Prepare sets the current transaction hash and index and block hash which is
// used when the EVM emits new state logs.
func (self *StateDB) Prepare(thash, bhash libcommon.Hash, bheight uint64, ti int, deleteEmptyObjects bool) {
	self.thash = thash
	self.bhash = bhash
	self.blockHeight = bheight
	self.txIndex = ti
	self.deleteEmptyObjects = deleteEmptyObjects
	// log.Info("statedb/Prepare", "block", self.blockHeight, "tx", self.txIndex)
}

func (self *StateDB) TxIndex() int {
	return self.txIndex
}

func (s *StateDB) clearJournalAndRefund() {
	s.journal = newJournal()
	s.validRevisions = s.validRevisions[:0]
	s.refund = 0
}
