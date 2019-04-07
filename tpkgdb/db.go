package tpkgdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type DB struct {
	data     []byte
	version  uint32
	flags    uint64
	created  time.Time
	os, arch uint32
	name     string

	ino      map[uint64]*Package
	pkgName  map[string]*Package
	pkgAlias map[string]*Package
}

func New(f *os.File) (*DB, error) {
	// we use mmap
	fi, err := f.Stat()
	size := fi.Size()

	if size <= 0 {
		return nil, errors.New("tpkgdb: file size is way too low")
	}

	if size != int64(int(size)) {
		return nil, errors.New("tpkgdb: file size is over 4GB")
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}

	r := &DB{
		data:     data,
		ino:      make(map[uint64]*Package),
		pkgName:  make(map[string]*Package),
		pkgAlias: make(map[string]*Package),
	}
	runtime.SetFinalizer(r, (*DB).Close)

	err = r.index()
	if err != nil {
		r.Close()
		return nil, err
	}

	return r, nil
}

func (d *DB) Close() error {
	if d.data == nil {
		return nil
	}
	data := d.data
	d.data = nil
	runtime.SetFinalizer(d, nil)
	return syscall.Munmap(data)
}

func (d *DB) index() error {
	if string(d.data[:4]) != "TPDB" {
		return errors.New("not a tpkgdb file")
	}

	r := bytes.NewReader(d.data)
	r.Seek(4, io.SeekStart)

	// read version
	err := binary.Read(r, binary.BigEndian, &d.version)
	if err != nil {
		return err
	}
	if d.version != 1 {
		return errors.New("unsupported db version")
	}

	err = binary.Read(r, binary.BigEndian, &d.flags)
	if err != nil {
		return err
	}

	created := make([]int64, 2)
	err = binary.Read(r, binary.BigEndian, created)
	if err != nil {
		return err
	}
	d.created = time.Unix(created[0], created[1])

	log.Printf("tpkgdb: reading database generated on %s (%s ago)", d.created, time.Since(d.created))

	osarch := make([]uint32, 2)
	err = binary.Read(r, binary.BigEndian, osarch)
	if err != nil {
		return err
	}

	// TODO check values
	d.os = osarch[0]   // 0=linux 1=darwin 2=windows ...
	d.arch = osarch[1] // 0=i386 1=amd64 ...

	name := make([]byte, 32)
	_, err = io.ReadFull(r, name)
	if err != nil {
		return err
	}

	if offt := bytes.IndexByte(name, 0); offt != -1 {
		name = name[:offt]
	}
	d.name = string(name)

	// read location of indexes (unused)
	indices := make([]uint32, 2)
	err = binary.Read(r, binary.BigEndian, indices)
	if err != nil {
		return err
	}
	// TODO → use indices
	curIno := uint64(0)

	// OK now let's read each package
	for {
		var t uint8
		pos, _ := r.Seek(0, io.SeekCurrent)
		err = binary.Read(r, binary.BigEndian, &t)
		if err == io.EOF {
			// this is OK
			return nil
		}
		if err != nil {
			return err
		}
		if t != 0 {
			return errors.New("invalid data in db, couldn't open it")
		}

		pkg := &Package{
			startIno: curIno,
			pos:      pos,
		}

		// let's read the package id & other info
		_, err = io.ReadFull(r, pkg.id[:])
		if err != nil {
			return err
		}

		pkg.hash = make([]byte, 32)
		_, err = io.ReadFull(r, pkg.hash)
		if err != nil {
			return err
		}

		// read size
		err = binary.Read(r, binary.BigEndian, &pkg.size)
		if err != nil {
			return err
		}

		err = binary.Read(r, binary.BigEndian, &pkg.inodes)
		if err != nil {
			return err
		}

		// read name
		l, err := binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		name := make([]byte, l)
		_, err = io.ReadFull(r, name)
		if err != nil {
			return err
		}

		// read path
		l, err = binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		path := make([]byte, l)
		_, err = io.ReadFull(r, path)
		if err != nil {
			return err
		}
		pkg.name = string(name)
		pkg.path = string(path)

		d.ino[pkg.startIno] = pkg
		d.pkgName[pkg.name] = pkg
		aliasName := pkg.name
		for {
			p := strings.LastIndexByte(aliasName, '.')
			if p == -1 {
				break
			}
			aliasName = aliasName[:p]
			d.pkgAlias[aliasName] = pkg
		}

		log.Printf("read package %s pos=%d startIno=%d inodes=%d size=%d", pkg.name, pkg.pos, pkg.startIno, pkg.inodes, pkg.size)
		curIno += uint64(pkg.inodes)
	}

	return nil
}
