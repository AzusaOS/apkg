package apkgdb

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
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.atonline.com/azusa/apkg/apkgfs"
	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/KarpelesLab/smartremote"
	"github.com/KarpelesLab/squashfs"
	"github.com/boltdb/bolt"
	"github.com/petar/GoLLRB/llrb"
)

type packageMetaInfo struct {
	FullName string `json:"full_name"`
	Inodes   uint64 `json:"inodes"`
	Arch     string `json:"arch"`
	Os       string `json:"os"`
}

type Package struct {
	parent   *DB
	startIno uint64
	hash     []byte // typically sha256
	size     uint64
	inodes   uint64
	name     string
	path     string // path relative to where db file was downloaded from

	// from db
	rawHeader []byte
	rawSig    []byte
	rawMeta   []byte

	// from file
	flags   uint64
	created time.Time

	dl        sync.Once
	f         *smartremote.File
	offset    int64 // offset of data in file
	blockSize int64
	squash    *squashfs.Superblock
}

type pkgindex uint64

type pkgindexItem interface {
	Value() uint64
	Less(than llrb.Item) bool
}

func (p *Package) Less(than llrb.Item) bool {
	return p.startIno < than.(pkgindexItem).Value()
}

func (i pkgindex) Less(than llrb.Item) bool {
	return uint64(i) < than.(pkgindexItem).Value()
}

func (i pkgindex) Value() uint64 {
	return uint64(i)
}

func (p *Package) Value() uint64 {
	return p.startIno
}

var (
	// TODO move to inside DB
	pkgCache  = make(map[[32]byte]*Package)
	pkgCacheL sync.RWMutex
)

func (d *DB) getPkgTx(tx *bolt.Tx, startIno uint64, hash []byte) (*Package, error) {
	var hashB [32]byte
	copy(hashB[:], hash)

	// load a package based on its hash (from within a bolt transaction)
	pkgCacheL.RLock()
	if v, ok := pkgCache[hashB]; ok {
		pkgCacheL.RUnlock()
		return v, nil
	}
	pkgCacheL.RUnlock()

	b := tx.Bucket([]byte("pkg"))
	if b == nil {
		return nil, os.ErrInvalid
	}

	v := b.Get(hash)
	if v == nil {
		return nil, os.ErrInvalid
	}

	pkg := &Package{
		parent:   d,
		size:     binary.BigEndian.Uint64(v[1:9]),
		startIno: startIno,
		inodes:   binary.BigEndian.Uint64(v[17:25]),
		name:     string(v[25:]),
		path:     string(tx.Bucket([]byte("path")).Get(hash)),
		hash:     bytesDup(hash),
	}

	// read raw values (assuming buckets will exist)
	pkg.rawHeader = bytesDup(tx.Bucket([]byte("header")).Get(hash))
	pkg.rawSig = bytesDup(tx.Bucket([]byte("sig")).Get(hash))
	pkg.rawMeta = bytesDup(tx.Bucket([]byte("meta")).Get(hash))

	// keep pkg in cache
	pkgCacheL.Lock()
	defer pkgCacheL.Unlock()
	if v, ok := pkgCache[hashB]; ok {
		return v, nil
	}
	pkgCache[hashB] = pkg

	if d.parent != nil {
		d.parent.ino.ReplaceOrInsert(pkg)
	} else {
		d.ino.ReplaceOrInsert(pkg)
	}

	log.Printf("apkgdb: spawned package %s (hash=%s)", pkg.name, hex.EncodeToString(hash))

	// * pkg → package hash → package info (0 + size + inode num + inode count + package name)
	return pkg, nil
}

func OpenPackage(f *os.File) (*Package, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}

	p := &Package{
		size: uint64(st.Size()),
	}

	// read header, check file
	header := make([]byte, 124)
	_, err = f.ReadAt(header, 0)
	if err != nil {
		return nil, err
	}

	p.rawHeader = header
	headerHash := sha256.Sum256(header)
	p.hash = headerHash[:]

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

	_, err = apkgsig.VerifyPkg(header, bytes.NewReader(sig))
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

	// set name & inodes from meta
	var meta packageMetaInfo
	err = p.Meta(&meta)
	if err != nil {
		return nil, err
	}

	p.name = meta.FullName
	p.inodes = meta.Inodes

	return p, nil
}

func (p *Package) handleLookup(ino uint64) (apkgfs.Inode, error) {
	if ino == p.startIno {
		return apkgfs.NewSymlink([]byte(p.name)), nil
	}

	p.dl.Do(p.doDl)

	if p.squash == nil {
		// problem
		return nil, os.ErrInvalid
	}

	if ino <= p.startIno {
		// in case it is == it is symlink, which is returned by the
		return nil, os.ErrInvalid
	}

	return p.squash.GetInode(ino - p.startIno)
}

func (p *Package) doDl() {
	lpath := p.lpath()

	// Need to prepare path
	err := os.MkdirAll(path.Dir(lpath), 0755)
	if err != nil {
		log.Printf("apkgdb: failed to make dir: %s", err)
		return
	}

	// download this package
	// need to replace "+" with "%2B" for S3
	u := p.parent.prefix + "dist/" + p.parent.name + "/" + strings.ReplaceAll(p.path, "+", "%2B")

	f, err := smartremote.DefaultDownloadManager.OpenTo(u, lpath)
	if err != nil {
		log.Printf("apkgdb: failed to get package: %s", err)
		return
	}
	f.SetSize(int64(p.size))

	p.f = f

	err = p.validate()
	if err != nil {
		log.Printf("apkgdb: failed to validate file: %s", err)
		go func() {
			// cause download to be re-available in 10 seconds
			time.Sleep(10 * time.Second)
			p.dl = sync.Once{}
		}()
		defer p.f.Close()
		p.f = nil
		return
	}

	p.squash, err = squashfs.New(p, p.startIno)
	if err != nil {
		log.Printf("apkgdb: failed to mount: %s", err)
		defer p.f.Close()
		p.f = nil
		p.squash = nil
		return
	}
}

func (p *Package) lpath() string {
	return filepath.Join(p.parent.path, p.parent.name, p.path)
}

func (p *Package) validate() error {
	// read header, check file
	header := make([]byte, 124)
	_, err := p.f.ReadAt(header, 0)
	if err != nil {
		return err
	}

	// check hash
	h256 := sha256.Sum256(header)
	if !bytes.Equal(h256[:], p.hash) {
		os.Remove(p.lpath())
		return errors.New("header invalid or corrupted")
	}

	if string(header[:4]) != "APKG" {
		return errors.New("not a APKG file")
	}

	// we use readat + newreader to make sure so other process seeks this file
	r := bytes.NewReader(header[4:])
	var version uint32
	err = binary.Read(r, binary.BigEndian, &version)
	if err != nil {
		return err
	}
	if version != 1 {
		return errors.New("unsupported file version")
	}

	err = binary.Read(r, binary.BigEndian, &p.flags)
	if err != nil {
		return err
	}

	ts := make([]int64, 2)
	err = binary.Read(r, binary.BigEndian, ts)
	if err != nil {
		return err
	}
	p.created = time.Unix(ts[0], ts[1])

	metadata := make([]uint32, 2) // metadata offset + len (json encoded)
	err = binary.Read(r, binary.BigEndian, metadata)
	if err != nil {
		return err
	}

	metadata_hash := make([]byte, 32)
	_, err = io.ReadFull(r, metadata_hash)
	if err != nil {
		return err
	}

	table := make([]uint32, 2) // hash table offset + len
	err = binary.Read(r, binary.BigEndian, table)
	if err != nil {
		return err
	}

	table_hash := make([]byte, 32)
	_, err = io.ReadFull(r, table_hash)
	if err != nil {
		return err
	}

	// read sign_offset + data_offset + block size
	last_offt := make([]uint32, 3)
	err = binary.Read(r, binary.BigEndian, last_offt)
	if err != nil {
		return err
	}

	// check signature
	sig := make([]byte, 128)
	_, err = p.f.ReadAt(sig, int64(last_offt[0]))
	if err != nil {
		return err
	}
	_, err = apkgsig.VerifyPkg(header, bytes.NewReader(sig))
	if err != nil {
		return err
	}
	//log.Printf("apkgdb: verified package signature, signed by %s", sigV.Name)

	// TODO store all that stuff

	p.offset = int64(last_offt[1])
	p.blockSize = int64(last_offt[2])

	return nil
}

func (p *Package) Meta(v interface{}) error {
	return json.Unmarshal(p.rawMeta, v)
}

func (p *Package) ReadAt(b []byte, off int64) (int, error) {
	if p.f == nil {
		return 0, os.ErrInvalid // should return E_IO
	}
	//log.Printf("converted read = %d", off+p.offset)
	// we need to align offset to lower blockSize, and size to higher blockSize
	/*
		offDelta = p.blockSize - (off % p.blockSize)
		if offDelta == p.blockSize {
			offDelta = 0
		}*/

	return p.f.ReadAt(b, off+p.offset)
}
