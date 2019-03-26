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
