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
	unsignedMap   map[ArchOS]map[string]*unsignedPkg
	unsignedMapLk sync.RWMutex
)

type unsignedPkg struct {
	fn     string
	pkg    string
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
	unsignedMap = make(map[ArchOS]map[string]*unsignedPkg)

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
	name, archos, ok := cleanUnsignedName(f)
	if !ok {
		return
	}

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
	log.Printf("apkgdb: add unsigned package: %s OS=%s ARCH=%s", name, archos.OS, archos.Arch)
	unsignedMapLk.Lock()
	defer unsignedMapLk.Unlock()

	if _, ok := unsignedMap[archos]; !ok {
		unsignedMap[archos] = make(map[string]*unsignedPkg)
	}

	unsignedMap[archos][name] = &unsignedPkg{
		fn:  fn,
		pkg: name,
	}
}

func removeUnsignedFile(p, f string) {
	name, archos, ok := cleanUnsignedName(f)
	if !ok {
		return
	}

	log.Printf("apkgdb: remove unsigned file: %s", f)
	unsignedMapLk.Lock()
	defer unsignedMapLk.Unlock()

	if m, ok := unsignedMap[archos]; ok {
		delete(m, name)
	}
}

func cleanUnsignedName(f string) (name string, archos ArchOS, ok bool) {
	if !strings.HasSuffix(f, ".squashfs") {
		return
	}
	name = strings.TrimSuffix(f, ".squashfs")

	v := strings.LastIndexByte(name, '.')
	if v == -1 {
		// there can be no filename without a '.'
		log.Printf("apkgdb: skipping UNSIGNED file %s: no dots", f)
		return
	}

	// name must be suffixed by cpu/OS
	// eg: azusa.symlinks.core.0.0.3.20210216.linux.amd64
	archos.Arch = ParseArch(name[v+1:])
	if archos.Arch == BadArch {
		log.Printf("apkgdb: skipping UNSIGNED file %s: bad ARCH", f)
		return
	}

	name = name[:v]
	v = strings.LastIndexByte(name, '.')
	if v == -1 {
		log.Printf("apkgdb: skipping UNSIGNED file %s: no OS?", f)
		return
	}
	archos.OS = ParseOS(name[v+1:])
	if archos.OS == BadOS {
		log.Printf("apkgdb: skipping UNSIGNED file %s: bad OS", f)
		return
	}
	name = name[:v]

	ok = true
	return
}
