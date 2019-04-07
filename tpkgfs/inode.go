package tpkgfs

import (
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

type Inode interface {
	//os.FileInfo

	Mode() os.FileMode
	Lookup(name string) (uint64, error)
	FillAttr(attr *fuse.Attr) error
	Readlink() ([]byte, error)

	Open(flags uint32) error
	OpenDir() error
}

// see: https://golang.org/src/os/stat_linux.go
func modeToUnix(mode os.FileMode) uint32 {
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