package apkgfs

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// FuseServer handles the raw FUSE protocol over /dev/fuse.
// Unlike go-fuse's Server, the Fd field is public to support graceful exec.
type FuseServer struct {
	Fd         int    // /dev/fuse file descriptor
	MountPoint string // real filesystem mount point

	fs       *PkgFS
	maxWrite uint32
	bufSize  int
	writeMu  sync.Mutex
	done     chan struct{}
}

// newFuseServer creates a FuseServer wrapping an already-opened and mounted fd.
func newFuseServer(fd int, mountPoint string, fs *PkgFS) *FuseServer {
	return &FuseServer{
		Fd:         fd,
		MountPoint: mountPoint,
		fs:         fs,
		done:       make(chan struct{}),
	}
}

// Serve runs the FUSE event loop, reading requests from the kernel and dispatching them.
// It blocks until the filesystem is unmounted or an unrecoverable error occurs.
func (s *FuseServer) Serve() {
	if err := s.handleInit(); err != nil {
		log.Printf("apkgfs: FUSE init failed: %s", err)
		return
	}

	log.Printf("apkgfs: FUSE server ready (maxWrite=%d)", s.maxWrite)

	var wg sync.WaitGroup
	for {
		buf := make([]byte, s.bufSize)
		n, err := syscall.Read(s.Fd, buf)
		if err != nil {
			if err == syscall.ENODEV {
				// filesystem unmounted
				break
			}
			if err == syscall.EINTR {
				continue
			}
			log.Printf("apkgfs: read error: %s", err)
			break
		}
		if n < inHeaderSize {
			log.Printf("apkgfs: short read (%d bytes)", n)
			continue
		}

		wg.Add(1)
		go func(data []byte) {
			defer wg.Done()
			s.handleRequest(data)
		}(buf[:n])
	}

	wg.Wait()
	close(s.done)
}

// handleInit performs the FUSE INIT handshake.
func (s *FuseServer) handleInit() error {
	buf := make([]byte, inHeaderSize+int(unsafe.Sizeof(fuseInitIn{})))
	n, err := syscall.Read(s.Fd, buf)
	if err != nil {
		return fmt.Errorf("read INIT: %w", err)
	}
	if n < inHeaderSize {
		return fmt.Errorf("short INIT read: %d bytes", n)
	}

	hdr := (*fuseInHeader)(unsafe.Pointer(&buf[0]))
	if hdr.Opcode != opInit {
		return fmt.Errorf("expected INIT opcode, got %d", hdr.Opcode)
	}

	initIn := (*fuseInitIn)(unsafe.Pointer(&buf[inHeaderSize]))
	if initIn.Major != 7 {
		return fmt.Errorf("unsupported FUSE major version %d", initIn.Major)
	}
	if initIn.Minor < 12 {
		return fmt.Errorf("FUSE minor version %d too old (need >= 12)", initIn.Minor)
	}

	minor := initIn.Minor
	if minor > 28 {
		minor = 28
	}

	s.maxWrite = 131072 // 128 KiB
	maxPages := uint16((s.maxWrite - 1) / 4096 + 1)
	s.bufSize = int(s.maxWrite) + inHeaderSize + 256 // room for headers + filename

	// Negotiate capabilities
	wantFlags := uint32(capAsyncRead | capBigWrites | capFileOps | capAutoInvalData |
		capReaddirplus | capReaddirplusAuto | capMaxPages | capCacheSymlinks |
		capParallelDirops | capAsyncDio)
	flags := wantFlags & initIn.Flags

	out := fuseInitOut{
		Major:               7,
		Minor:               minor,
		MaxReadAhead:        initIn.MaxReadAhead,
		Flags:               flags,
		MaxBackground:       12,
		CongestionThreshold: 9,
		MaxWrite:            s.maxWrite,
		TimeGran:            1,
		MaxPages:            maxPages,
	}

	return s.reply(hdr.Unique, 0, (*[unsafe.Sizeof(fuseInitOut{})]byte)(unsafe.Pointer(&out))[:])
}

// handleRequest parses and dispatches a single FUSE request.
func (s *FuseServer) handleRequest(data []byte) {
	hdr := (*fuseInHeader)(unsafe.Pointer(&data[0]))
	body := data[inHeaderSize:]

	switch hdr.Opcode {
	case opLookup:
		s.doLookup(hdr, body)
	case opForget:
		s.doForget(hdr, body)
	case opBatchForget:
		s.doBatchForget(hdr, body)
	case opGetattr:
		s.doGetAttr(hdr, body)
	case opReadlink:
		s.doReadlink(hdr, body)
	case opOpen:
		s.doOpen(hdr, body)
	case opRead:
		s.doRead(hdr, body)
	case opRelease, opReleasedir:
		// no-op, no response needed for release
	case opStatfs:
		s.doStatFs(hdr, body)
	case opOpendir:
		s.doOpenDir(hdr, body)
	case opReaddir:
		s.doReadDir(hdr, body, false)
	case opReaddirplus:
		s.doReadDir(hdr, body, true)
	case opAccess:
		s.doAccess(hdr, body)
	case opFlush, opFsync, opFsyncdir:
		s.replyStatus(hdr.Unique, 0) // OK
	case opDestroy:
		// filesystem unmounting
	case opInterrupt:
		// we don't track cancellation — just ignore
	case opSetattr, opMknod, opMkdir, opUnlink, opRmdir, opRename, opRename2,
		opLink, opSymlink, opCreate, opWrite, opSetxattr, opRemovexattr,
		opFallocate, opCopyFileRange:
		s.replyStatus(hdr.Unique, int32(syscall.EROFS))
	default:
		s.replyStatus(hdr.Unique, int32(syscall.ENOSYS))
	}
}

func (s *FuseServer) doLookup(hdr *fuseInHeader, body []byte) {
	// body is null-terminated filename
	name := cstring(body)

	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	ctx := context.WithValue(context.Background(), Pid, hdr.Pid)
	sub, err := ino.Lookup(ctx, name)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	subI, err := s.fs.getInode(sub)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	subI.AddRef(1)
	s.fs.addToCache(sub, subI)

	var out fuse.EntryOut
	out.NodeId = sub
	out.Generation = 0
	out.Ino = sub
	if err := subI.FillAttr(&out.Attr); err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}
	out.SetEntryTimeout(30 * time.Second)
	out.SetAttrTimeout(30 * time.Second)

	s.reply(hdr.Unique, 0, entryOutBytes(&out))
}

func (s *FuseServer) doForget(hdr *fuseInHeader, body []byte) {
	if len(body) < 8 {
		return
	}
	nlookup := binary.LittleEndian.Uint64(body[:8])
	s.fs.removeFromCache(hdr.NodeId, nlookup)
	// FORGET has no reply
}

func (s *FuseServer) doBatchForget(hdr *fuseInHeader, body []byte) {
	if len(body) < 8 {
		return
	}
	count := binary.LittleEndian.Uint32(body[:4])
	body = body[8:] // skip count + dummy
	for i := uint32(0); i < count; i++ {
		if len(body) < 16 {
			break
		}
		nodeId := binary.LittleEndian.Uint64(body[:8])
		nlookup := binary.LittleEndian.Uint64(body[8:16])
		s.fs.removeFromCache(nodeId, nlookup)
		body = body[16:]
	}
	// BATCH_FORGET has no reply
}

func (s *FuseServer) doGetAttr(hdr *fuseInHeader, body []byte) {
	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	var out fuse.AttrOut
	out.Attr.Ino = hdr.NodeId
	if err := ino.FillAttr(&out.Attr); err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}
	out.SetTimeout(30 * time.Second)

	s.reply(hdr.Unique, 0, attrOutBytes(&out))
}

func (s *FuseServer) doReadlink(hdr *fuseInHeader, body []byte) {
	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	target, err := ino.Readlink()
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}
	s.reply(hdr.Unique, 0, target)
}

func (s *FuseServer) doOpen(hdr *fuseInHeader, body []byte) {
	if len(body) < 8 {
		s.replyStatus(hdr.Unique, int32(syscall.EINVAL))
		return
	}
	flags := binary.LittleEndian.Uint32(body[:4])

	if flags&syscall.O_WRONLY != 0 {
		s.replyStatus(hdr.Unique, int32(syscall.EROFS))
		return
	}

	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	openFlags, err := ino.Open(flags)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	var out fuse.OpenOut
	out.OpenFlags = openFlags
	s.reply(hdr.Unique, 0, openOutBytes(&out))
}

func (s *FuseServer) doRead(hdr *fuseInHeader, body []byte) {
	if len(body) < int(unsafe.Sizeof(fuse.ReadIn{}))-inHeaderSize {
		s.replyStatus(hdr.Unique, int32(syscall.EINVAL))
		return
	}
	// ReadIn layout after InHeader: Fh(8) + Offset(8) + Size(4) + ...
	offset := binary.LittleEndian.Uint64(body[8:16])
	size := binary.LittleEndian.Uint32(body[16:20])

	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	r, ok := ino.(io.ReaderAt)
	if !ok {
		s.replyStatus(hdr.Unique, int32(syscall.ENOSYS))
		return
	}

	buf := make([]byte, size)
	n, err := r.ReadAt(buf, int64(offset))
	if n > 0 {
		s.reply(hdr.Unique, 0, buf[:n])
		return
	}
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}
	s.reply(hdr.Unique, 0, nil)
}

func (s *FuseServer) doStatFs(hdr *fuseInHeader, body []byte) {
	var out fuse.StatfsOut
	if err := s.fs.root.StatFs(&out); err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}
	s.reply(hdr.Unique, 0, statfsOutBytes(&out))
}

func (s *FuseServer) doOpenDir(hdr *fuseInHeader, body []byte) {
	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	if !ino.Mode().IsDir() {
		s.replyStatus(hdr.Unique, int32(syscall.ENOTDIR))
		return
	}

	openFlags, err := ino.OpenDir()
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	var out fuse.OpenOut
	out.OpenFlags = openFlags
	s.reply(hdr.Unique, 0, openOutBytes(&out))
}

func (s *FuseServer) doReadDir(hdr *fuseInHeader, body []byte, plus bool) {
	if len(body) < 24 {
		s.replyStatus(hdr.Unique, int32(syscall.EINVAL))
		return
	}
	// ReadIn after InHeader: Fh(8) + Offset(8) + Size(4)
	offset := binary.LittleEndian.Uint64(body[8:16])
	size := binary.LittleEndian.Uint32(body[16:20])

	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	if !ino.Mode().IsDir() {
		s.replyStatus(hdr.Unique, int32(syscall.ENOTDIR))
		return
	}

	// Build a fuse.ReadIn to pass to the Inode.ReadDir interface
	var readIn fuse.ReadIn
	readIn.NodeId = hdr.NodeId
	readIn.Offset = offset
	readIn.Size = size

	buf := make([]byte, size)
	dirList := fuse.NewDirEntryList(buf, offset)

	if err := ino.ReadDir(&readIn, dirList, plus); err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	s.reply(hdr.Unique, 0, dirEntryListBytes(dirList))
}

func (s *FuseServer) doAccess(hdr *fuseInHeader, body []byte) {
	if len(body) < 8 {
		s.replyStatus(hdr.Unique, int32(syscall.EINVAL))
		return
	}
	mask := binary.LittleEndian.Uint32(body[:4])

	if mask&fuse.W_OK != 0 {
		s.replyStatus(hdr.Unique, int32(syscall.EPERM))
		return
	}

	if hdr.Uid == 0 {
		s.replyStatus(hdr.Unique, 0)
		return
	}

	ino, err := s.fs.getInode(hdr.NodeId)
	if err != nil {
		s.replyErr(hdr.Unique, err)
		return
	}

	mode := ino.Mode()
	if mask&fuse.X_OK != 0 && mode&1 == 0 {
		s.replyStatus(hdr.Unique, int32(syscall.EPERM))
		return
	}
	s.replyStatus(hdr.Unique, 0)
}

// InodeNotify tells the kernel to invalidate cached data for an inode.
func (s *FuseServer) InodeNotify(ino uint64, off int64, length int64) error {
	notify := fuseNotifyInvalInodeOut{
		Ino:    ino,
		Off:    off,
		Length: length,
	}
	notifySize := int(unsafe.Sizeof(notify))
	hdr := fuseOutHeader{
		Length: uint32(outHeaderSize + notifySize),
		Status: notifyInvalInode,
		Unique: 0,
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	buf := make([]byte, outHeaderSize+notifySize)
	copy(buf, (*[outHeaderSize]byte)(unsafe.Pointer(&hdr))[:])
	copy(buf[outHeaderSize:], (*[24]byte)(unsafe.Pointer(&notify))[:]) // sizeof(fuseNotifyInvalInodeOut) = 24

	_, err := syscall.Write(s.Fd, buf)
	return err
}

// InodeNotifyStoreCache stores data into the kernel's inode cache.
func (s *FuseServer) InodeNotifyStoreCache(ino uint64, off int64, data []byte) error {
	notify := fuseNotifyStoreOut{
		Nodeid:  ino,
		Offset:  uint64(off),
		Size:    uint32(len(data)),
	}
	notifySize := int(unsafe.Sizeof(notify))
	hdr := fuseOutHeader{
		Length: uint32(outHeaderSize + notifySize + len(data)),
		Status: notifyStoreCache,
		Unique: 0,
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	headerBuf := make([]byte, outHeaderSize+notifySize)
	copy(headerBuf, (*[outHeaderSize]byte)(unsafe.Pointer(&hdr))[:])
	copy(headerBuf[outHeaderSize:], (*[16]byte)(unsafe.Pointer(&notify))[:]) // sizeof(fuseNotifyStoreOut) = 16

	_, err := syscall.Write(s.Fd, append(headerBuf, data...))
	return err
}

// reply sends a successful response to the kernel.
func (s *FuseServer) reply(unique uint64, status int32, payload []byte) error {
	hdr := fuseOutHeader{
		Length: uint32(outHeaderSize + len(payload)),
		Status: -status, // kernel expects negated errno
		Unique: unique,
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	buf := make([]byte, outHeaderSize+len(payload))
	copy(buf, (*[outHeaderSize]byte)(unsafe.Pointer(&hdr))[:])
	if len(payload) > 0 {
		copy(buf[outHeaderSize:], payload)
	}
	_, err := syscall.Write(s.Fd, buf)
	return err
}

// replyStatus sends an error-only response (no payload).
func (s *FuseServer) replyStatus(unique uint64, errno int32) error {
	return s.reply(unique, errno, nil)
}

// replyErr converts a Go error to an errno and sends it.
func (s *FuseServer) replyErr(unique uint64, err error) error {
	return s.replyStatus(unique, int32(toErrno(err)))
}

// toErrno converts a Go error to a syscall.Errno.
func toErrno(err error) syscall.Errno {
	if err == nil {
		return 0
	}
	if err == os.ErrNotExist {
		return syscall.ENOENT
	}
	if err == os.ErrPermission {
		return syscall.EPERM
	}
	if err == os.ErrInvalid {
		return syscall.EINVAL
	}
	if err == os.ErrExist {
		return syscall.EEXIST
	}
	if errno, ok := err.(syscall.Errno); ok {
		return errno
	}
	return syscall.EIO
}

// cstring extracts a null-terminated string from a byte slice.
func cstring(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// Helpers to get raw bytes from go-fuse types for writing to the kernel.
// These rely on the structs being laid out identically to the kernel ABI,
// which go-fuse guarantees.

func entryOutBytes(e *fuse.EntryOut) []byte {
	return (*[unsafe.Sizeof(fuse.EntryOut{})]byte)(unsafe.Pointer(e))[:]
}

func attrOutBytes(a *fuse.AttrOut) []byte {
	return (*[unsafe.Sizeof(fuse.AttrOut{})]byte)(unsafe.Pointer(a))[:]
}

func openOutBytes(o *fuse.OpenOut) []byte {
	return (*[unsafe.Sizeof(fuse.OpenOut{})]byte)(unsafe.Pointer(o))[:]
}

func statfsOutBytes(o *fuse.StatfsOut) []byte {
	return (*[unsafe.Sizeof(fuse.StatfsOut{})]byte)(unsafe.Pointer(o))[:]
}

// dirEntryListBytes extracts the filled portion of a DirEntryList's buffer.
// We use reflect to access the unexported 'buf' field.
func dirEntryListBytes(l *fuse.DirEntryList) []byte {
	// Access the unexported buf field via reflect
	v := reflect.ValueOf(l).Elem()
	buf := v.FieldByName("buf")
	return buf.Bytes()
}
