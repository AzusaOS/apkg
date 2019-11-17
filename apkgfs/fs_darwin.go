package apkgfs

import (
	"os"

	"github.com/MagicalTux/go-fuse/fuse"
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
