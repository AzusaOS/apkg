package tpkgfs

import (
	"log"
	"os"
	"sync/atomic"

	"github.com/petar/GoLLRB/llrb"
)

type inodeR struct {
	start  uint64
	count  uint64
	lookup func(ino uint64) (Inode, error)
}

type inodeRindex uint64

func (i *inodeR) Less(than llrb.Item) bool {
	return i.start < than.(inodeRitem).Value()
}

func (i inodeRindex) Less(than llrb.Item) bool {
	return uint64(i) < than.(inodeRitem).Value()
}

type inodeRitem interface {
	Value() uint64
	Less(than llrb.Item) bool
}

func (i *inodeR) Value() uint64 {
	return i.start
}

func (i inodeRindex) Value() uint64 {
	return uint64(i)
}

// allocateInode returns a numeric ID suitable for a new inode
func (p *PkgFS) allocateInode() uint64 {
	return atomic.AddUint64(&p.inodeLast, 1)
}

func (p *PkgFS) getInode(ino uint64) (Inode, error) {
	p.inodesLock.RLock()
	defer p.inodesLock.RUnlock()

	if ino == 1 {
		return p.root, nil
	}

	a, b := p.inodes[ino]
	if b {
		return a, nil
	}

	var res *inodeR

	p.inodesIdx.DescendLessOrEqual(inodeRindex(ino), func(i llrb.Item) bool {
		res = i.(*inodeR)
		return false
	})

	if res != nil {
		if ino >= res.start && ino < res.start+res.count {
			return res.lookup(ino)
		}
	}

	return nil, os.ErrInvalid
}

func (p *PkgFS) AllocateInodes(count uint64, lookup func(ino uint64) (Inode, error)) (uint64, error) {
	// allocate count number of inodes
	lastIno := atomic.AddUint64(&p.inodeLast, count)
	firstIno := lastIno - count

	log.Printf("tpkgfs: allocated %d inodes starting %d", count, firstIno)

	p.inodesLock.Lock()
	defer p.inodesLock.Unlock()

	p.inodesIdx.ReplaceOrInsert(&inodeR{start: firstIno, count: count, lookup: lookup})

	return firstIno, nil
}
