package apkgdb

import (
	"encoding/binary"
	"os"
	"sync/atomic"
	"time"

	"git.atonline.com/azusa/apkg/apkgfs"
	"github.com/boltdb/bolt"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func (i *DB) Mode() os.FileMode {
	return os.ModeDir | 0444
}

func (i *DB) IsDir() bool {
	return true
}

func (i *DB) fillEntry(entry *fuse.EntryOut) {
	entry.NodeId = 1
	entry.Attr.Ino = entry.NodeId
	i.FillAttr(&entry.Attr)
	entry.SetEntryTimeout(time.Second)
	entry.SetAttrTimeout(time.Second)
}

func (i *DB) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *DB) Open(flags uint32) (uint32, error) {
	return 0, os.ErrInvalid
}

func (i *DB) OpenDir() (uint32, error) {
	return 0, os.ErrPermission
}

func (d *DB) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error {
	pos := input.Offset + 1
	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	return d.dbptr.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("p2i"))
		var c *bolt.Cursor
		var nodeinfo []byte

		cur := uint64(0)
		for {
			cur += 1

			if cur > 2 {
				if b == nil {
					break
				}
				if c == nil {
					c = b.Cursor()
					_, nodeinfo = c.First()
				} else {
					_, nodeinfo = c.Next()
				}
				if nodeinfo == nil {
					// end of list
					break
				}
			}

			if cur < pos {
				// need to continue seeking
				continue
			}

			if cur == 1 || cur == 2 {
				// . & ..
				// we are root, so both . and .. point to inode 1
				n := "."
				if cur == 2 {
					n = ".."
				}

				if !plus {
					if !out.Add(0, n, 1, uint32(0755)) {
						break
					}
				} else {
					entry := out.AddDirLookupEntry(fuse.DirEntry{Mode: apkgfs.ModeToUnix(d.Mode()), Name: n, Ino: 1})
					if entry == nil {
						break
					}
					d.fillEntry(entry)
				}
				continue
			}

			// nodeinfo = inode + package hash + package name
			ino := binary.BigEndian.Uint64(nodeinfo[:8]) + 1
			n := nodeinfo[8+32:]

			// return value
			if !plus {
				if !out.Add(0, string(n), ino, uint32(0755)) {
					break
				}
			} else {
				entry := out.AddDirLookupEntry(fuse.DirEntry{Mode: uint32(0755), Name: string(n), Ino: ino})
				if entry == nil {
					break
				}
				d.fillEntry(entry)
			}
		}

		return nil
	})
}

func (i *DB) AddRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, count)
}

func (i *DB) DelRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, ^(count - 1))
}

func (i *DB) StatFs(out *fuse.StatfsOut) error {
	out.Blocks = (uint64(i.Length()) / 4096) + 1
	out.Bfree = 0
	out.Bavail = 0
	out.Files = i.Inodes() + 2 // root & INFO
	out.Ffree = 0
	out.Bsize = 4096
	out.NameLen = 255
	out.Frsize = 4096 // Fragment size

	return nil
}
