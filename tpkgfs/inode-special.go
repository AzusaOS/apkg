package tpkgfs

import (
	"log"
	"os"

	"github.com/hanwen/go-fuse/fuse"
)

const (
	InodeRoot uint64 = 1
	InodeInfo uint64 = 2
)

type specialInodeObj struct {
	parent   *PkgFS
	ino      uint64
	refcount int64
	mode     os.FileMode
	children map[string]*inodeObj
}

func (i *specialInodeObj) NodeId() (uint64, uint64) {
	// special nodes have generation=0
	return i.ino, 0
}

func (i *specialInodeObj) Lookup(name string) (inodeObj, error) {
	switch i.ino {
	case InodeRoot:
		if name == "INFO" {
			// special file
			ino, _ := i.parent.getInode(InodeInfo)
			return ino, nil
		}
		log.Printf("ROOT lookup: %s", name)
	}
	return nil, os.ErrNotExist
}

func (i *specialInodeObj) Mode() os.FileMode {
	return i.mode
}

func (i *specialInodeObj) UnixMode() uint32 {
	return modeToUnix(i.Mode())
}

func (i *specialInodeObj) IsDir() bool {
	return i.mode.IsDir()
}

func (i *specialInodeObj) FillAttr(attr *fuse.Attr) error {
	attr.Ino = i.ino
	attr.Size = 4096
	attr.Blocks = 1
	attr.Mode = i.UnixMode()
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Blksize = 4096
	attr.Atimensec = 0
	attr.Mtimensec = 0
	attr.Ctimensec = 0
	return nil
}

func (i *specialInodeObj) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *specialInodeObj) Open(flags uint32) error {
	return nil
}

func (i *specialInodeObj) OpenDir() error {
	return os.ErrPermission
}
