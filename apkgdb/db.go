package apkgdb

import (
	"encoding/binary"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"
	"github.com/petar/GoLLRB/llrb"
)

type DB struct {
	prefix string
	path   string
	name   string
	db     *bolt.DB
	upd    chan struct{}

	ino    *llrb.LLRB
	refcnt uint64
}

func New(prefix, name, path string) (*DB, error) {
	os.MkdirAll(path, 0755) // make sure dir exists
	db, err := bolt.Open(filepath.Join(path, name+".db"), 0600, nil)
	if err != nil {
		return nil, err
	}

	res := &DB{
		db:     db,
		prefix: prefix,
		path:   path,
		name:   name,
		ino:    llrb.New(),
		upd:    make(chan struct{}),
	}

	if res.CurrentVersion() == "" {
		log.Printf("apkgdb: no data yet, download initial database")
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
	http.Handle("/apkgdb/"+name, res)

	return res, nil
}

func (d *DB) CurrentVersion() (v string) {
	// get current version from db
	d.db.View(func(tx *bolt.Tx) error {
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
	// grab next inode id
	d.db.View(func(tx *bolt.Tx) error {
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
	return d.db.Close()
}
