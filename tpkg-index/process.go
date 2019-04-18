package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ed25519"
)

type fileKey struct {
	arch string
	os   string
}

type dbFile struct {
	f    *os.File
	name string
	path string
	arch string
	os   string
}

func processDb(name string) error {
	dir := filepath.Join(os.Getenv("HOME"), "projects/tpkg-tools/repo/tpkg/dist", name)
	files := make(map[fileKey]*dbFile)
	now := time.Now()
	stamp := now.Format("20060102150405")

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.Mode().IsRegular() {
			return nil
		}
		if !strings.HasSuffix(path, ".tpkg") {
			return nil
		}
		rpath := strings.TrimLeft(strings.TrimPrefix(path, dir), "/")
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		log.Printf("Indexing: %s", rpath)
		p, err := parsePkgHeader(f)
		if err != nil {
			return err
		}

		fk := fileKey{arch: p.meta["arch"].(string), os: p.meta["os"].(string)}
		db, ok := files[fk]
		if !ok {
			db = &dbFile{
				path: filepath.Join(os.Getenv("HOME"), "projects/tpkg-tools/repo/tpkg/db", name, fk.os, fk.arch, stamp+".db"),
				arch: fk.arch,
				os:   fk.os,
				name: name,
			}

			// open db
			db.f, err = os.Create(db.path)
			if err != nil {
				return err
			}
			err = db.init(now)
			if err != nil {
				return err
			}
			files[fk] = db
		}

		db.index(rpath, info, p)
		_ = p
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (db *dbFile) init(now time.Time) error {
	log.Printf("Initializing database file %s", db.path)
	// write header to file
	db.f.Write([]byte("TPDB"))
	binary.Write(db.f, binary.BigEndian, uint32(0x00000001)) // version
	binary.Write(db.f, binary.BigEndian, uint64(0))          // flags
	binary.Write(db.f, binary.BigEndian, uint64(now.Unix()))
	binary.Write(db.f, binary.BigEndian, uint64(now.Nanosecond()))

	var os uint32
	var arch uint32
	switch db.os {
	case "linux":
		os = 0
	case "darwin":
		os = 1
	case "windows":
		os = 2
	default:
		return errors.New("unsupported os")
	}

	switch db.arch {
	case "i386":
		arch = 0
	case "amd64":
		arch = 1
	default:
		return errors.New("unsupported arch")
	}

	binary.Write(db.f, binary.BigEndian, os)
	binary.Write(db.f, binary.BigEndian, arch)
	binary.Write(db.f, binary.BigEndian, uint32(0)) // offset 40: number of packages (filled at the end)
	nameBuf := make([]byte, 32)
	copy(nameBuf, db.name)
	db.f.Write(nameBuf)

	// SHA256('') = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	emptyHash, _ := hex.DecodeString("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	binary.Write(db.f, binary.BigEndian, uint32(196+128)) // location of data
	binary.Write(db.f, binary.BigEndian, uint32(0))       // length of data
	db.f.Write(emptyHash)                                 // hash of data
	binary.Write(db.f, binary.BigEndian, uint32(0))       // location of id index
	binary.Write(db.f, binary.BigEndian, uint32(0))       // length of id index
	db.f.Write(emptyHash)                                 // hash of id index
	binary.Write(db.f, binary.BigEndian, uint32(0))       // location of name index
	binary.Write(db.f, binary.BigEndian, uint32(0))       // length of name index
	db.f.Write(emptyHash)                                 // hash of name index

	n, _ := db.f.Seek(0, io.SeekCurrent)

	if n != 196 {
		return errors.New("invalid header length")
	}

	db.f.Write(make([]byte, 128)) // reserved space for signature

	return nil
}

func (db *dbFile) index(rpath string, info os.FileInfo, p *pkginfo) {

}

type pkginfo struct {
	flags   uint64
	created time.Time
	meta    map[string]interface{}

	// details in signature
	key []byte
	sig []byte
}

func parsePkgHeader(f *os.File) (*pkginfo, error) {
	p := &pkginfo{}

	// read header, check file
	header := make([]byte, 120)
	_, err := f.ReadAt(header, 0)
	if err != nil {
		return nil, err
	}

	if string(header[:4]) != "TPKG" {
		return nil, errors.New("not a TPKG file")
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
	sig := make([]byte, 128) // should be less
	_, err = f.ReadAt(sig, int64(last_offt[0]))
	if err != nil {
		return nil, err
	}

	// read signature version
	sigB := bytes.NewReader(sig)
	sigV, err := binary.ReadUvarint(sigB)
	if err != nil {
		return nil, err
	}
	if sigV != 0x0001 { // only supported is 0x0001 ed25519
		return nil, errors.New("unsupported signature version")
	}

	pubL, err := binary.ReadUvarint(sigB)
	if err != nil {
		return nil, err
	}
	p.key = make([]byte, pubL)
	_, err = sigB.Read(p.key)
	if err != nil {
		return nil, err
	}

	sigL, err := binary.ReadUvarint(sigB)
	if err != nil {
		return nil, err
	}
	p.sig = make([]byte, sigL)
	_, err = sigB.Read(p.sig)
	if err != nil {
		return nil, err
	}

	// check signature
	if !ed25519.Verify(ed25519.PublicKey(p.key), header, p.sig) {
		return nil, errors.New("invalid signature")
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
