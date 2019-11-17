package apkgfs

import "github.com/MagicalTux/go-fuse/fuse"

func (i symlinkInodeObj) FillAttr(attr *fuse.Attr) error {
	attr.Size = uint64(len(i))
	attr.Blocks = 1
	attr.Mode = ModeToUnix(i.Mode())
	attr.Nlink = 1 // 1 required
	attr.Rdev = 1
	attr.Atimensec = 0
	attr.Mtimensec = 0
	attr.Ctimensec = 0
	return nil
}
