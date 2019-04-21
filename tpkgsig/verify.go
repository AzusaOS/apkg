package tpkgsig

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/ed25519"
)

type SigReader interface {
	io.Reader
	io.ByteReader
}

type VerifyResult struct {
	Version int
	Key     string
	Name    string
}

// varint(1), varint(ed25519.PublicKeySize), varint(ed25519.SignatureSize) = 3
const SignatureSize = 3 + ed25519.PublicKeySize + ed25519.SignatureSize

func VerifyPkg(data []byte, sig SigReader) (*VerifyResult, error) {
	return verify(data, sig, trustedPkgSig)
}

func VerifyDb(data []byte, sig SigReader) (*VerifyResult, error) {
	return verify(data, sig, trustedDbSig)
}

func verify(data []byte, sigB SigReader, trust map[string]string) (*VerifyResult, error) {
	n, _ := binary.ReadUvarint(sigB)
	if n != 0x0001 {
		return nil, errors.New("unsupported package signature version")
	}

	// read pubkey
	pub, err := ReadVarblob(sigB, ed25519.PublicKeySize)
	if err != nil {
		return nil, err
	}

	// read sig
	blob, err := ReadVarblob(sigB, ed25519.SignatureSize)
	if err != nil {
		return nil, err
	}

	// check sig
	if !ed25519.Verify(ed25519.PublicKey(pub), data, blob) {
		return nil, errors.New("invalid signature")
	}

	// check trust data
	keyS := base64.RawURLEncoding.EncodeToString(pub)
	keyN, ok := trust[keyS]
	if !ok {
		return nil, errors.New("valid signature from non trusted key")
	}

	//log.Printf("tpkgsig: Verified valid signature by %s (%s)", keyN, keyS)

	res := &VerifyResult{
		Version: int(n),
		Key:     keyS,
		Name:    keyN,
	}

	return res, nil
}
