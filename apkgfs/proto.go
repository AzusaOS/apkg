package apkgfs

import "unsafe"

// FUSE protocol opcodes (from linux/fuse.h)
const (
	opLookup       = 1
	opForget       = 2
	opGetattr      = 3
	opSetattr      = 4
	opReadlink     = 5
	opSymlink      = 6
	opMknod        = 8
	opMkdir        = 9
	opUnlink       = 10
	opRmdir        = 11
	opRename       = 12
	opLink         = 13
	opOpen         = 14
	opRead         = 15
	opWrite        = 16
	opStatfs       = 17
	opRelease      = 18
	opFsync        = 20
	opSetxattr     = 21
	opGetxattr     = 22
	opListxattr    = 23
	opRemovexattr  = 24
	opFlush        = 25
	opInit         = 26
	opOpendir      = 27
	opReaddir      = 28
	opReleasedir   = 29
	opFsyncdir     = 30
	opAccess       = 34
	opCreate       = 35
	opInterrupt    = 36
	opDestroy      = 38
	opFallocate    = 43
	opReaddirplus  = 44
	opRename2      = 45
	opCopyFileRange = 47
	opBatchForget  = 42
)

// FUSE INIT capability flags
const (
	capAsyncRead      = 1 << 0
	capBigWrites      = 1 << 5
	capFileOps        = 1 << 2
	capAutoInvalData  = 1 << 12
	capReaddirplus    = 1 << 13
	capReaddirplusAuto = 1 << 14
	capMaxPages       = 1 << 22
	capCacheSymlinks  = 1 << 23
	capParallelDirops = 1 << 18
	capAsyncDio       = 1 << 15
)

// FUSE notification codes (written as OutHeader.Unique)
const (
	notifyInvalInode = -2
	notifyInvalEntry = -3
	notifyStoreCache = -4
)

// fuseInHeader is the header for every FUSE request from the kernel.
// 40 bytes, matching struct fuse_in_header.
type fuseInHeader struct {
	Length  uint32
	Opcode  uint32
	Unique  uint64
	NodeId  uint64
	Uid     uint32
	Gid     uint32
	Pid     uint32
	Padding uint32
}

// fuseOutHeader is the header for every FUSE response to the kernel.
// 16 bytes, matching struct fuse_out_header.
type fuseOutHeader struct {
	Length uint32
	Status int32
	Unique uint64
}

// fuseInitIn is the body of the INIT request (after InHeader).
type fuseInitIn struct {
	Major        uint32
	Minor        uint32
	MaxReadAhead uint32
	Flags        uint32
	Flags2       uint32
	Unused       [11]uint32
}

// fuseInitOut is the response body for INIT.
type fuseInitOut struct {
	Major               uint32
	Minor               uint32
	MaxReadAhead        uint32
	Flags               uint32
	MaxBackground       uint16
	CongestionThreshold uint16
	MaxWrite            uint32
	TimeGran            uint32
	MaxPages            uint16
	Padding             uint16
	Flags2              uint32
	MaxStackDepth       uint32
	RequestTimeout      uint16
	Unused              [11]uint16
}

// fuseNotifyInvalInodeOut is written to the fd to invalidate inode data.
type fuseNotifyInvalInodeOut struct {
	Ino    uint64
	Off    int64
	Length int64
}

// fuseNotifyStoreOut is written to the fd to store data in kernel cache.
type fuseNotifyStoreOut struct {
	Nodeid  uint64
	Offset  uint64
	Size    uint32
	Padding uint32
}

const (
	inHeaderSize  = int(unsafe.Sizeof(fuseInHeader{}))  // 40
	outHeaderSize = int(unsafe.Sizeof(fuseOutHeader{})) // 16
)
