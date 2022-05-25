package types

import (
	"fmt"
	"math/big"

	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon/common"
)

// const (
// 	BloomByteLength = 256
// 	BloomBitLength  = 8 * BloomByteLength
// )

type BlockSpecimen struct {
	Type            string
	NetworkId       uint64
	Hash            common.Hash
	TotalDifficulty *big.Int
	Header          *BlockSpecimenHeader
	Transactions    []*BlockSpecimenTransaction
	Uncles          []*BlockSpecimenHeader
	Receipts        []*BlockSpecimenReceipt
	Senders         []common.Address
	State           *StateSpecimen
}
type StateSpecimen struct {
	AccountRead []*accountRead
	StorageRead []*storageRead
	CodeRead    []*codeRead
}

type BlockSpecimenBlockNonce [8]byte

type BlockSpecimenBloom [BloomByteLength]byte

type BlockSpecimenHeader struct {
	ParentHash  common.Hash             `json:"parentHash"`
	UncleHash   common.Hash             `json:"sha3Uncles"`
	Coinbase    common.Address          `json:"miner"`
	Root        common.Hash             `json:"stateRoot"`
	TxHash      common.Hash             `json:"transactionsRoot"`
	ReceiptHash common.Hash             `json:"receiptsRoot"`
	Bloom       BlockSpecimenBloom      `json:"logsBloom"`
	Difficulty  *big.Int                `json:"difficulty"`
	Number      *big.Int                `json:"number"`
	GasLimit    uint64                  `json:"gasLimit"`
	GasUsed     uint64                  `json:"gasUsed"`
	Time        uint64                  `json:"timestamp"`
	Extra       []byte                  `json:"extraData"`
	MixDigest   common.Hash             `json:"mixHash"`
	Nonce       BlockSpecimenBlockNonce `json:"nonce"`
	BaseFee     *big.Int                `json:"baseFeePerGas"`
}

type BlockSpecimenTransaction struct {
	AccountNonce uint64          `json:"nonce"`
	Price        *big.Int        `json:"gasPrice"`
	GasLimit     uint64          `json:"gas"`
	Sender       common.Address  `json:"from"`
	Recipient    *common.Address `json:"to,omitempty" rlp:"nil"` // nil means contract creation
	Amount       *big.Int        `json:"value"`
	Payload      []byte          `json:"input"`
}

func FatemeUncles(uncles []*BlockSpecimenHeader) []*Header {
	var newUncles []*Header
	for _, uncle := range uncles {
		newUncle := FatemeHeader(uncle)
		newUncles = append(newUncles, newUncle)
	}
	return newUncles
}

func FatemeTx(txST *BlockSpecimenTransaction) (Transaction, error) {
	price, overflow1 := uint256.FromBig(txST.Price)
	amount, overflow2 := uint256.FromBig(txST.Amount)
	if overflow1 || overflow2 {
		return nil, fmt.Errorf("failed converting big.Int to uint256.Int")
	}
	legacyTx := NewTransaction(uint64(txST.AccountNonce), *txST.Recipient, amount, uint64(txST.GasLimit),
		price, txST.Payload) // no sender??
	return legacyTx, nil
}

func FatemeTxs(txs []*BlockSpecimenTransaction) ([]Transaction, error) {
	var transactions []Transaction
	for _, tx := range txs {
		tx2, err := FatemeTx(tx)
		if err != nil {
			return nil, fmt.Errorf("failed converting big.Int to uint256.Int")
		}
		transactions = append(transactions, tx2)
	}
	return transactions, nil
}

type BlockSpecimenLogs struct {
	Address     common.Address `json:"address"`
	Topics      []common.Hash  `json:"topics"`
	Data        []byte         `json:"data"`
	BlockNumber uint64         `json:"blockNumber"`
	TxHash      common.Hash    `json:"transactionHash"`
	TxIndex     uint           `json:"transactionIndex"`
	BlockHash   common.Hash    `json:"blockHash"`
	Index       uint           `json:"logIndex"`
	Removed     bool           `json:"removed"`
}

type BlockSpecimenReceipt struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	TxHash            common.Hash
	ContractAddress   common.Address
	Logs              []*BlockSpecimenLogs
	GasUsed           uint64
}

func FatemeReceipt(BSReceipt *BlockSpecimenReceipt) *Receipt {
	var r *Receipt
	r = new(Receipt)
	r.Type = 0
	r.PostState = BSReceipt.PostStateOrStatus
	r.Status = 0
	r.CumulativeGasUsed = BSReceipt.CumulativeGasUsed
	// r.Bloom = Bloom()
	r.Logs = FatemeLogs(BSReceipt.Logs)
	r.GasUsed = BSReceipt.GasUsed

	return r
}

func FatemeReceipts(receipts []*BlockSpecimenReceipt) []*Receipt {
	var newReceipts []*Receipt
	for _, rec := range receipts {
		var newRec *Receipt
		newRec = FatemeReceipt(rec)
		newReceipts = append(newReceipts, newRec)
	}
	return newReceipts
}

func FatemeLogs(logs []*BlockSpecimenLogs) Logs {
	var resLogs Logs
	for _, log := range logs {
		var newLog Log
		newLog.Address = log.Address
		newLog.Topics = log.Topics
		newLog.Data = log.Data
		newLog.BlockNumber = log.BlockNumber
		newLog.TxHash = log.TxHash
		newLog.TxIndex = log.TxIndex
		newLog.BlockHash = log.BlockHash
		newLog.Index = log.Index
		newLog.Removed = log.Removed
		resLogs = append(resLogs, &newLog)
	}
	return resLogs
}

type accountRead struct {
	Address  common.Address
	Nonce    uint64
	Balance  *big.Int
	CodeHash common.Hash
}

type storageRead struct {
	Account common.Address
	SlotKey common.Hash
	Value   common.Hash
}

type codeRead struct {
	Hash common.Hash
	Code []byte
}

// func (tx BlockSpecimenTransaction) Hash() common.Hash {

// 	hash := rlpHash([]interface{}{
// 		tx.AccountNonce,
// 		tx.Price,
// 		tx.GasLimit,
// 		tx.Sender,
// 		tx.Recipient,
// 		tx.Amount,
// 		tx.Payload,
// 	})
// 	return hash
// }

// func (tx BlockSpecimenTransaction) Type() byte {
// 	return blockSpecimenTxType
// }

func FatemeHeader(blockSH *BlockSpecimenHeader) *Header {
	var h *Header
	h = new(Header)
	h.ParentHash = blockSH.ParentHash
	h.UncleHash = blockSH.UncleHash
	h.Coinbase = blockSH.Coinbase
	h.Root = blockSH.Root
	h.TxHash = blockSH.TxHash
	h.ReceiptHash = blockSH.ReceiptHash
	h.Bloom = Bloom(blockSH.Bloom)
	h.Difficulty = blockSH.Difficulty
	h.Number = blockSH.Number
	h.GasLimit = blockSH.GasLimit
	h.GasUsed = blockSH.GasLimit
	h.Time = blockSH.Time
	h.Extra = blockSH.Extra
	h.MixDigest = blockSH.MixDigest
	h.Nonce = BlockNonce(blockSH.Nonce)
	h.BaseFee = blockSH.BaseFee
	h.Eip1559 = false
	h.Seal = nil
	h.WithSeal = false

	return h
}
