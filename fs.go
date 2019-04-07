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
		2: &specialInodeObj{ino: 2, refcount: 999, mode: 0444},
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

	// TODO sub addref()

	out.SetEntryTimeout(time.Second)
	out.SetAttrTimeout(time.Second)
	return fuse.OK
}

func (p *pkgFS) Forget(nodeid, nlookup uint64) {
	// Forget is called when the kernel discards entries from its
	// dentry cache. This happens on unmount, and when the kernel
	// is short on memory. Since it is not guaranteed to occur at
	// any moment, and since there is no return value, Forget
	// should not do I/O, as there is no channel to report back
	// I/O errors.
	// TODO
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

func (p *pkgFS) Readlink(cancel <-chan struct{}, header *fuse.InHeader) (out []byte, code fuse.Status) {
	ino, ok := p.getInode(header.NodeId)
	if !ok {
		return nil, fuse.EINVAL
	}

	v, err := ino.Readlink()
	return v, fuse.ToStatus(err)
}

func (p *pkgFS) Open(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	// TODO
	return fuse.ENOSYS
}

func (p *pkgFS) StatFs(cancel <-chan struct{}, input *fuse.InHeader, out *fuse.StatfsOut) (code fuse.Status) {
	loadDb()

	out.Blocks = (uint64(dbMain.Length()+dbMain.PackagesSize()) / 4096) + 1
	out.Bfree = 0
	out.Bavail = 0
	out.Files = dbMain.Inodes() + 2 // root & INFO
	out.Ffree = 0
	out.Bsize = 4096
	out.NameLen = 255
	out.Frsize = 0

	return fuse.OK
}

//    Read(cancel <-chan struct{}, input *ReadIn, buf []byte) (ReadResult, Status)
//    Lseek(cancel <-chan struct{}, in *LseekIn, out *LseekOut) Status
//

// Write methods
func (p *pkgFS) SetAttr(cancel <-chan struct{}, input *fuse.SetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Mknod(cancel <-chan struct{}, input *fuse.MknodIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Rename(cancel <-chan struct{}, input *fuse.RenameIn, oldName string, newName string) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Link(cancel <-chan struct{}, input *fuse.LinkIn, filename string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Symlink(cancel <-chan struct{}, header *fuse.InHeader, pointedTo string, linkName string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *pkgFS) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) (code fuse.Status) {
	return fuse.EROFS
}

//    // File locking
//    GetLk(cancel <-chan struct{}, input *LkIn, out *LkOut) (code Status)
//    SetLk(cancel <-chan struct{}, input *LkIn) (code Status)
//    SetLkw(cancel <-chan struct{}, input *LkIn) (code Status)
//
//    Release(cancel <-chan struct{}, input *ReleaseIn)
//    Write(cancel <-chan struct{}, input *WriteIn, data []byte) (written uint32, code Status)
//    CopyFileRange(cancel <-chan struct{}, input *CopyFileRangeIn) (written uint32, code Status)
//
//    Flush(cancel <-chan struct{}, input *FlushIn) Status
//    Fsync(cancel <-chan struct{}, input *FsyncIn) (code Status)
//    Fallocate(cancel <-chan struct{}, input *FallocateIn) (code Status)
//
//    // Directory handling
//    OpenDir(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status)
//    ReadDir(cancel <-chan struct{}, input *ReadIn, out *DirEntryList) Status
//    ReadDirPlus(cancel <-chan struct{}, input *ReadIn, out *DirEntryList) Status
//    ReleaseDir(input *ReleaseIn)
//    FsyncDir(cancel <-chan struct{}, input *FsyncIn) (code Status)
//
//
//    // This is called on processing the first request. The
//    // filesystem implementation can use the server argument to
//    // talk back to the kernel (through notify methods).
//    Init(*Server)
//
//    // GetXAttr reads an extended attribute, and should return the
//    // number of bytes. If the buffer is too small, return ERANGE,
//    // with the required buffer size.
//    GetXAttr(cancel <-chan struct{}, header *InHeader, attr string, dest []byte) (sz uint32, code Status)
//
//    // ListXAttr lists extended attributes as '\0' delimited byte
//    // slice, and return the number of bytes. If the buffer is too
//    // small, return ERANGE, with the required buffer size.
//    ListXAttr(cancel <-chan struct{}, header *InHeader, dest []byte) (uint32, Status)
//
//    // SetAttr writes an extended attribute.
//    SetXAttr(cancel <-chan struct{}, input *SetXAttrIn, attr string, data []byte) Status
//
//    // RemoveXAttr removes an extended attribute.
//    RemoveXAttr(cancel <-chan struct{}, header *InHeader, attr string) (code Status)
//
