package apkgdb

import bolt "go.etcd.io/bbolt"

func (d *DB) Length() (sz uint64) {
	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	d.dbptr.View(func(tx *bolt.Tx) error {
		sz = uint64(tx.Size())
		return nil
	})
	return
}

func (d *DB) Inodes() uint64 {
	return d.nextInode() - 1
}

func (d *DB) PackagesSize() uint64 {
	return 0
}
