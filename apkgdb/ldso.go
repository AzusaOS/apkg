package apkgdb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"sort"

	"github.com/KarpelesLab/ldcache"
	"github.com/hanwen/go-fuse/v2/fuse"
	bolt "go.etcd.io/bbolt"
)

type ldsoIno struct {
	d    *DB
	ldso []byte
}

func (i *ldsoIno) Mode() os.FileMode {
	return 0444
}

func (i *ldsoIno) IsDir() bool {
	return false
}

func (i *ldsoIno) Size() uint64 {
	return uint64(len(i.ldso))
}

func (i *ldsoIno) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(i.ldso)) {
		return 0, io.EOF
	}
	return copy(p, i.ldso[off:]), nil
}

func (i *ldsoIno) Readlink() ([]byte, error) {
	return nil, os.ErrInvalid
}

func (i *ldsoIno) Open(flags uint32) (uint32, error) {
	return fuse.FOPEN_KEEP_CACHE, nil
}

func (i *ldsoIno) OpenDir() (uint32, error) {
	return 0, os.ErrInvalid
}

func (d *ldsoIno) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) error {
	return os.ErrInvalid
}

func (i *ldsoIno) AddRef(count uint64) uint64 {
	return 1
}

func (i *ldsoIno) DelRef(count uint64) uint64 {
	return 0
}

func (i *ldsoIno) Lookup(ctx context.Context, name string) (n uint64, err error) {
	return 0, os.ErrInvalid
}

// buildLdso builds d.ldso based on entries found in the db
func (d *DB) buildLdso() error {
	entries := make(map[string]*ldcache.Entry)

	d.dbptr.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("ldso"))
		if bucket == nil {
			// no ldso data?
			return nil
		}
		return bucket.ForEach(func(k, v []byte) error {
			var e *ldcache.Entry
			err := json.Unmarshal(v, &e)
			if err != nil {
				return err
			}
			entries[e.Key] = e
			return nil
		})
	})

	//natSort(res)

	f := ldcache.New()
	for _, e := range entries {
		f.Entries = append(f.Entries, e)
	}
	sort.Sort(f.Entries)

	buf := &bytes.Buffer{}
	f.WriteTo(buf)

	d.ldso = buf.Bytes()

	log.Printf("apkgdb: built ld.so.cache containing %d libs (%d bytes)", len(entries), len(d.ldso))

	return nil
}
