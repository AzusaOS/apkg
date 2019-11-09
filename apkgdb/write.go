package apkgdb

import (
	"path/filepath"

	"github.com/boltdb/bolt"
)

func (d *DB) writeStart() error {
	d.dbrw.Lock()

	d.dbptr.Close()

	db, err := bolt.Open(filepath.Join(d.path, d.name+".db"), 0600, nil)
	if err != nil {
		// failed to open, re-open
		var err2 error
		db, err2 = bolt.Open(filepath.Join(d.path, d.name+".db"), 0600, &bolt.Options{ReadOnly: true})

		if err2 != nil {
			// can't help this anymore :(
			panic(err2)
		}
		d.dbptr = db

		d.dbrw.Unlock()

		return err
	}

	d.dbptr = db

	return nil
}

func (d *DB) writeEnd() {
	d.dbptr.Close()

	db, err := bolt.Open(filepath.Join(d.path, d.name+".db"), 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		// can't be helped
		panic(err)
	}

	d.dbptr = db

	d.dbrw.Unlock()
}
