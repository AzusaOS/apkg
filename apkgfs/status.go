package apkgfs

import (
	"github.com/hanwen/go-fuse/fuse"
)

func toStatus(err error) fuse.Status {
	if err == nil {
		return fuse.OK
	}
	//log.Printf("apkgfs: error received: %s", err)
	return fuse.ToStatus(err)
}