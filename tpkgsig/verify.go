package tpkgsig

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"log"

	"golang.org/x/crypto/ed25519"
)

type SigReader interface {
	io.Reader
	io.ByteReader
}

// varint(1), varint(ed25519.PublicKeySize), varint(ed25519.SignatureSize) = 3
const SignatureSize = 3 + ed25519.PublicKeySize + ed25519.SignatureSize

func VerifyPkg(data []byte, sig SigReader) error {
	return verify(data, sig, trustedPkgSig)
}

func VerifyDb(data []byte, sig SigReader) error {
	return verify(data, sig, trustedDbSig)
}

func verify(data []byte, sigB SigReader, trust map[string]string) error {
	n, _ := binary.ReadUvarint(sigB)
	if n != 0x0001 {
		return errors.New("unsupported package signature version")
	}

	// read pubkey
	pub, err := ReadVarblob(sigB)
	if err != nil {
		return err
	}

	// read sig
	blob, err := ReadVarblob(sigB)
	if err != nil {
		return err
	}

	// check sig
	if !ed25519.Verify(ed25519.PublicKey(pub), data, blob) {
		return errors.New("invalid signature")
	}

	// check trust data
	keyS := base64.RawURLEncoding.EncodeToString(pub)
	keyN, ok := trust[keyS]
	if !ok {
		return errors.New("valid signature from non trusted key")
	}

	log.Printf("tpkgsig: Verified valid signature by %s (%s)", keyN, keyS)

	return nil
}
