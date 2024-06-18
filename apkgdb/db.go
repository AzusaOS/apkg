package apkgdb

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/petar/GoLLRB/llrb"
	bolt "go.etcd.io/bbolt"
)

const PKG_URL_PREFIX = "https://data.apkg.net/"

type DB struct {
	prefix string
	path   string
	name   string
	os     string
	arch   string
	dbptr  *bolt.DB
	upd    chan struct{}

	ino    *llrb.LLRB
	inoLk  sync.RWMutex
	refcnt uint64
	dbrw   sync.RWMutex
	parent *DB // if this is called from another db

	osV   OS
	archV Arch

	nextIlk sync.RWMutex
	nextI   uint64 // next unallocated inode #
	pkgIlk  sync.RWMutex
	pkgI    map[[32]byte]uint64 // maps package hash → initial inode number
	sub     map[ArchOS]*DB
	subLk   sync.RWMutex
	ldso    []byte
}

func New(prefix, name, path string) (*DB, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	if val := os.Getenv("GOOS"); val != "" {
		goos = val
	}
	if val := os.Getenv("GOARCH"); val != "" {
		goarch = val
	}

	return NewOsArch(prefix, name, path, goos, goarch)
}

func NewOsArch(prefix, name, path, dbos, dbarch string) (*DB, error) {
	_ = os.MkdirAll(path, 0755) // make sure dir exists
	fn := filepath.Join(path, name+"."+dbos+"."+dbarch+".db")

	initUnsigned(path)

	opts := &bolt.Options{ReadOnly: true}

	if _, err := os.Stat(fn); os.IsNotExist(err) {
		opts = nil
	}

	db, err := bolt.Open(filepath.Join(path, name+"."+dbos+"."+dbarch+".db"), 0600, opts)
	if err != nil {
		return nil, err
	}

	res := &DB{
		dbptr:  db,
		prefix: prefix,
		path:   path,
		name:   name,
		os:     dbos,
		osV:    ParseOS(dbos),
		arch:   dbarch,
		archV:  ParseArch(dbarch),
		ino:    llrb.New(),
		pkgI:   make(map[[32]byte]uint64),
		nextI:  1000, // 1=root, 2=ld.so.cache
		upd:    make(chan struct{}),
		sub:    make(map[ArchOS]*DB),
	}

	res.buildLdso()

	updateReq := true

	if res.CurrentVersion() == "" {
		// need to perform download now
		_, err = res.download("")
		if err != nil {
			return nil, err
		}
		updateReq = false // since we updated now, no need for updateThread to check immediately
	}

	go res.updateThread(updateReq)

	return res, nil
}

func (d *DB) CurrentVersion() (v string) {
	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	// get current version from db
	_ = d.dbptr.View(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte("p2p")) == nil {
			// old db, needs update
			return nil
		}
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

func (d *DB) Close() error {
	d.dbrw.Lock()
	defer d.dbrw.Unlock()

	if err := d.dbptr.Close(); err != nil {
		return err
	}

	d.dbptr = nil

	return nil
}
