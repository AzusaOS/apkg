package main

import "os"

type inodeObj interface {
	NodeId() (uint64, uint64) // NodeId, Generation
	Lookup(name string) (inodeObj, error)
}

const (
	InodeRoot   = 1
	InodeById   = 2
	InodeByName = 3
)

type specialInodeObj struct {
	ino      uint64
	refcount int64
	mode     os.FileMode
	children map[string]*inodeObj
}

func (i *specialInodeObj) NodeId() (uint64, uint64) {
	// special nodes have generation=0
	return i.ino, 0
}

func (i *specialInodeObj) Lookup(name string) (inodeObj, error) {
	return nil, os.ErrNotExist
}
