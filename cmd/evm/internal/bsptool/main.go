package bsptool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	json.Unmarshal(byteValue, &bsp)

	if err := json.Unmarshal(byteValue, &bsp); err != nil {
		return NewError(ErrorJson, fmt.Errorf("failed unmarshaling txs-file: %v", err))
	}

	return nil

}
