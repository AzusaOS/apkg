package tpkgfs

import (
	"log"
	"os"

	"github.com/hanwen/go-fuse/fuse"
)

type rootInodeObj struct {
	parent   *PkgFS
	children map[string]uint64
}

func (d *PkgFS) RegisterRootInode(ino uint64, name string) {
	d.root.children[name] = ino
}

func (i *rootInodeObj) Lookup(name string) (uint64, error) {
	log.Printf("ROOT lookup: %s", name)
	ino, ok := i.children[name]
	if !ok {
		return 0, os.ErrNotExist
	}
	return ino, nil
}

func (i *rootInodeObj) Mode() os.FileMode {
	return os.ModeDir | 0444
}

func (i *rootInodeObj) IsDir() bool {
	return true
}

func (i *rootInodeObj) FillAttr(attr *fuse.Attr) error {
	attr.Ino = 1
	attr.Size = 4096
	attr.Blocks = 1
	attr.Mode = ModeToUnix(i.Mode())
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Blksize = 4096
	attr.Atimensec = 0
	attr.Mtimensec = 0
	attr.Ctimensec = 0
	return nil
}

func (i *rootInodeObj) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *rootInodeObj) Open(flags uint32) error {
	return nil
}

func (i *rootInodeObj) OpenDir() error {
	return os.ErrPermission
}
