package tpkgdb

import (
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

	p.validate()
}

func (p *Package) validate() {
	// read header, check file
}
