package apkgdb

import (
	"bytes"
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
		c.Seek(append(nameC, 0xff))
		k, v := c.Prev()

		if k == nil {
			return os.ErrNotExist
		}

		// compare name
		if !strings.HasPrefix(string(v[8+32:]), name+".") {
			return os.ErrNotExist
		}

		// TODO scroll to next until no match anymore so we use latest version
		// OR seek past (adding 0xff at end of string) and go prev once

		n = binary.BigEndian.Uint64(v)
		return nil
	})

	return
}

func (d *DB) GetInode(reqino uint64) (apkgfs.Inode, error) {
	var pkg *Package

	if reqino == 1 {
		// shouldn't happen
		return d, nil
	}

	// check if we have this in loaded cache
	d.ino.DescendLessOrEqual(pkgindex(reqino), func(i llrb.Item) bool {
		pkg = i.(*Package)
		return false
	})
	if pkg != nil && reqino < pkg.startIno+pkg.inodes {
		return pkg.handleLookup(reqino)
	}

	var ino apkgfs.Inode

	// load from database
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("i2p")) // inode to package
		if b == nil {
			return os.ErrInvalid // nothing yet
		}

		inoBin := make([]byte, 8)
		binary.BigEndian.PutUint64(inoBin, reqino)

		c := b.Cursor()

		k, v := c.Seek(inoBin)

		if k != nil {
			if bytes.Equal(k, inoBin) {
				// exact match means we should return a symlink
				// note: we duplicate the value because bolt
				ino = apkgfs.NewSymlink(bytesDup(v[32+8:]))
				return nil
			}
		}

		// unless we managed to seek exactly where we wanted, we should go back a bit
		k, v = c.Prev()

		if k == nil {
			return os.ErrInvalid
		}

		// get startIno + inoCount
		startIno := binary.BigEndian.Uint64(k)
		inoCount := binary.BigEndian.Uint64(v[32:40])

		// check range
		if reqino <= startIno || reqino > startIno+inoCount {
			return os.ErrInvalid
		}

		// load package
		var err error
		pkg, err = d.getPkgTx(tx, v[:32])
		return err
	})

	if err != nil {
		return nil, err
	}

	if ino != nil {
		return ino, nil
	}

	if pkg != nil {
		return pkg.handleLookup(reqino)
	}

	return nil, os.ErrInvalid
}
