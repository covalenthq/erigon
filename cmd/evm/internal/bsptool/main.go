package bsptool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/urfave/cli"
)

const (
	ErrorEVM              = 2
	ErrorVMConfig         = 3
	ErrorMissingBlockhash = 4

	ErrorJson = 10
	ErrorIO   = 11

	stdinSelector = "stdin"
)

type NumberedError struct {
	errorCode int
	err       error
}

func NewError(errorCode int, err error) *NumberedError {
	return &NumberedError{errorCode, err}
}

func (n *NumberedError) Error() string {
	return fmt.Sprintf("ERROR(%d): %v", n.errorCode, n.err.Error())
}

func (n *NumberedError) Code() int {
	return n.errorCode
}

type BSP struct {
	Type            string
	NetworkId       int
	Hash            string
	TotalDifficulty int
	Header          Header
	Transactions    []Transaction
	Uncles          []Uncle
	Receipts        []Receipt
	Senders         []string
	State           State
}

type Header struct {
	parentHash       string
	sha3Uncles       string
	miner            string
	stateRoot        string
	transactionsRoot string
	receiptsRoot     string
	logsBloom        []int
	difficulty       int
	number           int
	gasLimit         int
	gasUsed          int
	timestamp        int
	extraData        string
	mixHash          string
	nonce            []int
	baseFeePerGas    int
}

type Transaction struct {
	nonce    int
	gasPrice int
	gas      int
	from     string
	to       string
	value    int
	input    string
}

type Uncle struct{}

type Receipt struct {
	PostStateOrStatus string
	CumulativeGasUsed int
	TxHash            string
	ContractAddress   string
	Logs              []Log
	GasUsed           int
}

type Log struct {
	address          string
	topics           string
	data             string
	blockNumber      int
	transactionHash  string
	transactionIndex int
	blockHash        string
	logIndex         int
	removed          bool
}

type State struct {
	AccountRead []AccountRead
	StorageRead []StorageRead
	CodeRead    []CodeRead
}

type AccountRead struct {
	Address  string
	Nonce    int
	Balance  big.Int
	CodeHash string
}

type StorageRead struct {
	Account string
	SlotKey string
	Value   string
}

type CodeRead struct {
	Hash string
	Code string
}

func Main(ctx *cli.Context) error {

	var (
		bspStr = ctx.String(InputBSPFlag.Name)
	)

	inFile, err1 := os.Open(bspStr)
	if err1 != nil {
		return NewError(ErrorIO, fmt.Errorf("failed reading alloc file: %v", err1))
	}
	defer inFile.Close()
	byteValue, _ := ioutil.ReadAll(inFile)

	var bsp BSP

	if err := json.Unmarshal(byteValue, &bsp); err != nil {
		return NewError(ErrorJson, fmt.Errorf("failed unmarshaling txs-file: %v", err))
	}

	return nil

}
