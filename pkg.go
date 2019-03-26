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
		1: &specialInodeObj{ino: 1, refcount: 999, mode: os.ModeDir | 0755},
		2: &specialInodeObj{ino: 2, refcount: 999, mode: os.ModeDir | 0755},
		3: &specialInodeObj{ino: 3, refcount: 999, mode: os.ModeDir | 0755},
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

func (p *pkgFS) Access(input *fuse.AccessIn) (code fuse.Status) {
	if input.Mask&fuse.W_OK != 0 {
		return fuse.EPERM
	}
	return fuse.OK
}

func (p *pkgFS) Lookup(header *fuse.InHeader, name string, out *fuse.EntryOut) (code fuse.Status) {
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

	// fill attrs
	out.Attr.Ino = out.NodeId
	out.Attr.Mode = uint32(sub.Mode())
	out.Attr.Nlink = 1

	out.SetEntryTimeout(300 * time.Second)
	out.SetAttrTimeout(300 * time.Second)
	return fuse.OK
}

func (p *pkgFS) GetAttr(input *fuse.GetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	ino, ok := p.getInode(input.NodeId)
	if !ok {
		return fuse.EINVAL
	}

	out.Attr.Ino, _ = ino.NodeId()
	out.Attr.Size = 0
	out.Attr.Blocks = 1
	out.Attr.Mode = uint32(ino.Mode())
	out.Attr.Nlink = 1
	out.Attr.Rdev = 1
	out.Attr.Blksize = 512
	out.Ino = out.Attr.Ino

	out.SetTimeout(300 * time.Second)
	return fuse.OK
}
