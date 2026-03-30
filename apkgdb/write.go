package apkgdb

import (
	"log"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

func (d *DB) writeStart() error {
	d.dbrw.Lock()

	if d.dbptr != nil {
		d.dbptr.Close()
	}

	db, err := bolt.Open(filepath.Join(d.path, d.name+"."+d.os+"."+d.arch+".db"), 0600, nil)
	if err != nil {
		// failed to open, re-open
		var err2 error
		db, err2 = bolt.Open(filepath.Join(d.path, d.name+"."+d.os+"."+d.arch+".db"), 0600, &bolt.Options{ReadOnly: true})

		if err2 != nil {
			log.Printf("apkgdb: CRITICAL: failed to reopen database: %s (original error: %s)", err2, err)
			d.dbptr = nil
			d.dbrw.Unlock()
			return err
		}
		d.dbptr = db

		d.dbrw.Unlock()

		return err
	}

	d.dbptr = db

	return nil
}

func (d *DB) writeEnd() {
	if d.dbptr != nil {
		d.dbptr.Close()
	}

	db, err := bolt.Open(filepath.Join(d.path, d.name+"."+d.os+"."+d.arch+".db"), 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		log.Printf("apkgdb: CRITICAL: failed to reopen database read-only: %s", err)
		d.dbptr = nil
		d.dbrw.Unlock()
		return
	}

	d.dbptr = db

	d.dbrw.Unlock()
}
