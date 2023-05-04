package rlptracer

import (
	"fmt"
	"math/big"

	"github.com/ledgerwatch/erigon-lib/common"
)

// import (
// 	"fmt"
// 	"math/big"

// 	"github.com/ledgerwatch/erigon/common"
// 	"github.com/ledgerwatch/erigon/log"
// )

type HashFunction byte

const (
	_ HashFunction = iota
	hashFnSHA256
	hashFnRIPEMD160
	hashFnKeccak256
	hashFnKeccak256Trunc4
)

var hashFunctionToString = map[HashFunction]string{
	hashFnSHA256:          "sha256",
	hashFnRIPEMD160:       "ripemd160",
	hashFnKeccak256:       "sha3",
	hashFnKeccak256Trunc4: "4byte",
}

const HashFnKeccak256 = hashFnKeccak256

type HashImage struct {
	hashFn HashFunction
	output string
}

const defaultPreimageReductionLimit = 200

type PreimageSource interface {
	GetPreimage(hashFn HashFunction, image []byte) ([]byte, bool)
	PutPreimage(hashFn HashFunction, preimage []byte, image []byte)
	CountPreimages(hashFn HashFunction) int
}

func GetPreimageOrImage(s PreimageSource, hashFn HashFunction, image []byte) []byte {
	if s == nil {
		return image
	}
	if preimage, ok := s.GetPreimage(hashFn, image); ok {
		return preimage
	}
	return image
}

var maxSlotNumber = new(big.Int).SetUint64(256)

func GetPreimageChain(s PreimageSource, hashFn HashFunction, image []byte) (output []interface{}) {
	if s == nil {
		return
	}

	workStack := [][]byte{image}
	reductionsLeft := defaultPreimageReductionLimit
	imageAsNum := new(big.Int)

	for reductionsLeft > 0 && len(workStack) > 0 {
		maybeImage := workStack[len(workStack)-1]
		workStack = workStack[:len(workStack)-1]

		if len(workStack) == 0 && len(maybeImage) == 32 && imageAsNum.SetBytes(maybeImage).IsUint64() && imageAsNum.Cmp(maxSlotNumber) == -1 {
			output = append(output, imageAsNum.Uint64())
		} else if preimage, ok := s.GetPreimage(hashFn, maybeImage); ok {
			switch len(preimage) {
			case 32:
				workStack = append(workStack, preimage)
			case 64:
				workStack = append(workStack, preimage[32:64], preimage[0:32])
			default:
				output = append(output, preimage)
			}
		} else if len(maybeImage) == 32 {
			output = append(output, common.BytesToHash(maybeImage))
		} else {
			output = append(output, string(maybeImage))
		}

		reductionsLeft--
	}

	if reductionsLeft == 0 {
		// log.Error("GetPreimageChain exhausted reductions", "image", fmt.Sprintf("%#x", image))
		output = []interface{}{common.BytesToHash(image)}
	}

	// reverse output in place
	for i, j := 0, len(output)-1; i < j; i, j = i+1, j-1 {
		output[i], output[j] = output[j], output[i]
	}

	return
}

func GetPreimageChainVerbose(s PreimageSource, hashFn HashFunction, image []byte) (output []interface{}) {
	if s == nil {
		return
	}

	workStack := [][]byte{image}
	reductionsLeft := defaultPreimageReductionLimit
	imageAsNum := new(big.Int)

	fmt.Printf("GetPreimageChain start: image=%#x size(pSource, %s)=%d\n", image, hashFunctionToString[hashFn], s.CountPreimages(hashFn))

	for reductionsLeft > 0 && len(workStack) > 0 {
		maybeImage := workStack[len(workStack)-1]
		workStack = workStack[:len(workStack)-1]

		fmt.Printf("GetPreimageChain: len(workStack)=%d maybeImage=%#x\n", len(workStack), maybeImage)
		if len(workStack) == 0 && len(maybeImage) == 32 && imageAsNum.SetBytes(maybeImage).IsUint64() && imageAsNum.Cmp(maxSlotNumber) == -1 {
			fmt.Printf("GetPreimageChain -> SlotNumber(%d)\n", imageAsNum.Uint64())
			output = append(output, imageAsNum.Uint64())
		} else if preimage, ok := s.GetPreimage(hashFn, maybeImage); ok {
			switch len(preimage) {
			case 32:
				fmt.Printf("GetPreimageChain <- Hash(%#x)\n", preimage)
				workStack = append(workStack, preimage)
			case 64:
				fmt.Printf("GetPreimageChain <- Hash(%#x)\n", preimage[32:64])
				fmt.Printf("GetPreimageChain <- Hash(%#x)\n", preimage[0:32])
				workStack = append(workStack, preimage[32:64], preimage[0:32])
			default:
				fmt.Printf("GetPreimageChain -> Binary(%#x)\n", preimage)
				output = append(output, preimage)
			}
		} else if len(maybeImage) == 32 {
			fmt.Printf("GetPreimageChain -> Hash(%#x)\n", maybeImage)
			output = append(output, common.BytesToHash(maybeImage))
		} else {
			fmt.Printf("GetPreimageChain -> Binary(%#x)\n", maybeImage)
			output = append(output, string(maybeImage))
		}

		reductionsLeft--
	}

	if reductionsLeft == 0 {
		// log.Error("GetPreimageChain exhausted reductions", "image", fmt.Sprintf("%#x", image))
		return []interface{}{common.BytesToHash(image)}
	}

	// reverse output in place
	for i, j := 0, len(output)-1; i < j; i, j = i+1, j-1 {
		output[i], output[j] = output[j], output[i]
	}

	fmt.Printf("GetPreimageChain done: %d outputs\n\n", len(output))

	return
}

type EphemeralPreimageSource struct {
	preimages map[HashImage][]byte
	parent    PreimageSource
}

func NewEphemeralPreimageSource(parent PreimageSource) *EphemeralPreimageSource {
	return &EphemeralPreimageSource{
		preimages: map[HashImage][]byte{},
		parent:    parent,
	}
}

func (s *EphemeralPreimageSource) GetPreimage(hashFn HashFunction, image []byte) ([]byte, bool) {
	if preimage, ok := s.preimages[HashImage{hashFn, string(image)}]; ok {
		return preimage, true
	}
	if s.parent != nil {
		return s.parent.GetPreimage(hashFn, image)
	}
	return nil, false
}

func isEmpty(s []byte) bool {
	if s == nil || len(s) == 0 {
		return true
	}

	for _, v := range s {
		if v != 0 {
			return false
		}
	}
	return true
}

func isEmptyOrMissized(s []byte, expectedLen int) bool {
	if s == nil || len(s) != expectedLen {
		return true
	}

	for _, v := range s {
		if v != 0 {
			return false
		}
	}
	return true
}

func (s *EphemeralPreimageSource) CountPreimages(hashFn HashFunction) int {
	return len(s.preimages)
}

func (s *EphemeralPreimageSource) PutPreimage(hashFn HashFunction, input []byte, output []byte) {
	s.preimages[HashImage{hashFn, string(output)}] = input
}

func (s *EphemeralPreimageSource) Clear() {
	s.preimages = map[HashImage][]byte{}
}
