// Package apkgdb provides the package database functionality for apkg,
// including package indexing, lookup, and management of signed packages.
package apkgdb

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/petar/GoLLRB/llrb"
	bolt "go.etcd.io/bbolt"
)

// ErrDatabaseClosed is returned when an operation is attempted on a closed or failed database.
var ErrDatabaseClosed = errors.New("database is closed")

// PKG_URL_PREFIX is the default URL prefix for downloading packages and databases.
const PKG_URL_PREFIX = "https://data.apkg.net/"

// DB represents a package database that manages package metadata, lookups,
// and inode allocation for the FUSE filesystem. It uses BoltDB for persistent
// storage and supports multiple OS/architecture combinations through sub-databases.
type DB struct {
	prefix string
	path   string
	name   string
	os     string
	arch   string
	dbptr  *bolt.DB
	upd    chan struct{}
	done   chan struct{}

	ino    *llrb.LLRB
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
	ntgt    atomic.Value // stores NotifyTarget
	ldso    []byte
	channel string // release channel for version resolution ("latest" = no pins)
}

// New creates a new package database using the current system's OS and architecture.
// It opens or creates a BoltDB database file at the specified path.
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

// NewOsArch creates a new package database for a specific OS and architecture.
// This is used both for the primary database and for cross-architecture sub-databases.
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
		done:   make(chan struct{}),
		sub:    make(map[ArchOS]*DB),
	}

	_ = res.buildLdso()

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

// SetChannel sets the release channel used for version resolution.
// Use "latest" to always resolve to the newest version (no pins).
func (d *DB) SetChannel(ch string) {
	d.channel = ch
	d.subLk.RLock()
	for _, sub := range d.sub {
		sub.channel = ch
	}
	d.subLk.RUnlock()
}

// CurrentVersion returns the version string of the currently loaded database,
// or an empty string if no version is set.
func (d *DB) CurrentVersion() (v string) {
	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	if d.dbptr == nil {
		return ""
	}

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

// Close closes the underlying BoltDB database and all sub-databases.
func (d *DB) Close() error {
	// Close all sub-databases first
	d.subLk.Lock()
	for k, sub := range d.sub {
		sub.Close()
		delete(d.sub, k)
	}
	d.subLk.Unlock()

	d.dbrw.Lock()
	defer d.dbrw.Unlock()

	close(d.done)

	if d.dbptr != nil {
		if err := d.dbptr.Close(); err != nil {
			return err
		}
	}

	d.dbptr = nil

	return nil
}
