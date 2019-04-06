package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

var server *fuse.Server

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

	loadDb()

	mountPoint := "/pkg"
	if os.Geteuid() != 0 {
		h := os.Getenv("HOME")
		if h != "" {
			mountPoint = filepath.Join(h, "pkg")
		}
	}
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		log.Fatalf("tpkg: failed to create %s: %s", mountPoint, err)
	}

	var err error
	server, err = fuse.NewServer(pkgFSobj, mountPoint, &fuse.MountOptions{
		AllowOther: os.Geteuid() == 0,
		Debug:      true,
		FsName:     "tpkg",
		Name:       "tpkg",
	})
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	log.Printf("filesystem mounted on %s", mountPoint)

	server.Serve()
}
