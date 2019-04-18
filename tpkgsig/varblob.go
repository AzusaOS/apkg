package tpkgsig

import (
	"encoding/binary"
	"errors"
	"io"
)

func ReadVarblob(r SigReader) ([]byte, error) {
	l, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}

	if l > 64 {
		return nil, errors.New("invalid signature data (oversized blob)")
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
