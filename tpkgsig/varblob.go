package tpkgsig

import (
	"encoding/binary"
	"errors"
	"io"
)

func ReadVarblob(r SigReader, maxLen uint64) ([]byte, error) {
	l, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}

	if l > maxLen {
		return nil, errors.New("oversized blob")
	}

	b := make([]byte, l)
	_, err = r.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func WriteVarblob(w io.Writer, v []byte) error {
	b := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(b, uint64(len(v)))

	_, err := w.Write(b[:n])
	if err != nil {
		return err
	}
	_, err = w.Write(v)
	return err
}
