package apkgfs

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func (res *PkgFS) doMount() error {
	mkPath := filepath.Dir(res.mountPoint)
	mkName := filepath.Base(res.mountPoint)

	mountOverlay := false

	if s, err := os.Stat(filepath.Join(mkPath, "."+mkName+"-rw")); os.Geteuid() == 0 && err == nil && s.IsDir() {
		// mount as an overlay
		if err = os.MkdirAll(filepath.Join(mkPath, "."+mkName+"-ro"), 0755); err == nil {
			if err = os.MkdirAll(filepath.Join(mkPath, "."+mkName+"-work"), 0755); err == nil {
				// proceed with overlay
				mountOverlay = true
				res.mountPoint = filepath.Join(mkPath, "."+mkName+"-ro")
			}
		}
	}

	var err error
	res.server, err = fuse.NewServer(res, res.mountPoint, &fuse.MountOptions{
		AllowOther:       os.Getuid() == 0,
		Debug:            false,
		FsName:           "apkg",
		Name:             "apkg",
		DirectMount:      true,
		DirectMountFlags: syscall.MS_NOATIME,
	})

	if err != nil {
		return err
	}

	if mountOverlay {
		// now we need to mount an overlay fs
		// mount -t overlay overlay -o lowerdir=/pkg/main,upperdir="$tmp_dir/.pkg-main-rw",workdir="$tmp_dir/.pkg-main-work" "$tmp_dir/pkg/main"
		err = syscall.Mount("overlay", filepath.Join(mkPath, mkName), "overlay", syscall.MS_NOATIME, "lowerdir="+res.mountPoint+",upperdir="+filepath.Join(mkPath, "."+mkName+"-rw")+",workdir="+filepath.Join(mkPath, "."+mkName+"-work"))
		if err != nil {
			res.server.Unmount()
			return err
		}
	}

	return nil
}
