package apkgdb

import (
	"encoding/binary"
	"os"
	"strings"

	"git.atonline.com/azusa/apkg/apkgfs"
	"github.com/boltdb/bolt"
	"github.com/petar/GoLLRB/llrb"
)

func (i *DB) Lookup(name string) (n uint64, err error) {
	if strings.IndexByte(name, '.') == -1 {
		return 0, os.ErrNotExist
	}

	err = i.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("p2i"))
		if b == nil {
			return os.ErrNotExist
		}

		nameC := collatedVersion(name)

		v := b.Get(nameC)
		if v != nil {
			// exact match, return ino+1
			n = binary.BigEndian.Uint64(v) + 1
			return nil
		}

		// try to find value prefix
		c := b.Cursor()
		k, v := c.Seek(nameC)

		if k == nil {
			return os.ErrNotExist
		}

		// compare name
		if !strings.HasPrefix(string(v[8+32:]), name+".") {
			return os.ErrNotExist
		}

		n = binary.BigEndian.Uint64(v)
		return nil
	})

	return
}

func (d *DB) GetInode(reqino uint64) (apkgfs.Inode, error) {
	var pkg *Package

	// check if we have this in loaded cache
	d.ino.DescendLessOrEqual(pkgindex(reqino), func(i llrb.Item) bool {
		pkg = i.(*Package)
		return false
	})
	if pkg != nil {
		return pkg.handleLookup(reqino)
	}

	// load from database
	err := d.db.View(func(tx *bolt.Tx) error {
		// TODO
		return nil
	})

	if err != nil {
		return nil, err
	}

	if pkg != nil {
		return pkg.handleLookup(reqino)
	}

	return nil, os.ErrInvalid
}
