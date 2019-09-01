package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"git.atonline.com/azusa/apkg/apkgsig"
)

type pkgMeta struct {
	FullName string `json:"full_name"`
	Inodes   uint32 `json:"inodes"`
	Arch     string `json:"arch"`
	Os       string `json:"os"`
}

type pkginfo struct {
	flags   uint64
	created time.Time
	meta    *pkgMeta

	rawHeader []byte
	rawSig    []byte
	rawMeta   []byte

	sig *apkgsig.VerifyResult

	// details in signature
	headerHash [32]byte // sha256 of header
}

func parsePkgHeader(f *os.File) (*pkginfo, error) {
	p := &pkginfo{}

	// read header, check file
	header := make([]byte, 124)
	_, err := f.ReadAt(header, 0)
	if err != nil {
		return nil, err
	}

	p.rawHeader = header
	p.headerHash = sha256.Sum256(header)

	if string(header[:4]) != "APKG" {
		return nil, errors.New("not a APKG file")
	}

	r := bytes.NewReader(header[4:])
	var version uint32
	err = binary.Read(r, binary.BigEndian, &version)
	if err != nil {
		return nil, err
	}
	if version != 1 {
		return nil, errors.New("unsupported file version")
	}

	err = binary.Read(r, binary.BigEndian, &p.flags)
	if err != nil {
		return nil, err
	}

	ts := make([]int64, 2)
	err = binary.Read(r, binary.BigEndian, ts)
	if err != nil {
		return nil, err
	}
	p.created = time.Unix(ts[0], ts[1])

	metadata := make([]uint32, 2) // metadata offset + len (json encoded)
	err = binary.Read(r, binary.BigEndian, metadata)
	if err != nil {
		return nil, err
	}

	metadata_hash := make([]byte, 32)
	_, err = io.ReadFull(r, metadata_hash)
	if err != nil {
		return nil, err
	}

	table := make([]uint32, 2) // hash table offset + len
	err = binary.Read(r, binary.BigEndian, table)
	if err != nil {
		return nil, err
	}

	table_hash := make([]byte, 32)
	_, err = io.ReadFull(r, table_hash)
	if err != nil {
		return nil, err
	}

	// read sign_offset + data_offset
	last_offt := make([]uint32, 2)
	err = binary.Read(r, binary.BigEndian, last_offt)
	if err != nil {
		return nil, err
	}

	// read sign
	sig := make([]byte, apkgsig.SignatureSize)
	_, err = f.ReadAt(sig, int64(last_offt[0]))
	if err != nil {
		return nil, err
	}
	p.rawSig = sig

	p.sig, err = apkgsig.VerifyPkg(header, bytes.NewReader(sig))
	if err != nil {
		return nil, err
	}

	// read metadata
	mt := make([]byte, metadata[1])           // len
	_, err = f.ReadAt(mt, int64(metadata[0])) // pos
	if err != nil {
		return nil, err
	}

	// check hash
	mth := sha256.Sum256(mt)
	if !bytes.Equal(mth[:], metadata_hash) {
		return nil, errors.New("corrupted metadata")
	}
	p.rawMeta = mt

	// parse json
	err = json.Unmarshal(mt, &p.meta)
	if err != nil {
		return nil, err
	}

	// check hash table hash
	ht := make([]byte, table[1])
	_, err = f.ReadAt(ht, int64(table[0]))
	if err != nil {
		return nil, err
	}

	// check hash
	hth := sha256.Sum256(ht)
	if !bytes.Equal(hth[:], table_hash) {
		return nil, errors.New("corrupted hash table")
	}

	return p, nil
}
