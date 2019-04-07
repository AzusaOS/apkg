package tpkgdb

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
)

func (d *DB) Update() error {
	if _, err := os.Stat(d.name + ".bin"); os.IsNotExist(err) {
		resp, err := http.Get(d.prefix + "db/" + d.name + "/" + runtime.GOOS + "/" + runtime.GOARCH + "/LATEST.txt")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		version, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		version = bytes.TrimSpace(version)

		log.Printf("tpkgdb: Downloading %s database version %s ...", d.name, version)

		resp, err = http.Get(d.prefix + "db/" + d.name + "/" + runtime.GOOS + "/" + runtime.GOARCH + "/" + string(version) + ".bin")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		out, err := os.Create(d.name + ".bin~")
		if err != nil {
			return err
		}

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			out.Close()
			return err
		}
		out.Close()

		// rename method allows already open db to stay the same
		os.Rename("main.bin~", "main.bin")
	}
	return nil
}
