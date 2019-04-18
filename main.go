package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/tardigradeos/tpkg/tpkgdb"
	"github.com/tardigradeos/tpkg/tpkgfs"
)

const PKG_URL_PREFIX = "https://pkg.tardigradeos.com/"

var dbMain *tpkgdb.DB
var shutdownChan = make(chan struct{})

func shutdown() {
	log.Println("tpkg: shutting down...")
	close(shutdownChan)
}

func setupSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)

	go func() {
		<-c
		shutdown()
	}()
}

func main() {
	log.Printf("tpkg: preparing")
	setupSignals()

	mp, err := tpkgfs.New()
	if err != nil {
		fmt.Printf("Mount fail: %s\n", err)
		os.Exit(1)
	}
	go mp.Serve()
	defer mp.Unmount()

	p := "/var/lib/tpkg"
	if os.Geteuid() != 0 {
		h := os.Getenv("HOME")
		if h != "" {
			p = filepath.Join(h, ".cache/tpkg")
		}
	}
	dbMain, err = tpkgdb.New(PKG_URL_PREFIX, "main", p, mp)
	if err != nil {
		log.Printf("db: failed to load: %s", err)
		return
	}

	<-shutdownChan
}
