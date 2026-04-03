package apkgfs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// fuseMount opens /dev/fuse and mounts it at mountPoint.
// Returns the fd or an error.
func fuseMount(mountPoint string, allowOther bool) (int, error) {
	fd, err := syscall.Open("/dev/fuse", os.O_RDWR, 0)
	if err != nil {
		return -1, fmt.Errorf("open /dev/fuse: %w", err)
	}

	var st syscall.Stat_t
	if err := syscall.Stat(mountPoint, &st); err != nil {
		syscall.Close(fd)
		return -1, fmt.Errorf("stat %s: %w", mountPoint, err)
	}

	opts := []string{
		fmt.Sprintf("fd=%d", fd),
		fmt.Sprintf("rootmode=%o", st.Mode&syscall.S_IFMT),
		fmt.Sprintf("user_id=%d", os.Geteuid()),
		fmt.Sprintf("group_id=%d", os.Getegid()),
		fmt.Sprintf("max_read=%d", 131072),
	}
	if allowOther {
		opts = append(opts, "allow_other")
	}

	flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOATIME)
	err = syscall.Mount("apkg", mountPoint, "fuse.apkg", flags, strings.Join(opts, ","))
	if err != nil {
		syscall.Close(fd)
		return -1, fmt.Errorf("mount %s: %w", mountPoint, err)
	}

	return fd, nil
}

// fuseUnmount unmounts the filesystem at mountPoint.
func fuseUnmount(mountPoint string) error {
	return syscall.Unmount(mountPoint, 0)
}

// inheritedFd checks if a FUSE fd was inherited from a parent process
// (graceful restart). Returns the fd and mount point, or -1 if not inherited.
func inheritedFd() (fd int, mountPoint string) {
	fdStr := os.Getenv("APKG_FUSE_FD")
	mountPoint = os.Getenv("APKG_MOUNT_POINT")
	if fdStr == "" || mountPoint == "" {
		return -1, ""
	}

	n, err := strconv.Atoi(fdStr)
	if err != nil {
		return -1, ""
	}

	// Clear the env vars so children don't inherit them accidentally
	os.Unsetenv("APKG_FUSE_FD")
	os.Unsetenv("APKG_MOUNT_POINT")

	return n, mountPoint
}

// GracefulExec replaces the current process with newBinary, preserving
// the FUSE fd so the new process can pick up the mount seamlessly.
func (s *FuseServer) GracefulExec(newBinary string) error {
	log.Printf("apkgfs: graceful exec to %s (fd=%d, mount=%s)", newBinary, s.Fd, s.MountPoint)

	// Clear close-on-exec so the fd survives exec
	_, _, errno := syscall.RawSyscall(syscall.SYS_FCNTL, uintptr(s.Fd), syscall.F_SETFD, 0)
	if errno != 0 {
		return fmt.Errorf("fcntl F_SETFD: %w", errno)
	}

	// Build environment with FUSE fd info
	env := os.Environ()
	env = setEnv(env, "APKG_FUSE_FD", strconv.Itoa(s.Fd))
	env = setEnv(env, "APKG_MOUNT_POINT", s.MountPoint)

	return syscall.Exec(newBinary, os.Args, env)
}

// doMount handles the full mount sequence for PkgFS, including inherited fd.
func (res *PkgFS) doMount() error {
	// Check for inherited fd from graceful restart
	if fd, mp := inheritedFd(); fd >= 0 {
		log.Printf("apkgfs: inheriting FUSE fd %d from previous process (mount=%s)", fd, mp)
		res.fuseServer = newFuseServer(fd, mp, res)
		return nil
	}

	// Fresh mount
	mkPath := filepath.Dir(res.mountPoint)
	mkName := filepath.Base(res.mountPoint)
	mountOverlay := false
	actualMountPoint := res.mountPoint

	if s, err := os.Stat(filepath.Join(mkPath, "."+mkName+"-rw")); os.Geteuid() == 0 && err == nil && s.IsDir() {
		if err = os.MkdirAll(filepath.Join(mkPath, "."+mkName+"-ro"), 0755); err == nil {
			if err = os.MkdirAll(filepath.Join(mkPath, "."+mkName+"-work"), 0755); err == nil {
				mountOverlay = true
				actualMountPoint = filepath.Join(mkPath, "."+mkName+"-ro")
			}
		}
	}

	allowOther := os.Getuid() == 0
	fd, err := fuseMount(actualMountPoint, allowOther)
	if err != nil {
		return err
	}

	res.fuseServer = newFuseServer(fd, actualMountPoint, res)

	if mountOverlay {
		err = syscall.Mount("overlay", filepath.Join(mkPath, mkName), "overlay", syscall.MS_NOATIME,
			"lowerdir="+actualMountPoint+",upperdir="+filepath.Join(mkPath, "."+mkName+"-rw")+",workdir="+filepath.Join(mkPath, "."+mkName+"-work"))
		if err != nil {
			fuseUnmount(actualMountPoint)
			syscall.Close(fd)
			return err
		}
	}

	return nil
}

// setEnv sets or replaces a key=value pair in an environment slice.
func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
