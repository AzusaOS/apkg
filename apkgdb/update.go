package apkgdb

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"
)

func (d *DB) download(v string) (bool, error) {
	resp, err := http.Get(d.prefix + "db/" + d.name + "/" + runtime.GOOS + "/" + runtime.GOARCH + "/LATEST.txt")
	if err != nil {
		return false, err
	}

	version, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return false, err
	}

	version = bytes.TrimSpace(version)

	if v != "" && v == string(version) {
		// no update needed
		return false, nil
	}

	log.Printf("apkgdb: Downloading %s database version %s ...", d.name, version)

	resp, err = http.Get(d.prefix + "db/" + d.name + "/" + runtime.GOOS + "/" + runtime.GOARCH + "/" + string(version) + ".bin")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	out, err := ioutil.TempFile("", "apkg")
	if err != nil {
		return false, err
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		os.Remove(out.Name())

		return false, err
	}

	out.Seek(0, io.SeekStart)

	err = d.index(out)
	out.Close()
	os.Remove(out.Name())

	return true, err
}

func (d *DB) update() {
	d.download(d.CurrentVersion())
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
