package librsync

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/md4"
)

const (
	BLAKE2_SUM_LENGTH = 32
	MD4_SUM_LENGTH    = 16
)

type SignatureType struct {
	sigType    MagicNumber
	blockLen   uint32
	strongLen  uint32
	strongSigs [][]byte
	weak2block map[uint32]int
}

func CalcStrongSum(data []byte, sigType MagicNumber, strongLen uint32) ([]byte, error) {
	switch sigType {
	case BLAKE2_SIG_MAGIC:
		d := blake2b.Sum256(data)
		return d[:strongLen], nil
	case MD4_SIG_MAGIC:
		d := md4.New()
		d.Write(data)
		return d.Sum(nil)[:strongLen], nil
	}
	return nil, fmt.Errorf("Invalid sigType %#x", sigType)
}

func Signature(input io.Reader, output io.Writer, blockLen, strongLen uint32, sigType MagicNumber) (*SignatureType, error) {
	var maxStrongLen uint32

	switch sigType {
	case BLAKE2_SIG_MAGIC:
		maxStrongLen = BLAKE2_SUM_LENGTH
	case MD4_SIG_MAGIC:
		maxStrongLen = MD4_SUM_LENGTH
	default:
		return nil, fmt.Errorf("invalid sigType %#x", sigType)
	}

	if strongLen > maxStrongLen {
		return nil, fmt.Errorf("invalid strongLen %d for sigType %#x", strongLen, sigType)
	}

	err := binary.Write(output, binary.BigEndian, sigType)
	if err != nil {
		return nil, err
	}
	err = binary.Write(output, binary.BigEndian, blockLen)
	if err != nil {
		return nil, err
	}
	err = binary.Write(output, binary.BigEndian, strongLen)
	if err != nil {
		return nil, err
	}

	block := make([]byte, blockLen)

	var ret SignatureType
	ret.weak2block = make(map[uint32]int)
	ret.sigType = sigType
	ret.strongLen = strongLen
	ret.blockLen = blockLen

	for {
		n, err := input.Read(block)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		data := block[:n]

		weak := WeakChecksum(data)
		err = binary.Write(output, binary.BigEndian, weak)
		if err != nil {
			return nil, err
		}

		strong, _ := CalcStrongSum(data, sigType, strongLen)
		output.Write(strong)

		ret.weak2block[weak] = len(ret.strongSigs)
		ret.strongSigs = append(ret.strongSigs, strong)
	}

	return &ret, nil
}

// ReadSignature reads a signature from an io.Reader.
func ReadSignature(r io.Reader) (*SignatureType, error) {
	var magic MagicNumber
	err := binary.Read(r, binary.BigEndian, &magic)
	if err != nil {
		return nil, err
	}

	var blockLen uint32
	err = binary.Read(r, binary.BigEndian, &blockLen)
	if err != nil {
		return nil, err
	}

	var strongLen uint32
	err = binary.Read(r, binary.BigEndian, &strongLen)
	if err != nil {
		return nil, err
	}

	strongSigs := [][]byte{}
	weak2block := map[uint32]int{}

	for {
		var weakSum uint32
		err = binary.Read(r, binary.BigEndian, &weakSum)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		strongSum := make([]byte, strongLen)
		n, err := r.Read(strongSum)
		if err != nil {
			return nil, err
		}
		if n != int(strongLen) {
			return nil, fmt.Errorf("got only %d/%d bytes of the strong hash", n, strongLen)
		}

		weak2block[weakSum] = len(strongSigs)
		strongSigs = append(strongSigs, strongSum)
	}

	return &SignatureType{
		sigType:    magic,
		blockLen:   blockLen,
		strongLen:  strongLen,
		strongSigs: strongSigs,
		weak2block: weak2block,
	}, nil
}

// ReadSignatureFile reads a signature from the file at path.
func ReadSignatureFile(path string) (*SignatureType, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadSignature(f)
}
