package apkgfs

func (p *PkgFS) addToCache(ino uint64, o Inode) {
	p.inoCacheL.Lock()
	defer p.inoCacheL.Unlock()

	c, ok := p.inoCache[ino]
	if !ok {
		// perfect
		p.inoCache[ino] = o
		return
	}

	// mmh
	v := o.AddRef(0)
	if v != 0 {
		c.AddRef(v)
		o.DelRef(v)
	}
}

func (p *PkgFS) getInodeCache(nodeid uint64) (Inode, bool) {
	p.inoCacheL.RLock()
	defer p.inoCacheL.RUnlock()

	c, ok := p.inoCache[nodeid]
	return c, ok
}

func (p *PkgFS) removeFromCache(nodeid, nlookup uint64) {
	c, ok := p.getInodeCache(nodeid)
	if !ok {
		return
	}

	v := c.DelRef(nlookup)

	// also checking if over large value in case of overflow
	if v == 0 || v > 0xffffffffffff {
		// need to forget inode
		p.inoCacheL.Lock()
		defer p.inoCacheL.Unlock()

		delete(p.inoCache, nodeid)
	}
}
