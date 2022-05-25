package blockSpecimenTool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ledgerwatch/erigon/core/types"
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

func Main(ctx *cli.Context) error {

	var (
		blockSpecimenStr = ctx.String(InputBlockSpecimenFlag.Name)
	)

	inFile, err1 := os.Open(blockSpecimenStr)
	if err1 != nil {
		return NewError(ErrorIO, fmt.Errorf("failed reading alloc file: %v", err1))
	}
	defer inFile.Close()
	byteValue, _ := ioutil.ReadAll(inFile)

	var blockSpecimen types.BlockSpecimen

	if err := json.Unmarshal(byteValue, &blockSpecimen); err != nil {
		return NewError(ErrorJson, fmt.Errorf("failed unmarshaling txs-file: %v", err))
	}

	fmt.Println(blockSpecimen.Transactions[0])

	transactions, err := types.FatemeTxs(blockSpecimen.Transactions)
	if err != nil {
		fmt.Println("error")
	}

	block := types.NewBlock(types.FatemeHeader(blockSpecimen.Header),
		transactions, types.FatemeUncles(blockSpecimen.Uncles),
		types.FatemeReceipts(blockSpecimen.Receipts))

	fmt.Println(block.Hash())

	return nil

}
