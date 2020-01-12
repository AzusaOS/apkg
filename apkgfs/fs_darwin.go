package apkgfs

import (
	"os"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func (res *PkgFS) doMount() error {
	var err error
	res.server, err = fuse.NewServer(res, res.mountPoint, &fuse.MountOptions{
		AllowOther: os.Getuid() == 0,
		Debug:      false,
		FsName:     "apkg",
		Name:       "apkg",
	})

	if err != nil {
		return err
	}

	return nil
}
