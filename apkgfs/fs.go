package apkgfs

import (
	"io"
	"log"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/MagicalTux/go-fuse/fuse"
)

type PkgFS struct {
	fuse.RawFileSystem

	root       RootInode
	inodesLock sync.RWMutex
	server     *fuse.Server
	mountPoint string

	inoCache  map[uint64]Inode
	inoCacheL sync.RWMutex
}

func New(mountPoint string, root RootInode) (*PkgFS, error) {
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return nil, err
	}

	res := &PkgFS{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
		root:          root,
		mountPoint:    mountPoint,
		inoCache:      make(map[uint64]Inode),
	}

	if err := res.doMount(); err != nil {
		log.Printf("apkgfs: failed to mount: %s", err)
		return nil, err
	}

	log.Printf("apkgfs: filesystem mounted on %s", mountPoint)
	return res, nil
}

func (p *PkgFS) String() string {
	return "APkgFS"
}

func (p *PkgFS) Path() string {
	return p.mountPoint
}

func (p *PkgFS) Serve() {
	p.server.Serve()
}

func (p *PkgFS) Unmount() {
	p.server.Unmount()
}

func (p *PkgFS) Access(cancel <-chan struct{}, input *fuse.AccessIn) (code fuse.Status) {
	if input.Mask&fuse.W_OK != 0 {
		return fuse.EPERM
	}
	return fuse.OK
}

func (p *PkgFS) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) (code fuse.Status) {
	ino, err := p.getInode(header.NodeId)
	if err != nil {
		// this shouldn't be possible
		return toStatus(err)
	}

	sub, err := ino.Lookup(name)
	if err != nil {
		return toStatus(err)
	}

	subI, err := p.getInode(sub)
	if err != nil {
		return toStatus(err)
	}

	subI.AddRef(1)
	go p.addToCache(sub, subI)

	out.NodeId, out.Generation = sub, 0
	out.Ino = sub
	subI.FillAttr(&out.Attr)

	// TODO sub addref()

	out.SetEntryTimeout(30 * time.Second)
	out.SetAttrTimeout(30 * time.Second)
	return fuse.OK
}

func (p *PkgFS) Forget(nodeid, nlookup uint64) {
	// Forget is called when the kernel discards entries from its
	// dentry cache. This happens on unmount, and when the kernel
	// is short on memory. Since it is not guaranteed to occur at
	// any moment, and since there is no return value, Forget
	// should not do I/O, as there is no channel to report back
	// I/O errors.
	p.removeFromCache(nodeid, nlookup)
}

func (p *PkgFS) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	ino, err := p.getInode(input.NodeId)
	if err != nil {
		return toStatus(err)
	}

	out.Ino = input.NodeId
	ino.FillAttr(&out.Attr)
	out.SetTimeout(30 * time.Second)
	return fuse.OK
}

func (p *PkgFS) Readlink(cancel <-chan struct{}, header *fuse.InHeader) (out []byte, code fuse.Status) {
	ino, err := p.getInode(header.NodeId)
	if err != nil {
		return nil, toStatus(err)
	}

	v, err := ino.Readlink()
	return v, toStatus(err)
}

func (p *PkgFS) Open(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	if input.Flags&syscall.O_WRONLY != 0 {
		// can be readwrite or wronly
		return fuse.EROFS
	}

	// grab inode
	ino, err := p.getInode(input.NodeId)
	if err != nil {
		return toStatus(err)
	}

	// check if can open
	out.OpenFlags, err = ino.Open(input.Flags)
	if err != nil {
		return toStatus(err)
	}

	return fuse.OK
}

func (p *PkgFS) Read(cancel <-chan struct{}, input *fuse.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	ino, err := p.getInode(input.NodeId)
	if err != nil {
		return nil, toStatus(err)
	}

	r, ok := ino.(io.ReaderAt)
	if !ok {
		return nil, fuse.ENOSYS
	}

	n, err := r.ReadAt(buf, int64(input.Offset))
	if err != nil {
		return nil, toStatus(err)
	}

	return fuse.ReadResultData(buf[:n]), fuse.OK
}

//    Lseek(cancel <-chan struct{}, in *LseekIn, out *LseekOut) Status
//    Release(cancel <-chan struct{}, input *ReleaseIn)

// Directory handling
func (p *PkgFS) OpenDir(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	// directories (open is always for read only)
	// check stats → if not dir return error
	ino, err := p.getInode(input.NodeId)
	if err != nil {
		return toStatus(err)
	}

	if !ino.Mode().IsDir() {
		return fuse.ENOTDIR
	}

	out.OpenFlags, err = ino.OpenDir()
	return toStatus(err)
}

func (p *PkgFS) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	ino, err := p.getInode(input.NodeId)
	if err != nil {
		return toStatus(err)
	}

	if !ino.Mode().IsDir() {
		return fuse.ENOTDIR
	}

	err = ino.ReadDir(input, out, false)
	return toStatus(err)
}

func (p *PkgFS) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	ino, err := p.getInode(input.NodeId)
	if err != nil {
		return toStatus(err)
	}

	if !ino.Mode().IsDir() {
		return fuse.ENOTDIR
	}

	err = ino.ReadDir(input, out, true)
	return toStatus(err)
}

//    ReleaseDir(input *ReleaseIn)
//    FsyncDir(cancel <-chan struct{}, input *FsyncIn) (code Status)
//

func (p *PkgFS) StatFs(cancel <-chan struct{}, input *fuse.InHeader, out *fuse.StatfsOut) (code fuse.Status) {
	err := p.root.StatFs(out)
	return toStatus(err)
}

// Write methods
func (p *PkgFS) SetAttr(cancel <-chan struct{}, input *fuse.SetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Mknod(cancel <-chan struct{}, input *fuse.MknodIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Rename(cancel <-chan struct{}, input *fuse.RenameIn, oldName string, newName string) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Link(cancel <-chan struct{}, input *fuse.LinkIn, filename string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Symlink(cancel <-chan struct{}, header *fuse.InHeader, pointedTo string, linkName string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) SetXAttr(cancel <-chan struct{}, input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	return fuse.EROFS
}

func (p *PkgFS) RemoveXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Flush(cancel <-chan struct{}, input *fuse.FlushIn) fuse.Status {
	return fuse.OK
}

func (p *PkgFS) Fsync(cancel <-chan struct{}, input *fuse.FsyncIn) (code fuse.Status) {
	return fuse.OK
}

func (p *PkgFS) Fallocate(cancel <-chan struct{}, input *fuse.FallocateIn) (code fuse.Status) {
	return fuse.EROFS
}

func (p *PkgFS) Write(cancel <-chan struct{}, input *fuse.WriteIn, data []byte) (written uint32, code fuse.Status) {
	return 0, fuse.EROFS
}

func (p *PkgFS) CopyFileRange(cancel <-chan struct{}, input *fuse.CopyFileRangeIn) (written uint32, code fuse.Status) {
	return 0, fuse.EROFS
}

//    // File locking
//    GetLk(cancel <-chan struct{}, input *LkIn, out *LkOut) (code Status)
//    SetLk(cancel <-chan struct{}, input *LkIn) (code Status)
//    SetLkw(cancel <-chan struct{}, input *LkIn) (code Status)
//
//
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
