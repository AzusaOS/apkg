package apkgdb

import (
	"os"
	"sync/atomic"

	"github.com/MagicalTux/go-fuse/fuse"
)

func (i *DB) Mode() os.FileMode {
	return os.ModeDir | 0444
}

func (i *DB) IsDir() bool {
	return true
}

func (i *DB) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *DB) Open(flags uint32) (uint32, error) {
	return 0, os.ErrInvalid
}

func (i *DB) OpenDir() (uint32, error) {
	return 0, os.ErrPermission
}

func (i *DB) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error {
	return os.ErrInvalid
}

func (i *DB) AddRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, count)
}

func (i *DB) DelRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, ^(count - 1))
}

func (i *DB) StatFs(out *fuse.StatfsOut) error {
	out.Blocks = (uint64(i.Length()) / 4096) + 1
	out.Bfree = 0
	out.Bavail = 0
	out.Files = i.Inodes() + 2 // root & INFO
	out.Ffree = 0
	out.Bsize = 4096
	out.NameLen = 255
	out.Frsize = 4096 // Fragment size

	return nil
}
