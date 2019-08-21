package apkgdb

import (
	"encoding/binary"
	"log"
	"os"
	"path/filepath"

	"git.atonline.com/azusa/apkg/apkgfs"
	"github.com/boltdb/bolt"
	"github.com/petar/GoLLRB/llrb"
)

type DB struct {
	prefix string
	path   string
	name   string
	db     *bolt.DB
	upd    chan struct{}

	fs  *apkgfs.PkgFS
	ino *llrb.LLRB
}

func New(prefix, name, path string, fs *apkgfs.PkgFS) (*DB, error) {
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
		fs:     fs,
		ino:    llrb.New(),
		upd:    make(chan struct{}),
	}

	if res.CurrentVersion() == "" {
		log.Printf("apkgdb: no data yet, will download")
		// need to perform download now
		_, err = res.download("")
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func (d *DB) CurrentVersion() (v string) {
	// get current version from db
	d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("info"))
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

/*
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
			log.Printf("apkgdb: failed to update: %s", err)
		}
	}

	http.Handle("/apkgdb/"+name, res)

	go res.updateThread()

	return res, nil
}

func (d *DBData) Close() error {
	if d.data == nil {
		return nil
	}
	data := d.data
	d.data = nil
	runtime.SetFinalizer(d, nil)
	return syscall.Munmap(data)
}*/

func (d *DB) Close() error {
	return d.db.Close()
}
