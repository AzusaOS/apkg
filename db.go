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
	dbMain *tpkgdb.DB
)

func loadDb() {
	go dbLoad.Do(realLoadDb)
}

func realLoadDb() {
	log.Printf("db: loading main database")

	var err error
	dbMain, err = tpkgdb.New(PKG_URL_PREFIX, "main", 2)
	if err != nil {
		log.Printf("db: failed to load: %s", err)
		return
	}
}
