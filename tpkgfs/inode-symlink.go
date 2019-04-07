package tpkgfs

import (
	"os"

	"github.com/hanwen/go-fuse/fuse"
)

type symlinkInodeObj struct {
	ino    uint64
	target []byte
}

func NewSymlink(ino uint64, link []byte) Inode {
	return &symlinkInodeObj{
		ino:    ino,
		target: link,
	}
}

func (i *symlinkInodeObj) NodeId() (uint64, uint64) {
	// special nodes have generation=0
	return i.ino, 0
}

func (i *symlinkInodeObj) Lookup(name string) (Inode, error) {
	return nil, os.ErrInvalid
}

func (i *symlinkInodeObj) Mode() os.FileMode {
	return os.ModeSymlink | 0444
}

func (i *symlinkInodeObj) IsDir() bool {
	return false
}

func (i *symlinkInodeObj) FillAttr(attr *fuse.Attr) error {
	attr.Ino = i.ino
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

func (i *symlinkInodeObj) Readlink() ([]byte, error) {
	return i.target, os.ErrInvalid
}

func (i *symlinkInodeObj) Open(flags uint32) error {
	return os.ErrInvalid
}

func (i *symlinkInodeObj) OpenDir() error {
	return os.ErrInvalid
}
