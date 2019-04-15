package tpkgdb

import (
	"os"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/petar/GoLLRB/llrb"
	"github.com/tardigradeos/tpkg/tpkgfs"
)

func (i *DB) Lookup(name string) (uint64, error) {
	var found *Package
	i.nameIdx.AscendGreaterOrEqual(&llrbString{k: name}, func(i llrb.Item) bool {
		found = i.(*llrbString).v
		return false
	})

	if found != nil {
		if name == found.name {
			// return root (ino+1)
			return found.startIno + 1, nil
		}
		if strings.HasPrefix(found.name, name+".") {
			// next character in found.name should be a digit
			c := found.name[len(name)+1]
			if c >= '0' && c <= '9' {
				// return inode
				return found.startIno, nil
			}
		}
	}
	return 0, os.ErrNotExist
	// TODO
}

func (i *DB) Mode() os.FileMode {
	return os.ModeDir | 0444
}

func (i *DB) IsDir() bool {
	return true
}

func (i *DB) FillAttr(attr *fuse.Attr) error {
	attr.Ino = 1
	attr.Size = i.totalSize
	attr.Blocks = 1
	attr.Mode = tpkgfs.ModeToUnix(i.Mode())
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
