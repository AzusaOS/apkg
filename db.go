package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/tardigradeos/tpkg/tpkgdb"
)

// download db and keep it up to date

const PKG_URL_PREFIX = "https://pkg.tardigradeos.com/dist/"

var (
	dbLoad sync.Once
	db     *tpkgdb.DB
)

func loadDb() {
	dbLoad.Do(realLoadDb)
}

func realLoadDb() {
	log.Printf("db: loading database")

	if _, err := os.Stat("main.bin"); os.IsNotExist(err) {
		log.Printf("Downloading main database...")
		resp, err := http.Get(PKG_URL_PREFIX + "main/db.bin")
		if err != nil {
			log.Printf("failed: %s", err)
			return
		}
		defer resp.Body.Close()

		out, err := os.Create("main.bin~")
		if err != nil {
			log.Printf("failed: %s", err)
			return
		}

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			log.Printf("db: failed: %s", err)
			out.Close()
			return
		}
		out.Close()

		os.Rename("main.bin~", "main.bin")
	}

	f, err := os.Open("main.bin")
	if err != nil {
		log.Printf("db: failed to open file: %s", err)
		return
	}

	db, err = tpkgdb.New(f)
	if err != nil {
		log.Printf("db: failed: %s", err)
		return
	}
}
