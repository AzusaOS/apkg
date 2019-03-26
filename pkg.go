package main

import "github.com/hanwen/go-fuse/fuse"

type PkgFS struct {
	fuse.RawFileSystem
}

func NewPkgFS() *PkgFS {
	res := &PkgFS{RawFileSystem: fuse.NewDefaultRawFileSystem()}
	return res
}

func (p *PkgFS) String() string {
	return "tpkgFS"
}

func (p *PkgFS) Access(input *fuse.AccessIn) (code fuse.Status) {
	if input.Mask&fuse.W_OK != 0 {
		return fuse.EPERM
	}
	return fuse.OK
}

func (p *PkgFS) Lookup(header *fuse.InHeader, name string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.ENOENT
}
