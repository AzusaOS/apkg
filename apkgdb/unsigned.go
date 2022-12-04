package apkgdb

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"git.atonline.com/azusa/apkg/apkgfs"
	"github.com/KarpelesLab/squashfs"
	"github.com/fsnotify/fsnotify"
	"github.com/petar/GoLLRB/llrb"
)

var (
	loadUnsigned  = flag.Bool("load_unsigned", false, "load unsigned packages from disk (DANGEROUS)")
	unsignedMap   map[ArchOS]map[string]*unsignedPkg
	unsignedMapLk sync.RWMutex
)

type unsignedPkg struct {
	fn       string
	pkg      string
	startIno uint64
	inodes   uint64
	load     sync.Once
	squash   *squashfs.Superblock
	err      error
}

func (p *unsignedPkg) Value() uint64 {
	return p.startIno
}

func (p *unsignedPkg) Less(than llrb.Item) bool {
	return p.startIno < than.(pkgindexItem).Value()
}

func (p *unsignedPkg) open() {
	log.Printf("apkgdb: opening UNSIGNED %s", p.fn)

	var err error
	p.squash, err = squashfs.Open(p.fn)
	if err != nil {
		p.err = err
		return
	}
	p.inodes = uint64(p.squash.InodeCnt)
}

func (p *unsignedPkg) handleLookup(ino uint64) (apkgfs.Inode, error) {
	if p.err != nil {
		return nil, p.err
	}
	if ino == p.startIno {
		return apkgfs.NewSymlink([]byte(p.pkg)), nil
	}

	if p.squash == nil {
		// problem
		return nil, os.ErrInvalid
	}

	if ino <= p.startIno {
		// in case it is == it is symlink, which is returned by the
		return nil, os.ErrInvalid
	}

	return p.squash.GetInode(ino - p.startIno)
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

func listUnsigned(osV OS, arch Arch) []string {
	if unsignedMap == nil {
		return nil
	}

	unsignedMapLk.RLock()
	defer unsignedMapLk.RUnlock()

	archos := ArchOS{OS: osV, Arch: arch}

	if m, ok := unsignedMap[archos]; ok {
		res := make([]string, 0, len(m))
		for n := range m {
			res = append(res, n)
		}
		sort.Strings(res)
		return res
	}
	return nil
}

func lookupUnsigned(osV OS, arch Arch, name string) *unsignedPkg {
	if unsignedMap == nil {
		return nil
	}

	unsignedMapLk.RLock()
	defer unsignedMapLk.RUnlock()

	archos := ArchOS{OS: osV, Arch: arch}

	if m, ok := unsignedMap[archos]; ok {
		// try to find a matching name
		for n, v := range m {
			if strings.HasPrefix(n, name) {
				v.load.Do(v.open)
				return v
			}
		}
	}
	return nil
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

	pkg := &unsignedPkg{
		fn:  fn,
		pkg: name,
	}

	// file name should look like: category.package.core.1.2.3.linux.amd64.squashfs
	log.Printf("apkgdb: add unsigned package: %s OS=%s ARCH=%s", name, archos.OS, archos.Arch)
	unsignedMapLk.Lock()
	defer unsignedMapLk.Unlock()

	if _, ok := unsignedMap[archos]; !ok {
		unsignedMap[archos] = make(map[string]*unsignedPkg)
	}

	unsignedMap[archos][name] = pkg
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

	sname := name[:v]
	v = strings.LastIndexByte(sname, '.')
	if v == -1 {
		log.Printf("apkgdb: skipping UNSIGNED file %s: no OS?", f)
		return
	}
	archos.OS = ParseOS(sname[v+1:])
	if archos.OS == BadOS {
		log.Printf("apkgdb: skipping UNSIGNED file %s: bad OS", f)
		return
	}
	//name = name[:v]

	ok = true
	return
}
