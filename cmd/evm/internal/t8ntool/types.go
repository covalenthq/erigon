//nolint:stylecheck,revive
package t8ntool

import (
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/core/types"
)

const (
	BloomByteLength = 256
	BloomBitLength  = 8 * BloomByteLength
)

// BlockReplica it's actually the "block-specimen" portion of the block replica. Fields
// like Receipts, senders are block-result-specific and won't actually be present in the input.
type BlockReplica struct {
	Type            string
	NetworkId       uint64
	Hash            common.Hash
	TotalDifficulty *BigInt
	Header          *Header
	Transactions    []*Transaction
	Uncles          []*Header `json:"uncles"`
	Receipts        []*Receipt
	Senders         []common.Address
	State           *StateSpecimen `json:"State"`
}
type StateSpecimen struct {
	AccountRead   []*AccountRead
	StorageRead   []*StorageRead
	CodeRead      []*CodeRead
	BlockhashRead []*BlockhashRead
}

type BlockNonce [8]byte

type Bloom [BloomByteLength]byte

type Header struct {
	ParentHash  common.Hash    `json:"parentHash"`
	UncleHash   common.Hash    `json:"sha3Uncles"`
	Coinbase    common.Address `json:"miner"`
	Root        common.Hash    `json:"stateRoot"`
	TxHash      common.Hash    `json:"transactionsRoot"`
	ReceiptHash common.Hash    `json:"receiptsRoot"`
	Bloom       Bloom          `json:"logsBloom"`
	Difficulty  *BigInt        `json:"difficulty"`
	Number      *BigInt        `json:"number"`
	GasLimit    uint64         `json:"gasLimit"`
	GasUsed     uint64         `json:"gasUsed"`
	Time        uint64         `json:"timestamp"`
	Extra       []byte         `json:"extraData"`
	MixDigest   common.Hash    `json:"mixHash"`
	Nonce       BlockNonce     `json:"nonce"`
	BaseFee     *BigInt        `json:"baseFeePerGas"`
}

type Transaction struct {
	Type         byte             `json:"type"`
	AccessList   types.AccessList `json:"accessList"`
	ChainId      *BigInt          `json:"chainId"`
	AccountNonce uint64           `json:"nonce"`
	Price        *BigInt          `json:"gasPrice"`
	GasLimit     uint64           `json:"gas"`
	GasTipCap    *BigInt          `json:"gasTipCap"`
	GasFeeCap    *BigInt          `json:"gasFeeCap"`
	Sender       *common.Address  `json:"from"`
	Recipient    *common.Address  `json:"to" rlp:"nil"` // nil means contract creation
	Amount       *BigInt          `json:"value"`
	Payload      []byte           `json:"input"`
	V            *BigInt          `json:"v"`
	R            *BigInt          `json:"r"`
	S            *BigInt          `json:"s"`
}

type Logs struct {
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

type Receipt struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	TxHash            common.Hash
	ContractAddress   common.Address
	Logs              []*Logs
	GasUsed           uint64
}

type AccountRead struct {
	Address  common.Address
	Nonce    uint64
	Balance  *BigInt
	CodeHash common.Hash
}

type StorageRead struct {
	Account common.Address
	SlotKey common.Hash
	Value   common.Hash
}

type CodeRead struct {
	Hash common.Hash
	Code []byte
}

type BlockhashRead struct {
	BlockNumber uint64
	BlockHash   common.Hash
}
