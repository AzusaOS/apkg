package tpkgdb

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"
	"unsafe"
)

func (d *DBData) download(v string) (bool, error) {
	resp, err := http.Get(d.prefix + "db/" + d.name + "/" + runtime.GOOS + "/" + runtime.GOARCH + "/LATEST.txt")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	version, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	version = bytes.TrimSpace(version)

	if v != "" && v == string(version) {
		// no update needed
		return false, nil
	}

	log.Printf("tpkgdb: Downloading %s database version %s ...", d.name, version)

	resp, err = http.Get(d.prefix + "db/" + d.name + "/" + runtime.GOOS + "/" + runtime.GOARCH + "/" + string(version) + ".bin")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	out, err := os.Create(d.name + ".bin~")
	if err != nil {
		return false, err
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		return false, err
	}
	out.Close()

	// rename method allows already open db to stay the same
	os.Rename(d.name+".bin~", d.name+".bin")
	return true, nil
}

// Update will check server for new version, update and return a new instance of DB unless there was no new version, in which case the original instance is returned
func (d *DB) Update() error {
	r := &DBData{
		prefix:   d.DBData.prefix,
		name:     d.DBData.name,
		inoStart: d.DBData.inoStart + d.DBData.inoCount,
		ino:      make(map[uint64]*Package),
		pkgName:  make(map[string]*Package),
		pkgAlias: make(map[string]*Package),
	}

	v := d.created.UTC().Format("20060102150405")

	upd, err := r.download(v)
	if err != nil {
		return err
	}
	if !upd {
		// no update
		return nil
	}

	// index new value
	err = r.load()
	if err != nil {
		return err
	}

	// atomic update of ptr
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&d.DBData)), unsafe.Pointer(r))
	return nil
}