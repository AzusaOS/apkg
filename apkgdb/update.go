package apkgdb

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"time"
)

func (d *DB) download(v string) (bool, error) {
	resp, err := hClient.Get(d.prefix + "db/" + d.name + "/" + d.os + "/" + d.arch + "/LATEST.txt")
	if err != nil {
		return false, err
	}

	version, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("failed to fetch information on latest database version: %s", resp.Status)
	}

	if err != nil {
		return false, err
	}

	version = bytes.TrimSpace(version)

	resp = nil

	if v != "" {
		if v == string(version) {
			// no update needed
			return false, nil
		}

		// check for delta
		log.Printf("apkgdb: Downloading %s database delta to version %s ...", d.name, version)

		resp, err = hClient.Get(d.prefix + "db/" + d.name + "/" + d.os + "/" + d.arch + "/" + v + "-" + string(version) + ".bin")
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			log.Printf("apkgdb: Delta download failed with error %s, will download full database", resp.Status)
			// fallback to downloading the whole db
			resp = nil
		}
	}

	if resp == nil {
		log.Printf("apkgdb: Downloading %s database version %s ...", d.name, version)

		resp, err = hClient.Get(d.prefix + "db/" + d.name + "/" + d.os + "/" + d.arch + "/" + string(version) + ".bin")
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return false, fmt.Errorf("failed to fetch latest database: %s", resp.Status)
		}
	}

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

func (d *DB) update() error {
	_, err := d.download(d.CurrentVersion())
	return err
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
