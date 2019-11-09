package apkgdb

import (
	"encoding/binary"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/petar/GoLLRB/llrb"
)

const PKG_URL_PREFIX = "https://pkg.azusa.jp/"

type DB struct {
	prefix string
	path   string
	name   string
	os     string
	arch   string
	dbptr  *bolt.DB
	upd    chan struct{}

	ino    *llrb.LLRB
	refcnt uint64
	dbrw   sync.RWMutex
}

func New(prefix, name, path string) (*DB, error) {
	return NewOsArch(prefix, name, path, runtime.GOOS, runtime.GOARCH)
}

func NewOsArch(prefix, name, path, dbos, dbarch string) (*DB, error) {
	os.MkdirAll(path, 0755) // make sure dir exists
	db, err := bolt.Open(filepath.Join(path, name+".db"), 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}

	res := &DB{
		dbptr:  db,
		prefix: prefix,
		path:   path,
		name:   name,
		os:     dbos,
		arch:   dbarch,
		ino:    llrb.New(),
		upd:    make(chan struct{}),
	}

	if res.CurrentVersion() == "" {
		// need to perform download now
		_, err = res.download("")
		if err != nil {
			return nil, err
		}
	} else {
		// check for updates
		err = res.update()
		if err != nil {
			log.Printf("apkgdb: failed to update: %s", err)
		}
	}

	go res.updateThread()

	return res, nil
}

func (d *DB) CurrentVersion() (v string) {
	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	// get current version from db
	d.dbptr.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("info"))
		if b == nil {
			return nil
		}
		res := b.Get([]byte("version"))

		// check if res is not nil & contains data
		if len(res) > 0 {
			// casting to string will cause a copy of the data :)
			v = string(res)
		}
		return nil
	})
	return
}

func (d *DB) nextInode() (n uint64) {
	// NOTE: d.dbrw should be locked first

	// grab next inode id
	d.dbptr.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("info"))
		if b == nil {
			n = 2 // 1 is reserved for root
			return nil
		}
		res := b.Get([]byte("next_inode"))

		// check if res is not nil & contains data
		if len(res) == 8 {
			// casting to string will cause a copy of the data :)
			n = binary.BigEndian.Uint64(res)
		} else {
			n = 2 // 1 is reserved for root
		}
		return nil
	})
	return
}

func (d *DB) Close() error {
	d.dbrw.Lock()
	defer d.dbrw.Unlock()

	if err := d.dbptr.Close(); err != nil {
		return err
	}

	d.dbptr = nil

	return nil
}
