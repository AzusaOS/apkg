package apkgdb

import (
	"context"
	"encoding/binary"
	"os"
	"strings"

	"github.com/AzusaOS/apkg/apkgfs"
	"github.com/petar/GoLLRB/llrb"
	bolt "go.etcd.io/bbolt"
)

// Lookup resolves a package name to its inode number. It supports OS and
// architecture suffixes (e.g., "package.linux.amd64") and will delegate to
// the appropriate sub-database if the requested architecture differs from
// the current database.
func (i *DB) Lookup(ctx context.Context, name string) (n uint64, err error) {
	v := strings.LastIndexByte(name, '.')
	if v == -1 {
		// there can be no filename without a '.'
		return 0, os.ErrNotExist
	}

	// special cases
	switch name {
	case "ld.so.cache":
		return 2, nil
	}

	// name can be suffixed by cpu/OS
	// eg: azusa.symlinks.core.0.0.3.20210216.linux.amd64
	arch := ParseArch(name[v+1:])
	if arch == BadArch {
		// failed, just do normal lookup
		return i.ctxLookup(ctx, name)
	}

	sname := name[:v]
	v = strings.LastIndexByte(sname, '.')
	if v == -1 {
		// failed, just do normal lookup
		return i.ctxLookup(ctx, name)
	}
	osV := ParseOS(sname[v+1:])
	if osV == BadOS {
		// failed, just do normal lookup
		return i.ctxLookup(ctx, name)
	}
	sname = sname[:v]

	// ok we got an OS & arch
	if i.osV == osV && i.archV == arch {
		// this is us.
		n, err = i.internalLookup(name)
		if err == os.ErrNotExist {
			// if error, try without the OS/arch suffix
			n, err = i.internalLookup(sname)
		}
		return
	}

	db, err := i.SubGet(ArchOS{OS: osV, Arch: arch})
	if err != nil {
		return 0, err
	}
	n, err = db.internalLookup(name)
	if err == os.ErrNotExist {
		// if error, try without the OS/arch suffix
		n, err = db.internalLookup(sname)
	}
	return
}

func (i *DB) ctxLookup(ctx context.Context, name string) (n uint64, err error) {
	if i.parent != nil {
		return i.internalLookup(name)
	}

	return i.internalLookup(name)
}

func (i *DB) internalLookup(name string) (n uint64, err error) {
	if strings.IndexByte(name, '.') == -1 {
		// there can be no filename without a '.'
		return 0, os.ErrNotExist
	}

	if v := lookupUnsigned(i.osV, i.archV, name); v != nil {
		n, err = i.pkgInoUnsigned(v)
		if name == v.pkg {
			n += 1
		}
		return
	}

	i.dbrw.RLock()
	defer i.dbrw.RUnlock()

	err = i.dbptr.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("p2p"))
		if b == nil {
			return os.ErrNotExist
		}

		nameC := collatedVersion(name)

		v := b.Get(nameC)
		if v != nil {
			// exact match, return ino+1
			n = i.pkgIno(v) + 1

			// we need to instanciate pkg at this point
			_, err := i.getPkgTx(tx, n-1, v[:32])
			if err != nil {
				return err
			}
			return nil
		}

		// try to find value prefix
		c := b.Cursor()
		c.Seek(append(nameC, 0xff))
		k, v := c.Prev()

		if k == nil {
			return os.ErrNotExist
		}

		// compare name
		if !strings.HasPrefix(string(v[32+8:]), name+".") {
			return os.ErrNotExist
		}

		// TODO scroll to next until no match anymore so we use latest version
		// OR seek past (adding 0xff at end of string) and go prev once
		// TODO handle versionning through profiles and other methods

		n = i.pkgIno(v)
		// we need to instanciate pkg at this point
		_, err := i.getPkgTx(tx, n, v[:32])
		if err != nil {
			return err
		}

		return nil
	})

	return
}

func (i *DB) pkgIno(pkg []byte) uint64 {
	var pkgHash [32]byte
	copy(pkgHash[:], pkg[:32])

	i.pkgIlk.RLock()
	v, ok := i.pkgI[pkgHash]
	i.pkgIlk.RUnlock()

	if ok {
		return v
	}

	i.pkgIlk.Lock()
	defer i.pkgIlk.Unlock()

	v, ok = i.pkgI[pkgHash]
	if ok {
		return v
	}

	inoCnt := binary.BigEndian.Uint64(pkg[32 : 32+8])
	v = i.allocInodes(inoCnt + 1) // +1 for symlink

	i.pkgI[pkgHash] = v
	return v
}

func (i *DB) pkgInoUnsigned(p *unsignedPkg) (uint64, error) {
	if p.startIno != 0 {
		return p.startIno, nil
	}
	i.pkgIlk.Lock()
	defer i.pkgIlk.Unlock()

	if p.startIno != 0 {
		return p.startIno, nil
	}

	v := i.allocInodes(p.inodes + 1) // +1 for symlink
	p.startIno = v
	p.squash.SetInodeOffset(v)

	if i.parent != nil {
		i.parent.ino.ReplaceOrInsert(p)
	} else {
		i.ino.ReplaceOrInsert(p)
	}

	return v, nil
}

// GetInode returns the filesystem inode for the given inode number.
// Special inodes: 1 = root directory, 2 = ld.so.cache.
func (d *DB) GetInode(reqino uint64) (apkgfs.Inode, error) {
	var val pkgindexItem

	switch reqino {
	case 1: // root
		// shouldn't happen
		return d, nil
	case 2: // ld.so.cache
		return &ldsoIno{d: d, ldso: d.ldso}, nil
	}

	// check if we have this in loaded cache
	d.ino.DescendLessOrEqual(pkgindex(reqino), func(i llrb.Item) bool {
		val = i.(pkgindexItem)
		return false
	})

	switch pkg := val.(type) {
	case *Package:
		if pkg != nil && reqino < pkg.startIno+pkg.inodes+1 {
			return pkg.handleLookup(reqino)
		}
	case *unsignedPkg:
		if pkg != nil && reqino < pkg.startIno+pkg.inodes+1 {
			return pkg.handleLookup(reqino)
		}
	}

	return nil, os.ErrInvalid
}

func (d *DB) nextInode() (n uint64) {
	if d.parent != nil {
		return d.parent.nextInode()
	}

	d.nextIlk.RLock()
	defer d.nextIlk.RUnlock()
	return d.nextI
}

func (d *DB) allocInodes(c uint64) uint64 {
	if d.parent != nil {
		return d.parent.allocInodes(c)
	}

	d.nextIlk.Lock()
	defer d.nextIlk.Unlock()

	r := d.nextI
	d.nextI += c
	return r
}
