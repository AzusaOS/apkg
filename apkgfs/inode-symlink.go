package apkgfs

import (
	"os"

	"github.com/MagicalTux/go-fuse/fuse"
)

type symlinkInodeObj []byte

func NewSymlink(link []byte) Inode {
	return symlinkInodeObj(link)
}

func (i symlinkInodeObj) Lookup(name string) (uint64, error) {
	return 0, os.ErrInvalid
}

func (i symlinkInodeObj) Mode() os.FileMode {
	return os.ModeSymlink | 0444
}

func (i symlinkInodeObj) IsDir() bool {
	return false
}

func (i symlinkInodeObj) Readlink() ([]byte, error) {
	return []byte(i), nil
}

func (i symlinkInodeObj) Open(flags uint32) (uint32, error) {
	return 0, os.ErrInvalid
}

func (i symlinkInodeObj) OpenDir() (uint32, error) {
	return 0, os.ErrInvalid
}

func (i symlinkInodeObj) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error {
	return os.ErrInvalid
}

func (i symlinkInodeObj) AddRef(count uint64) uint64 {
	// we do not actually store count
	return 1
}

func (i symlinkInodeObj) DelRef(count uint64) uint64 {
	// virtual symlink is always OK to purge from cache
	return 0
}
