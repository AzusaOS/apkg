package main

import (
	"log"
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

type inodeObj interface {
	//os.FileInfo

	NodeId() (uint64, uint64) // NodeId, Generation
	Mode() os.FileMode
	UnixMode() uint32
	Lookup(name string) (inodeObj, error)
	FillAttr(attr *fuse.Attr) error
}

const (
	InodeRoot uint64 = 1
	InodeInfo uint64 = 2
)

type specialInodeObj struct {
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
		log.Printf("ROOT lookup: %s", name)
	}
	return nil, os.ErrNotExist
}

func (i *specialInodeObj) Mode() os.FileMode {
	return i.mode
}

func (i *specialInodeObj) UnixMode() uint32 {
	mode := i.Mode()
	res := uint32(mode.Perm())

	switch {
	case mode&os.ModeDir == os.ModeDir:
		res |= syscall.S_IFDIR
	case mode&os.ModeCharDevice == os.ModeCharDevice:
		res |= syscall.S_IFCHR
	case mode&os.ModeDevice == os.ModeDevice:
		res |= syscall.S_IFBLK
	case mode&os.ModeNamedPipe == os.ModeNamedPipe:
		res |= syscall.S_IFIFO
	case mode&os.ModeSymlink == os.ModeSymlink:
		res |= syscall.S_IFLNK
	case mode&os.ModeSocket == os.ModeSocket:
		res |= syscall.S_IFSOCK
	default:
		res |= syscall.S_IFREG
	}
	return res
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
