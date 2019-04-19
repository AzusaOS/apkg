package tpkgdb

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/petar/GoLLRB/llrb"
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

	os.MkdirAll(d.path, 0755)
	out, err := os.Create(filepath.Join(d.path, d.name+".bin~"))
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
	os.Rename(filepath.Join(d.path, d.name+".bin~"), filepath.Join(d.path, d.name+".bin"))
	return true, nil
}

// update will check server for new version, and update the db if needed
func (d *DB) update() error {
	r := &DBData{
		prefix:  d.DBData.prefix,
		name:    d.DBData.name,
		path:    d.DBData.path,
		fs:      d.DBData.fs,
		ino:     llrb.New(),
		nameIdx: llrb.New(),
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

func (d *DB) updateThread() {
	// keep running & check for updates
	t := time.NewTicker(1 * time.Hour)
	for {
		select {
		case <-t.C:
			d.update()
		case <-d.upd:
			d.update()
		}
	}
}
