package apkgdb

import (
	"context"
	"os"
	"sync/atomic"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type ldsoIno struct {
	d      *DB
	refcnt uint64
}

func (i *ldsoIno) Mode() os.FileMode {
	return 0444
}

func (i *ldsoIno) IsDir() bool {
	return false
}

func (i *ldsoIno) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *ldsoIno) Open(flags uint32) (uint32, error) {
	// TODO
	return 0, os.ErrInvalid
}

func (i *ldsoIno) OpenDir() (uint32, error) {
	return 0, os.ErrInvalid
}

func (d *ldsoIno) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error {
	return os.ErrInvalid
}

func (i *ldsoIno) AddRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, count)
}

func (i *ldsoIno) DelRef(count uint64) uint64 {
	return atomic.AddUint64(&i.refcnt, ^(count - 1))
}

func (i *ldsoIno) Lookup(ctx context.Context, name string) (n uint64, err error) {
	return 0, os.ErrInvalid
}
