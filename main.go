package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/tardigradeos/tpkg/tpkgdb"
	"github.com/tardigradeos/tpkg/tpkgfs"
)

const PKG_URL_PREFIX = "https://pkg.tardigradeos.com/"

var server *fuse.Server
var dbMain *tpkgdb.DB

func shutdown() {
	log.Println("tpkg: shutting down...")
	if server != nil {
		server.Unmount()
	}
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

	dbMain, err = tpkgdb.New(PKG_URL_PREFIX, "main", mp)
	if err != nil {
		log.Printf("db: failed to load: %s", err)
		return
	}

	mp.Serve()
}
