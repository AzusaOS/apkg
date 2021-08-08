package apkgfs

import (
	"context"
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type Inode interface {
	//os.FileInfo

	Mode() os.FileMode
	Lookup(ctx context.Context, name string) (uint64, error)
	FillAttr(attr *fuse.Attr) error
	Readlink() ([]byte, error)

	Open(flags uint32) (uint32, error)
	OpenDir() (uint32, error)
	ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error

	// functions for refcount
	AddRef(count uint64) uint64
	DelRef(count uint64) uint64
}

type RootInode interface {
	Inode

	GetInode(ino uint64) (Inode, error)
	StatFs(out *fuse.StatfsOut) error
}

func UnixToMode(mode uint32) os.FileMode {
	res := os.FileMode(mode & 0777)

	switch {
	case mode&syscall.S_IFCHR == syscall.S_IFCHR:
		res |= os.ModeCharDevice
	case mode&syscall.S_IFBLK == syscall.S_IFBLK:
		res |= os.ModeDevice
	case mode&syscall.S_IFDIR == syscall.S_IFDIR:
		res |= os.ModeDir
	case mode&syscall.S_IFIFO == syscall.S_IFIFO:
		res |= os.ModeNamedPipe
	case mode&syscall.S_IFLNK == syscall.S_IFLNK:
		res |= os.ModeSymlink
	case mode&syscall.S_IFSOCK == syscall.S_IFSOCK:
		res |= os.ModeSocket
	}

	// extra flags
	if mode&syscall.S_ISGID == syscall.S_ISGID {
		res |= os.ModeSetgid
	}

	if mode&syscall.S_ISUID == syscall.S_ISUID {
		res |= os.ModeSetuid
	}

	if mode&syscall.S_ISVTX == syscall.S_ISVTX {
		res |= os.ModeSticky
	}

	return res
}

// see: https://golang.org/src/os/stat_linux.go
func ModeToUnix(mode os.FileMode) uint32 {
	res := uint32(mode.Perm())

	// type of file
	switch {
	case mode&os.ModeCharDevice == os.ModeCharDevice:
		res |= syscall.S_IFCHR
	case mode&os.ModeDevice == os.ModeDevice:
		res |= syscall.S_IFBLK
	case mode&os.ModeDir == os.ModeDir:
		res |= syscall.S_IFDIR
	case mode&os.ModeNamedPipe == os.ModeNamedPipe:
		res |= syscall.S_IFIFO
	case mode&os.ModeSymlink == os.ModeSymlink:
		res |= syscall.S_IFLNK
	case mode&os.ModeSocket == os.ModeSocket:
		res |= syscall.S_IFSOCK
	default:
		res |= syscall.S_IFREG
	}

	// extra flags
	if mode&os.ModeSetgid == os.ModeSetgid {
		res |= syscall.S_ISGID
	}

	if mode&os.ModeSetuid == os.ModeSetuid {
		res |= syscall.S_ISUID
	}

	if mode&os.ModeSticky == os.ModeSticky {
		res |= syscall.S_ISVTX
	}

	return res
}

func (p *PkgFS) getInode(ino uint64) (Inode, error) {
	if ino == 1 {
		return p.root, nil
	}

	// check cache
	if i, ok := p.getInodeCache(ino); ok {
		return i, nil
	}

	// grab inode from root (caller will do the add to cache)
	return p.root.GetInode(ino)
}
