package apkgdb

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"git.atonline.com/azusa/apkg/squashfs"
	"github.com/fsnotify/fsnotify"
)

var (
	loadUnsigned  = flag.Bool("load_unsigned", false, "load unsigned packages from disk (DANGEROUS)")
	unsignedMap   = make(map[string]*unsignedPkg)
	unsignedMapLk sync.RWMutex
)

type unsignedPkg struct {
	os     string
	arch   string
	fn     string
	load   sync.Once
	squash *squashfs.Superblock
}

func initUnsigned(p string) {
	if !*loadUnsigned {
		return
	}

	p = filepath.Join(p, "unsigned")

	log.Printf("WARNING! -load_unsigned has been ENABLED. This means that unsigned packages found in %s will be loaded if requested", p)

	os.MkdirAll(p, 0755)

	go unsignedScan(p)
}

func unsignedScan(p string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("apkgdb: Unsigned FATAL, failed to initialize watcher: %s", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(p)
	if err != nil {
		log.Printf("apkgdb: failed to watch %s: %s", p, err)
		return
	}

	// initial scan
	l, _ := os.ReadDir(p)
	for _, f := range l {
		st, err := f.Info()
		if err != nil {
			continue
		}
		if !st.Mode().IsRegular() {
			continue
		}
		addUnsignedFile(p, f.Name())
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				// EOF
				return
			}
			//log.Printf("apkgdb: event: %s", event)
			if event.Op&fsnotify.Create == fsnotify.Create {
				addUnsignedFile(p, filepath.Base(event.Name))
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				removeUnsignedFile(p, filepath.Base(event.Name))
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				// EOF
				return
			}
			log.Printf("apkgdb: watcher error: %s", err)
		}
	}
}

func addUnsignedFile(p, f string) {
	if !strings.HasSuffix(f, ".squashfs") {
		return
	}
	pkgName := strings.TrimSuffix(f, ".squashfs")

	fn := filepath.Join(p, f)
	st, err := os.Stat(fn)
	if err != nil {
		log.Printf("apkgdb: failed to stat %s: %s", f, err)
		return
	}
	if !st.Mode().IsRegular() {
		// ignore directories, links, etc
		return
	}
	// file name should look like: category.package.core.1.2.3.linux.amd64.squashfs
	log.Printf("apkgdb: add unsigned package: %s", pkgName)
	unsignedMapLk.Lock()
	defer unsignedMapLk.Unlock()

	unsignedMap[pkgName] = &unsignedPkg{
		fn: fn,
	}
}

func removeUnsignedFile(p, f string) {
	log.Printf("apkgdb: remove unsigned file: %s", f)
	unsignedMapLk.Lock()
	defer unsignedMapLk.Unlock()

	delete(unsignedMap, f)
}
