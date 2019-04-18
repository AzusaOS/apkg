package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/MagicalTux/hsm"
	"github.com/tardigradeos/tpkg/tpkgsig"
	"golang.org/x/crypto/ed25519"
)

type fileKey struct {
	arch string
	os   string
}

type dbFile struct {
	f     *os.File
	name  string
	path  string
	stamp string
	arch  string
	os    string

	ino uint64
	cnt uint32

	idxFN  map[string]int64
	idxIno map[uint64]int64

	w    io.Writer
	hash hash.Hash
}

func processDb(name string, k hsm.Key) error {
	dir := filepath.Join(os.Getenv("HOME"), "projects/tpkg-tools/repo/tpkg/dist", name)
	files := make(map[fileKey]*dbFile)
	now := time.Now()
	stamp := now.UTC().Format("20060102150405")

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

		fk := fileKey{arch: p.meta.Arch, os: p.meta.Os}
		db, ok := files[fk]
		if !ok {
			db = &dbFile{
				path:   filepath.Join(os.Getenv("HOME"), "projects/tpkg-tools/repo/tpkg/db", name, fk.os, fk.arch, stamp+".bin"),
				stamp:  stamp,
				arch:   fk.arch,
				os:     fk.os,
				name:   name,
				idxFN:  make(map[string]int64),
				idxIno: make(map[uint64]int64),
			}

			// open db
			db.f, err = os.Create(db.path + "~")
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

	for _, db := range files {
		err = db.finalize(k)
		if err != nil {
			return err
		}
	}
	for _, db := range files {
		db.upload()
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
	// 40 (pkg count)
	binary.Write(db.f, binary.BigEndian, uint32(0)) // offset 40: number of packages (filled at the end)

	nameBuf := make([]byte, 32)
	copy(nameBuf, db.name)
	db.f.Write(nameBuf)

	// SHA256('') = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	emptyHash, _ := hex.DecodeString("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	// 76
	binary.Write(db.f, binary.BigEndian, uint32(196+128)) // location of data
	binary.Write(db.f, binary.BigEndian, uint32(0))       // length of data
	db.f.Write(emptyHash)                                 // hash of data
	// 110
	binary.Write(db.f, binary.BigEndian, uint32(0)) // location of id index
	binary.Write(db.f, binary.BigEndian, uint32(0)) // length of id index
	db.f.Write(emptyHash)                           // hash of id index
	// 156
	binary.Write(db.f, binary.BigEndian, uint32(0)) // location of name index
	binary.Write(db.f, binary.BigEndian, uint32(0)) // length of name index
	db.f.Write(emptyHash)                           // hash of name index

	n, _ := db.f.Seek(0, io.SeekCurrent)

	if n != 196 {
		return errors.New("invalid header length")
	}

	db.f.Write(make([]byte, 128)) // reserved space for signature

	db.hash = sha256.New()
	db.w = io.MultiWriter(db.f, db.hash)

	return nil
}

func (db *dbFile) index(rpath string, info os.FileInfo, p *pkginfo) {
	// write package to list & store position details
	pos, _ := db.f.Seek(0, io.SeekCurrent)
	db.idxFN[p.meta.FullName] = pos
	db.idxIno[db.ino] = pos
	db.ino += uint64(p.meta.Inodes) + 1
	db.cnt += 1

	db.w.Write([]byte{0}) // package
	db.w.Write(p.headerHash[:])
	binary.Write(db.w, binary.BigEndian, uint64(info.Size()))
	binary.Write(db.w, binary.BigEndian, p.meta.Inodes)

	tpkgsig.WriteVarblob(db.w, []byte(p.meta.FullName))
	tpkgsig.WriteVarblob(db.w, []byte(rpath))
}

func (db *dbFile) finalize(k hsm.Key) error {
	// compute hash, etc
	pos, _ := db.f.Seek(0, io.SeekCurrent)
	hash := db.hash.Sum(nil)

	// write to header
	db.w = nil
	db.hash = nil

	db.f.Seek(40, io.SeekStart)
	binary.Write(db.f, binary.BigEndian, db.cnt) // pkg count

	db.f.Seek(76, io.SeekStart) // length of data, data starts at 196+128
	var start uint32
	binary.Read(db.f, binary.BigEndian, &start)             // should be reading 196+128
	binary.Write(db.f, binary.BigEndian, uint32(pos)-start) // write length of data
	db.f.Write(hash)                                        // write hash of data

	// TODO: index, etc

	// compute header signature
	header := make([]byte, 196)
	_, err := db.f.ReadAt(header, 0)
	if err != nil {
		return err
	}

	log.Printf("Signing %s...", db.path)

	sigB := &bytes.Buffer{}
	vInt := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(vInt, 0x0001) // Signature type 1 = ed25519
	sigB.Write(vInt[:n])

	sig_pub, err := k.PublicBlob()
	if err != nil {
		return err
	}

	tpkgsig.WriteVarblob(sigB, sig_pub)

	// use raw hash for ed25519
	sig_blob, err := k.Sign(rand.Reader, header, crypto.Hash(0))
	if err != nil {
		return err
	}
	tpkgsig.WriteVarblob(sigB, sig_blob)

	// verify signature
	err = tpkgsig.VerifyDb(header, bytes.NewReader(sigB.Bytes()))
	if err != nil {
		return err
	}

	if sigB.Len() > 128 {
		return errors.New("signature buffer not large enough!")
	}

	db.f.Seek(196, io.SeekStart)
	db.f.Write(sigB.Bytes())

	db.f.Close()

	err = os.Rename(db.path+"~", db.path)
	if err != nil {
		return err
	}

	// update LATEST.txt
	lat, err := os.Create(filepath.Join(filepath.Dir(db.path), "LATEST.txt"))
	if err != nil {
		return err
	}
	fmt.Fprintf(lat, "%s\n", db.stamp)
	lat.Close()

	return nil
}

func (db *dbFile) upload() error {
	// upload file to s3
	s3pfx := "s3:/" + path.Join("/tpkg/db", db.name, db.os, db.arch)
	log.Printf("uploading files to %s", s3pfx)

	//system('aws s3 cp --cache-control max-age=31536000 '.escapeshellarg($db_path.'/'.$datestamp.'.bin').' '.escapeshellarg($s3_prefix.'/'.$datestamp.'.bin'));
	cmd1 := exec.Command("aws", "s3", "cp", "--cache-control", "max-age=31536000", db.path, s3pfx+"/"+db.stamp+".bin")
	cmd1.Stdout = os.Stdout
	cmd1.Stderr = os.Stderr
	err := cmd1.Run()
	if err != nil {
		return err
	}

	//system('aws s3 cp --cache-control max-age=60 '.escapeshellarg($db_path.'/LATEST.txt').' '.escapeshellarg($s3_prefix.'/LATEST.txt'));
	cmd2 := exec.Command("aws", "s3", "cp", "--cache-control", "max-age=60", filepath.Dir(db.path)+"/LATEST.txt", s3pfx+"/LATEST.txt")
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr

	return cmd2.Run()
}

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

	// details in signature
	headerHash [32]byte // sha256 of header
	key        []byte
	sig        []byte
}

func parsePkgHeader(f *os.File) (*pkginfo, error) {
	p := &pkginfo{}

	// read header, check file
	header := make([]byte, 120)
	_, err := f.ReadAt(header, 0)
	if err != nil {
		return nil, err
	}

	p.headerHash = sha256.Sum256(header)

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
