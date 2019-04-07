package tpkgdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type DB struct {
	*DBData
}

type DBData struct {
	prefix   string
	name     string
	data     []byte
	version  uint32
	flags    uint64
	created  time.Time
	os, arch uint32
	count    uint32

	totalSize uint64
	inoCount  uint64

	ready uint32

	ino      map[uint64]*Package
	pkgName  map[string]*Package
	pkgAlias map[string]*Package
}

func New(prefix, name string) (*DB, error) {
	r := &DBData{
		prefix:   prefix,
		name:     name,
		ino:      make(map[uint64]*Package),
		pkgName:  make(map[string]*Package),
		pkgAlias: make(map[string]*Package),
	}

	isNew := false
	if _, err := os.Stat(r.name + ".bin"); os.IsNotExist(err) {
		// immediate download
		_, err := r.download("")
		if err != nil {
			return nil, err
		}
		isNew = true
	}

	err := r.load()
	if err != nil {
		return nil, err
	}

	res := &DB{r}

	if !isNew {
		// check for updates
		err = res.Update()
		if err != nil {
			log.Printf("tpkgdb: failed to update: %s", err)
		}
	}

	return res, nil
}

func (d *DBData) load() error {
	if d.data != nil {
		return errors.New("tpkgdb: attempt to load an already loaded db")
	}

	// we use mmap
	f, err := os.Open(d.name + ".bin")
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	size := fi.Size()

	if size <= 0 {
		return errors.New("tpkgdb: file size is way too low")
	}

	if size != int64(int(size)) {
		return errors.New("tpkgdb: file size is over 4GB")
	}

	runtime.SetFinalizer(d, (*DBData).Close)
	d.data, err = syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	err = d.index()
	if err != nil {
		d.Close()
		return err
	}

	return nil
}

func (d *DB) Close() error {
	d.DBData = nil
	return nil
}

func (d *DBData) Close() error {
	if d.data == nil {
		return nil
	}
	data := d.data
	d.data = nil
	runtime.SetFinalizer(d, nil)
	return syscall.Munmap(data)
}

func (d *DBData) index() error {
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

	osarchcnt := make([]uint32, 3)
	err = binary.Read(r, binary.BigEndian, osarchcnt)
	if err != nil {
		return err
	}

	// TODO check values
	d.os = osarchcnt[0]   // 0=linux 1=darwin 2=windows ...
	d.arch = osarchcnt[1] // 0=i386 1=amd64 ...
	d.count = osarchcnt[2]

	name := make([]byte, 32)
	_, err = io.ReadFull(r, name)
	if err != nil {
		return err
	}

	if offt := bytes.IndexByte(name, 0); offt != -1 {
		name = name[:offt]
	}
	if string(name) != d.name {
		return fmt.Errorf("invalid database, was expecting %s but downloaded database was for %s", d.name, name)
	}

	// read location of indexes (unused)
	indices := make([]uint32, 2)
	err = binary.Read(r, binary.BigEndian, indices)
	if err != nil {
		return err
	}
	// TODO → use indices

	// OK now let's read each package
	for i := uint32(0); i < d.count; i++ {
		var t uint8
		pos, _ := r.Seek(0, io.SeekCurrent)
		err = binary.Read(r, binary.BigEndian, &t)
		if err != nil {
			return err
		}
		if t != 0 {
			return errors.New("invalid data in db, couldn't open it")
		}

		pkg := &Package{
			startIno: d.inoCount,
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
		first := true
		for {
			p := strings.LastIndexByte(aliasName, '.')
			if p == -1 {
				break
			}
			if !first {
				d.pkgAlias[aliasName] = pkg
			} else {
				first = false
			}
			aliasName = aliasName[:p]
		}

		//log.Printf("read package %s size=%d", pkg.name, pkg.size)
		d.inoCount += uint64(pkg.inodes)
		d.totalSize += pkg.size
	}

	return nil
}
