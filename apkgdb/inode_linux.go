package apkgdb

import (
	"git.atonline.com/azusa/apkg/apkgfs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func (i *DB) FillAttr(attr *fuse.Attr) error {
	attr.Ino = 1
	attr.Size = 4096
	attr.Blocks = 1
	attr.Mode = apkgfs.ModeToUnix(i.Mode())
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Blksize = 4096
	attr.Atimensec = 0
	attr.Mtimensec = 0
	attr.Ctimensec = 0
	attr.Owner.Uid = 0
	attr.Owner.Gid = 0
	return nil
}

func (i *ldsoIno) FillAttr(attr *fuse.Attr) error {
	attr.Ino = 2
	attr.Size = 4096
	attr.Blocks = 1
	attr.Mode = apkgfs.ModeToUnix(i.Mode())
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Blksize = 4096
	attr.Atimensec = 0
	attr.Mtimensec = 0
	attr.Ctimensec = 0
	attr.Owner.Uid = 0
	attr.Owner.Gid = 0
	return nil
}
