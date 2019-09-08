package apkgsig

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"encoding/binary"
	"errors"

	"github.com/MagicalTux/hsm"
)

func Sign(k hsm.Key, data []byte) ([]byte, error) {
	sigB := &bytes.Buffer{}
	vInt := make([]byte, binary.MaxVarintLen64)
	vIntL := binary.PutUvarint(vInt, 0x0001) // Signature type 1 = ed25519
	sigB.Write(vInt[:vIntL])

	sig_pub, err := k.PublicBlob()
	if err != nil {
		return nil, err
	}

	WriteVarblob(sigB, sig_pub)

	// use raw hash for ed25519
	sig_blob, err := k.Sign(rand.Reader, data, crypto.Hash(0))
	if err != nil {
		return nil, err
	}
	WriteVarblob(sigB, sig_blob)

	if sigB.Len() > SignatureSize {
		return nil, errors.New("signature was too large")
	}

	return sigB.Bytes(), nil
}
