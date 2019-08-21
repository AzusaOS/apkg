package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"git.atonline.com/azusa/apkg/apkgdb"
	"git.atonline.com/azusa/apkg/apkgfs"
)

const PKG_URL_PREFIX = "https://pkg.azusa.jp/"

var dbMain *apkgdb.DB
var shutdownChan = make(chan struct{})
var DATE_TAG = "unknown"

func shutdown() {
	log.Println("apkg: shutting down...")
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
	log.Printf("apkg: Starting apkg daemon built on %s", DATE_TAG)
	setupSignals()

	db := "main"

	mp, err := apkgfs.New(db)
	if err != nil {
		fmt.Printf("Mount fail: %s\n", err)
		os.Exit(1)
	}
	go mp.Serve()
	defer mp.Unmount()

	p := "/var/lib/apkg"
	if os.Geteuid() != 0 {
		h := os.Getenv("HOME")
		if h != "" {
			p = filepath.Join(h, ".cache/apkg")
		}
	}
	dbMain, err = apkgdb.New(PKG_URL_PREFIX, db, p)
	if err != nil {
		log.Printf("db: failed to load: %s", err)
		return
	}

	go updater(mp.Path())
	l := listenUnix()
	if l != nil {
		defer l.Close()
	}

	<-shutdownChan
}
