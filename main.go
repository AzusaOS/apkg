package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
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

	var err error
	server, err = fuse.NewServer(NewPkgFS(), "/pkg", &fuse.MountOptions{
		Debug: true,
	})
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	log.Printf("filesystem mounted on /pkg")

	server.Serve()
}
