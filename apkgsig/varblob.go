package apkgsig

import (
	"encoding/binary"
	"errors"
	"io"
)

// ReadVarblob reads a variable-length blob from the reader.
// The blob is prefixed with its length as a varint.
// Returns an error if the blob exceeds maxLen.
func ReadVarblob(r SigReader, maxLen uint64) ([]byte, error) {
	l, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}

	if l > maxLen {
		return nil, errors.New("oversized blob")
	}

	b := make([]byte, l)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// WriteVarblob writes a variable-length blob to the writer.
// The blob is prefixed with its length as a varint.
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
