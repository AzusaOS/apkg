package tpkgfs

import (
	"github.com/hanwen/go-fuse/fuse"
)

func toStatus(err error) fuse.Status {
	if err == nil {
		return fuse.OK
	}
	//log.Printf("tpkgfs: error received: %s", err)
	return fuse.ToStatus(err)
}
