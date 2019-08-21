package apkgdb

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"sync/atomic"

	"git.atonline.com/azusa/apkg/apkgfs"
	"github.com/boltdb/bolt"
	"github.com/hanwen/go-fuse/fuse"
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

		c := b.Cursor()

		nameC := collatedVersion(name)
		k, v := c.Seek(nameC)

		if k == nil {
			return os.ErrNotExist
		}

		if bytes.Equal(k, nameC) {
			// found exact, return ino+1
			n = binary.BigEndian.Uint64(v) + 1
			return nil
		}

		// rewind one
		k, v = c.Prev()
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

func (i *DB) Mode() os.FileMode {
	return os.ModeDir | 0444
}

func (i *DB) IsDir() bool {
	return true
}

func (i *DB) FillAttr(attr *fuse.Attr) error {
	attr.Ino = 1
	attr.Size = 4096
	attr.Blocks = 1
	attr.Mode = apkgfs.ModeToUnix(i.Mode())
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Blksize = 4096
	attr.Atimensec = 0
	attr.Mtimensec = 0
	attr.Ctimensec = 0
	return nil
}

func (i *DB) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *DB) Open(flags uint32) error {
	return os.ErrInvalid
}

func (i *DB) OpenDir() error {
	return os.ErrPermission
}

func (i *DB) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error {
	return os.ErrInvalid
}

func (i *DB) AddRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, count)
}

func (i *DB) DelRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, ^(count - 1))
}
