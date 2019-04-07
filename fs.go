package main

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

var pkgFSobj = &pkgFS{RawFileSystem: fuse.NewDefaultRawFileSystem(),
	inodeLast: 100, // values below 100 are reserved for special inodes
	inodes: map[uint64]inodeObj{
		1: &specialInodeObj{ino: 1, refcount: 999, mode: os.ModeDir | 0444},
		2: &specialInodeObj{ino: 2, refcount: 999, mode: os.ModeDir | 0444},
	},
}

type pkgFS struct {
	fuse.RawFileSystem

	inodes     map[uint64]inodeObj
	inodeLast  uint64 // last generated inode number (set to 1=root)
	inodesLock sync.RWMutex
}

func (p *pkgFS) String() string {
	return "tpkgFS"
}

// allocateInode returns a numeric ID suitable for a new inode
func (p *pkgFS) allocateInode() uint64 {
	return atomic.AddUint64(&p.inodeLast, 1)
}

func (p *pkgFS) getInode(ino uint64) (inodeObj, bool) {
	p.inodesLock.RLock()
	defer p.inodesLock.RUnlock()

	a, b := p.inodes[ino]
	return a, b
}

func (p *pkgFS) Access(cancel <-chan struct{}, input *fuse.AccessIn) (code fuse.Status) {
	if input.Mask&fuse.W_OK != 0 {
		return fuse.EPERM
	}
	return fuse.OK
}

func (p *pkgFS) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) (code fuse.Status) {
	ino, ok := p.getInode(header.NodeId)
	if !ok {
		// this shouldn't be possible
		return fuse.EINVAL
	}

	sub, err := ino.Lookup(name)
	if err != nil {
		return fuse.ToStatus(err)
	}

	out.NodeId, out.Generation = sub.NodeId()
	sub.FillAttr(&out.Attr)

	out.SetEntryTimeout(time.Second)
	out.SetAttrTimeout(time.Second)
	return fuse.OK
}

func (p *pkgFS) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	ino, ok := p.getInode(input.NodeId)
	if !ok {
		return fuse.EINVAL
	}

	ino.FillAttr(&out.Attr)
	out.SetTimeout(time.Second)
	return fuse.OK
}
