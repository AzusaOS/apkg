// Package apkgsig provides cryptographic signature verification and signing
// for packages and databases using Ed25519.
package apkgsig

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/ed25519"
)

// SigReader is an interface for reading signature data, combining
// io.Reader and io.ByteReader for efficient varint parsing.
type SigReader interface {
	io.Reader
	io.ByteReader
}

// VerifyResult contains the result of a successful signature verification.
type VerifyResult struct {
	Version int    // Signature format version
	Key     string // Base64-encoded public key
	Name    string // Name of the trusted signer
}

// SignatureSize is the maximum size of a signature blob in bytes.
// It consists of version varint, public key length varint, signature length varint,
// plus the actual public key and signature data.
const SignatureSize = 3 + ed25519.PublicKeySize + ed25519.SignatureSize

// VerifyPkg verifies a package signature against trusted package signing keys.
// Returns an error if the signature is invalid or from an untrusted key.
func VerifyPkg(data []byte, sig SigReader) (*VerifyResult, error) {
	return verify(data, sig, trustedPkgSig)
}

// VerifyDb verifies a database signature against trusted database signing keys.
// Returns an error if the signature is invalid or from an untrusted key.
func VerifyDb(data []byte, sig SigReader) (*VerifyResult, error) {
	return verify(data, sig, trustedDbSig)
}

// DbKeyName returns the name associated with a trusted database signing key,
// or an empty string if the key is not trusted.
func DbKeyName(k string) string {
	name, found := trustedDbSig[k]
	if !found {
		return ""
	}
	return name
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

	//log.Printf("apkgsig: Verified valid signature by %s (%s)", keyN, keyS)

	res := &VerifyResult{
		Version: int(n),
		Key:     keyS,
		Name:    keyN,
	}

	return res, nil
}
