//nolint:stylecheck,revive
package t8ntool

import (
	"fmt"
	"math/big"

	"github.com/holiman/uint256"
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

//type BlockNonce [8]byte

//type Bloom [BloomByteLength]byte

type Header struct {
	ParentHash  common.Hash      `json:"parentHash"`
	UncleHash   common.Hash      `json:"sha3Uncles"`
	Coinbase    common.Address   `json:"miner"`
	Root        common.Hash      `json:"stateRoot"`
	TxHash      common.Hash      `json:"transactionsRoot"`
	ReceiptHash common.Hash      `json:"receiptsRoot"`
	Bloom       types.Bloom      `json:"logsBloom"`
	Difficulty  *BigInt          `json:"difficulty"`
	Number      *BigInt          `json:"number"`
	GasLimit    uint64           `json:"gasLimit"`
	GasUsed     uint64           `json:"gasUsed"`
	Time        uint64           `json:"timestamp"`
	Extra       []byte           `json:"extraData"`
	MixDigest   common.Hash      `json:"mixHash"`
	Nonce       types.BlockNonce `json:"nonce"`
	BaseFee     *BigInt          `json:"baseFeePerGas"`
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

func adaptHeader(header *types.Header) (*Header, error) {
	return &Header{
		ParentHash:  header.ParentHash,
		UncleHash:   header.UncleHash,
		Coinbase:    header.Coinbase,
		Root:        header.Root,
		TxHash:      header.TxHash,
		ReceiptHash: header.ReceiptHash,
		Bloom:       header.Bloom,
		Difficulty:  &BigInt{header.Difficulty},
		Number:      &BigInt{header.Number},
		GasLimit:    header.GasLimit,
		GasUsed:     header.GasUsed,
		Time:        header.Time,
		Extra:       header.Extra,
		MixDigest:   header.MixDigest,
		Nonce:       header.Nonce,
		BaseFee:     &BigInt{header.BaseFee},
	}, nil
}

func (tx *Transaction) adaptTransaction() (types.Transaction, error) {
	gasPrice, value := uint256.NewInt(0), uint256.NewInt(0)
	var chainId *uint256.Int
	var overflow bool
	if tx.ChainId != nil {
		chainId, overflow = uint256.FromBig((*big.Int)(tx.ChainId.Int))
		if overflow {
			return nil, fmt.Errorf("chainId field caused an overflow (uint256)")
		}
	}

	if tx.Amount != nil {
		value, overflow = uint256.FromBig((*big.Int)(tx.Amount.Int))
		if overflow {
			return nil, fmt.Errorf("value field caused an overflow (uint256)")
		}
	}

	if tx.Price != nil {
		gasPrice, overflow = uint256.FromBig((*big.Int)(tx.Price.Int))
		if overflow {
			return nil, fmt.Errorf("gasPrice field caused an overflow (uint256)")
		}
	}
	switch tx.Type {
	case types.LegacyTxType, types.AccessListTxType:
		var toAddr common.Address = *tx.Recipient
		legacyTx := types.NewTransaction(uint64(tx.AccountNonce), toAddr, value, uint64(tx.GasLimit), gasPrice, tx.Payload)
		if tx.Sender != nil {
			legacyTx.CommonTx.SetFrom(*tx.Sender)
		}

		setSignatureValues(&legacyTx.CommonTx, tx.V, tx.R, tx.S)

		legacyTx.CommonTx.ChainID = chainId

		if tx.Type == types.AccessListTxType {
			accessListTx := types.AccessListTx{
				LegacyTx:   *legacyTx,
				ChainID:    chainId,
				AccessList: tx.AccessList,
			}

			return &accessListTx, nil
		} else {
			return legacyTx, nil
		}

	case types.DynamicFeeTxType:
		var tip *uint256.Int
		var feeCap *uint256.Int
		if tx.GasTipCap != nil {
			tip, overflow = uint256.FromBig((*big.Int)(tx.GasTipCap.Int))
			if overflow {
				return nil, fmt.Errorf("GasTipCap field caused an overflow (uint256)")
			}
		}

		if tx.GasFeeCap != nil {
			feeCap, overflow = uint256.FromBig((*big.Int)(tx.GasFeeCap.Int))
			if overflow {
				return nil, fmt.Errorf("GasTipCap field caused an overflow (uint256)")
			}
		}

		dynamicFeeTx := types.DynamicFeeTransaction{
			CommonTx: types.CommonTx{
				ChainID: chainId,
				Nonce:   uint64(tx.AccountNonce),
				To:      tx.Recipient,
				Value:   value,
				Gas:     uint64(tx.GasLimit),
				Data:    tx.Payload,
			},
			Tip:        tip,
			FeeCap:     feeCap,
			AccessList: tx.AccessList,
		}

		if tx.Sender != nil {
			dynamicFeeTx.CommonTx.SetFrom(*tx.Sender)
		}
		setSignatureValues(&dynamicFeeTx.CommonTx, tx.V, tx.R, tx.S)
		return &dynamicFeeTx, nil

	default:
		return nil, nil

	}
}

func setSignatureValues(tx *types.CommonTx, V, R, S *BigInt) error {
	if V != nil {
		value, overflow := uint256.FromBig((*big.Int)(V.Int))
		if overflow {
			return fmt.Errorf("value field caused an overflow (uint256)")
		}

		tx.V = *value
	}

	if R != nil {
		value, overflow := uint256.FromBig((*big.Int)(R.Int))
		if overflow {
			return fmt.Errorf("value field caused an overflow (uint256)")
		}

		tx.R = *value
	}

	if S != nil {
		value, overflow := uint256.FromBig((*big.Int)(S.Int))
		if overflow {
			return fmt.Errorf("value field caused an overflow (uint256)")
		}

		tx.S = *value
	}

	return nil
}

func convertReceipts(Receipts types.Receipts) []*Receipt {
	receipts := make([]*Receipt, 0)
	for _, rec := range Receipts {
		result_logs := make([]*Logs, 0)
		for _, logs := range rec.Logs {
			log := &Logs{
				Address:     logs.Address,
				Topics:      logs.Topics,
				Data:        logs.Data,
				BlockNumber: logs.BlockNumber,
				TxHash:      logs.TxHash,
				TxIndex:     logs.TxIndex,
				BlockHash:   logs.BlockHash,
				Index:       logs.Index,
				Removed:     logs.Removed,
			}
			result_logs = append(result_logs, log)
		}
		receipt := &Receipt{
			PostStateOrStatus: rec.PostState,
			CumulativeGasUsed: rec.CumulativeGasUsed,
			TxHash:            rec.TxHash,
			ContractAddress:   rec.ContractAddress,
			Logs:              result_logs,
			GasUsed:           rec.GasUsed,
		}
		receipts = append(receipts, receipt)
	}
	return receipts
}

func convertTransactions(txs types.Transactions) ([]*Transaction, error) {
	var Transactions []*Transaction
	for i, tx := range txs {
		sender, ok := tx.GetSender()
		if !ok {
			return Transactions, fmt.Errorf("tx index %d failed to get sender", i)
		}
		new_tx := &Transaction{
			Type:         tx.Type(),
			AccessList:   tx.GetAccessList(),
			ChainId:      &BigInt{tx.GetChainID().ToBig()},
			AccountNonce: tx.GetNonce(),
			Price:        &BigInt{tx.GetPrice().ToBig()},
			GasLimit:     tx.GetGas(),
			GasTipCap:    &BigInt{tx.GetTip().ToBig()},
			GasFeeCap:    &BigInt{tx.GetFeeCap().ToBig()},
			Sender:       &sender,
			Recipient:    tx.GetTo(),
			Amount:       &BigInt{tx.GetValue().ToBig()},
			Payload:      tx.GetData(),
		}
		Transactions = append(Transactions, new_tx)
	}
	return Transactions, nil
}

func converUncles(ommerHeaders []*types.Header) []*Header {
	var new_uncles []*Header
	for _, uncle := range ommerHeaders {
		adapted_uncle, _ := adaptHeader(uncle)
		new_uncles = append(new_uncles, adapted_uncle)
	}
	return new_uncles
}
