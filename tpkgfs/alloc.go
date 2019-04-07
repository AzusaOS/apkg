package tpkgfs

import (
	"log"
	"sync/atomic"
)

type inodeR struct {
	count  uint64
	lookup func(ino uint64) (Inode, error)
}

// allocateInode returns a numeric ID suitable for a new inode
func (p *PkgFS) allocateInode() uint64 {
	return atomic.AddUint64(&p.inodeLast, 1)
}

func (p *PkgFS) getInode(ino uint64) (Inode, bool) {
	p.inodesLock.RLock()
	defer p.inodesLock.RUnlock()

	a, b := p.inodes[ino]
	return a, b
}

func (p *PkgFS) AllocateInodes(count uint64, lookup func(ino uint64) (Inode, error)) (uint64, error) {
	// allocate count number of inodes
	lastIno := atomic.AddUint64(&p.inodeLast, count)
	firstIno := lastIno - count

	log.Printf("tpkgfs: allocated %d inodes starting %d", count, firstIno)

	p.inodesLock.Lock()
	defer p.inodesLock.Unlock()

	p.inodesRange[firstIno] = &inodeR{count: count, lookup: lookup}

	return firstIno, nil
}
