package apkgfs

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/petar/GoLLRB/llrb"
)

type rootInodeObj struct {
	parent   *PkgFS
	children *llrb.LLRB
	chLock   sync.RWMutex
	t        time.Time
}

func (d *PkgFS) RegisterRootInode(ino uint64, name string) {
	d.root.chLock.Lock()
	defer d.root.chLock.Unlock()

	d.root.children.ReplaceOrInsert(&llrbString{k: name, v: ino})
}

func (i *rootInodeObj) fillEntry(entry *fuse.EntryOut) {
	entry.NodeId = 1
	entry.Attr.Ino = entry.NodeId
	i.FillAttr(&entry.Attr)
	entry.SetEntryTimeout(time.Second)
	entry.SetAttrTimeout(time.Second)
}

func (i *rootInodeObj) Lookup(name string) (uint64, error) {
	i.chLock.RLock()
	defer i.chLock.RUnlock()

	var ino uint64
	i.children.AscendGreaterOrEqual(&llrbString{k: name}, func(i llrb.Item) bool {
		v := i.(*llrbString)
		if v.k == name {
			ino = v.v
		}
		return false
	})

	if ino == 0 {
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
	attr.Size = 0
	attr.Blocks = 0
	attr.Mode = ModeToUnix(i.Mode())
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Blksize = 4096
	attr.Atime = uint64(i.t.Unix())
	attr.Mtime = uint64(i.t.Unix())
	attr.Ctime = uint64(i.t.Unix())
	attr.Atimensec = uint32(i.t.UnixNano())
	attr.Mtimensec = uint32(i.t.UnixNano())
	attr.Ctimensec = uint32(i.t.UnixNano())
	return nil
}

func (i *rootInodeObj) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *rootInodeObj) Open(flags uint32) error {
	return nil
}

func (i *rootInodeObj) OpenDir() error {
	return nil
}

func (i *rootInodeObj) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error {
	// for each entry
	pos := input.Offset
	cur := uint64(0)
	if pos == 0 {
		// .
		if plus {
			entry := out.AddDirLookupEntry(fuse.DirEntry{Mode: 0444, Name: ".", Ino: 1})
			i.fillEntry(entry)
		} else {
			if !out.Add(0, ".", 1, 0444) {
				return nil
			}
		}
		pos += 1
		cur += 1
	}
	if pos == 1 {
		// ..
		if plus {
			entry := out.AddDirLookupEntry(fuse.DirEntry{Mode: 0444, Name: "..", Ino: 1})
			i.fillEntry(entry)
		} else {
			if !out.Add(0, "..", 1, 0444) {
				return nil
			}
		}
		pos += 1
		cur += 1
	}
	i.children.AscendGreaterOrEqual(&llrbString{k: ""}, func(item llrb.Item) bool {
		if pos > cur {
			cur += 1
			return true
		}
		cur += 1

		v := item.(*llrbString)

		ino, err := i.parent.getInode(v.v)
		if err != nil {
			log.Printf("apkgfs: failed to get inode: %s", err)
			return false
		}

		if plus {
			entry := out.AddDirLookupEntry(fuse.DirEntry{Mode: uint32(ino.Mode().Perm()), Name: v.k, Ino: v.v})
			if entry == nil {
				return false
			}
			entry.NodeId = v.v
			entry.Attr.Ino = entry.NodeId
			ino.FillAttr(&entry.Attr)
			entry.SetEntryTimeout(time.Second)
			entry.SetAttrTimeout(time.Second)
		} else {
			if !out.Add(0, v.k, v.v, uint32(ino.Mode().Perm())) {
				return false
			}
		}
		return true
	})
	return nil
}

func (i *rootInodeObj) AddRef(count uint64) uint64 {
	// we do not actually store count
	return 999
}

func (i *rootInodeObj) DelRef(count uint64) uint64 {
	// virtual good is never good to forget
	return 999
}
