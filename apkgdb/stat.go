package apkgdb

import "github.com/boltdb/bolt"

func (d *DB) Length() (sz uint64) {
	d.db.View(func(tx *bolt.Tx) error {
		sz = uint64(tx.Size())
		return nil
	})
	return
}

func (d *DB) Inodes() uint64 {
	return d.inoCount
}

func (d *DB) PackagesSize() uint64 {
	return 0
}
