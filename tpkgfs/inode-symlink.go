package tpkgfs

import (
	"os"

	"github.com/hanwen/go-fuse/fuse"
)

type symlinkInodeObj []byte

func NewSymlink(link []byte) Inode {
	return symlinkInodeObj(link)
}

func (i symlinkInodeObj) Lookup(name string) (uint64, error) {
	return 0, os.ErrInvalid
}

func (i symlinkInodeObj) Mode() os.FileMode {
	return os.ModeSymlink | 0444
}

func (i symlinkInodeObj) IsDir() bool {
	return false
}

func (i symlinkInodeObj) FillAttr(attr *fuse.Attr) error {
	attr.Size = 4096
	attr.Blocks = 1
	attr.Mode = modeToUnix(i.Mode())
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Blksize = 4096
	attr.Atimensec = 0
	attr.Mtimensec = 0
	attr.Ctimensec = 0
	return nil
}

func (i symlinkInodeObj) Readlink() ([]byte, error) {
	return []byte(i), nil
}

func (i symlinkInodeObj) Open(flags uint32) error {
	return os.ErrInvalid
}

func (i symlinkInodeObj) OpenDir() error {
	return os.ErrInvalid
}