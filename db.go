package main

import (
	"log"
	"sync"

	"github.com/tardigradeos/tpkg/tpkgdb"
)

// download db and keep it up to date

const PKG_URL_PREFIX = "https://pkg.tardigradeos.com/"

var (
	dbLoad sync.Once
	db     *tpkgdb.DB
)

func loadDb() {
	dbLoad.Do(realLoadDb)
}

func realLoadDb() {
	log.Printf("db: loading database")

	var err error
	db, err = tpkgdb.New(PKG_URL_PREFIX, "main")
	if err != nil {
		log.Printf("db: failed: %s", err)
		return
	}
}
