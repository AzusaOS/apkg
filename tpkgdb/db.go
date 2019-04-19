package tpkgdb

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/petar/GoLLRB/llrb"
	"github.com/tardigradeos/tpkg/tpkgfs"
)

type DB struct {
	*DBData
	upd chan struct{}
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
	path     string

	totalSize uint64
	fs        *tpkgfs.PkgFS
	inoCount  uint64
	nameIdx   *llrb.LLRB
	ino       *llrb.LLRB

	ready uint32
}

func New(prefix, name, path string, fs *tpkgfs.PkgFS) (*DB, error) {
	r := &DBData{
		prefix:  prefix,
		name:    name,
		path:    path,
		fs:      fs,
		ino:     llrb.New(),
		nameIdx: llrb.New(),
	}

	isNew := false
	if _, err := os.Stat(filepath.Join(r.path, r.name+".bin")); os.IsNotExist(err) {
		// immediate download
		_, err := r.download("")
		if err != nil {
			return nil, err
		}
		isNew = true
	}

	err := r.load()
	if err != nil {
		if !isNew {
			// may have more luck downloading again
			log.Printf("tpkgdb: failed to load existing db, will try redownload: %s", err)
			r.Close() // free any mmap resource
			_, err = r.download("")
			if err != nil {
				return nil, err
			}
			err = r.load()
			if err != nil {
				return nil, err
			}
			isNew = true
		} else {
			return nil, err
		}
	}

	res := &DB{DBData: r, upd: make(chan struct{})}

	ino, err := r.fs.AllocateInode(res)
	if err != nil {
		return nil, err
	}
	r.fs.RegisterRootInode(ino, r.name)

	if !isNew {
		// check for updates
		err = res.update()
		if err != nil {
			log.Printf("tpkgdb: failed to update: %s", err)
		}
	}

	go res.updateThread()

	return res, nil
}

func (d *DBData) load() error {
	if d.data != nil {
		return errors.New("tpkgdb: attempt to load an already loaded db")
	}

	// we use mmap
	f, err := os.Open(filepath.Join(d.path, d.name+".bin"))
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
