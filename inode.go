package main

import (
	"log"
	"os"
)

type inodeObj interface {
	//os.FileInfo

	NodeId() (uint64, uint64) // NodeId, Generation
	Mode() os.FileMode
	Lookup(name string) (inodeObj, error)
}

const (
	InodeRoot   uint64 = 1
	InodeById          = 2
	InodeByName        = 3
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
	switch i.ino {
	case InodeRoot:
		log.Printf("ROOT lookup: %s", name)
		switch name {
		case "by-id":
			v, ok := pkgFSobj.getInode(InodeById)
			if ok {
				return v, nil
			} else {
				return nil, os.ErrNotExist
			}
		case "by-name":
			v, ok := pkgFSobj.getInode(InodeByName)
			if ok {
				return v, nil
			} else {
				return nil, os.ErrNotExist
			}
		}
	}
	return nil, os.ErrNotExist
}

func (i *specialInodeObj) Mode() os.FileMode {
	return i.mode
}

func (i *specialInodeObj) IsDir() bool {
	return i.mode.IsDir()
}
