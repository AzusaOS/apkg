package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func updater(p string) {
	p = filepath.Join(p, "main/core.tpkg")
	v := DATE_TAG

	for {
		//
		n, err := os.Readlink(p)
		if err != nil {
			log.Printf("tpkg: failed to read latest version: %s", err)
			time.Sleep(1 * time.Hour)
			continue
		}
		n = strings.TrimPrefix(n, "core.tpkg.")
		n = strings.Split(n, ".")[0]
		//log.Printf("version: %s self=%s", n, v)

		switch strings.Compare(v, n) {
		case 0:
			// no change, do nothing
		case 1:
			log.Printf("tpkg: currently running version is more recent than latest, not doing update")
		case -1:
			log.Printf("tpkg: update to version %s required, copying...", n)

			exec, err := os.Executable()
			if err != nil {
				log.Printf("tpkg: update failed, unknown executable: %s", err)
				break
			}
			newV, err := os.Open(filepath.Join(p, "tpkg"))
			if err != nil {
				log.Printf("tpkg: update failed, failed to read new version: %s", err)
				break
			}

			f, err := os.Create(exec + "." + n)
			if err != nil {
				log.Printf("tpkg: update failed: %s", err)
				newV.Close()
				break
			}

			_, err = io.Copy(f, newV)
			newV.Close()
			f.Close()

			if err != nil {
				log.Printf("tpkg: update failed: failed to copy data: %s", err)
				os.Remove(exec + "." + n)
				break
			}

			os.Chmod(exec+"."+n, 0755)

			// rename
			err = os.Rename(exec+"."+n, exec)
			if err != nil {
				log.Printf("tpkg: update failed: %s", err)
				os.Remove(exec + "." + n)
				break
			}

			log.Printf("tpkg: updated to version %s, restart required", n)
			// we update v so we don't update again unless needed
			v = n
		}

		time.Sleep(1 * time.Hour)
	}
}
