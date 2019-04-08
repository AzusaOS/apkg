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

	"github.com/google/uuid"
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

	dl     sync.Once
	f      *os.File
	offset uint64 // offset of data in file
}

func (p *Package) handleLookup(ino uint64) (tpkgfs.Inode, bool) {
	p.dl.Do(p.doDl)
	log.Printf("inode lookup WIP %d %s", ino, p.name)

	return nil, false
}

func (p *Package) doDl() {
	lpath := path.Join("data", p.parent.name, p.path)

	if _, err := os.Stat(lpath); err == nil {
		// TODO: check size, checksum, etc
		return
	}

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

	err = p.validate()
	if err != nil {
		log.Printf("tpkgdb: failed to validate file: %s", err)
		p.f = nil
		f.Close()
		return
	}
}

func (p *Package) validate() error {
	// read header, check file
	header := make([]byte, 120)
	_, err := p.f.ReadAt(header, 0)
	if err != nil {
		return err
	}

	if string(header[:3]) != "TPKG" {
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

	return nil
}
