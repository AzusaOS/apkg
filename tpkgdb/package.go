package tpkgdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/petar/GoLLRB/llrb"
	"github.com/tardigradeos/tpkg/squashfs"
	"github.com/tardigradeos/tpkg/tpkgfs"
)

type Package struct {
	parent   *DBData
	startIno uint64
	pos      int64 // position in archive of data
	id       uuid.UUID
	hash     []byte // typically sha256
	size     uint64
	inodes   uint64
	name     string
	path     string // path relative to where db file was downloaded from

	// from file
	flags   uint64
	created time.Time

	dl     sync.Once
	f      *os.File
	offset int64 // offset of data in file
	squash *squashfs.Superblock
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

func (p *Package) handleLookup(ino uint64) (tpkgfs.Inode, error) {
	if ino == p.startIno {
		return tpkgfs.NewSymlink([]byte(p.name)), nil
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
	lpath := path.Join("data", p.parent.name, p.path)

	if _, err := os.Stat(lpath); os.IsNotExist(err) {
		p.dlFile()
		// TODO: check size, checksum, etc
	} else {
		f, err := os.Open(lpath)
		if err != nil {
			log.Printf("tpkgdb: failed to open: %s", err)
			return
		}
		p.f = f
	}

	err := p.validate()
	if err != nil {
		log.Printf("tpkgdb: failed to validate file: %s", err)
		defer p.f.Close()
		p.f = nil
		return
	}

	p.squash, err = squashfs.New(p, p.startIno, p.parent.fs)
	if err != nil {
		log.Printf("tpkgdb: failed to mount: %s", err)
		defer p.f.Close()
		p.f = nil
		p.squash = nil
		return
	}
}

func (p *Package) dlFile() {
	lpath := path.Join("data", p.parent.name, p.path)

	// download this package
	resp, err := http.Get(p.parent.prefix + "dist/" + p.parent.name + "/" + p.path)
	if err != nil {
		log.Printf("tpkgdb: failed to get package: %s", err)
		return
	}
	defer resp.Body.Close()

	// Need to write to file
	err = os.MkdirAll(path.Dir(lpath), 0755)
	if err != nil {
		log.Printf("tpkgdb: failed to make dir: %s", err)
		return
	}

	f, err := os.Create(lpath)
	if err != nil {
		log.Printf("tpkgdb: failed to create file: %s", err)
		return
	}

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		log.Printf("tpkgdb: failed to write file: %s", err)
		return
	}

	p.f = f
}

func (p *Package) validate() error {
	// read header, check file
	header := make([]byte, 120)
	_, err := p.f.ReadAt(header, 0)
	if err != nil {
		return err
	}

	if string(header[:4]) != "TPKG" {
		return errors.New("not a TPKG file")
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

	// read sign_offset + data_offset
	last_offt := make([]uint32, 2)
	err = binary.Read(r, binary.BigEndian, last_offt)
	if err != nil {
		return err
	}

	// TODO store all that stuff

	p.offset = int64(last_offt[1])

	return nil
}

func (p *Package) ReadAt(b []byte, off int64) (int, error) {
	if p.f == nil {
		return 0, os.ErrInvalid // should return E_IO
	}
	//log.Printf("converted read = %d", off+p.offset)

	return p.f.ReadAt(b, off+p.offset)
}
