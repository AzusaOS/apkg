package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"git.atonline.com/azusa/apkg/apkgdb"
	"git.atonline.com/azusa/apkg/apkgfs"
)

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
	signal.Notify(c, syscall.SIGHUP)

	go func() {
		for {
			select {
			case sig := <-c:
				switch sig {
				case os.Interrupt, syscall.SIGTERM:
					shutdown()
					return
				case syscall.SIGHUP:
					// reload
					go dbMain.Update()
				}
			}
		}
	}()
}

func setRlimit() {
	var rLimit syscall.Rlimit
	rLimit.Cur = 65536
	rLimit.Max = 65536
	syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
}

func main() {
	log.Printf("apkg: Starting apkg daemon built on %s", DATE_TAG)
	setRlimit()
	setupSignals()

	db := "main"
	var err error

	// instanciate database
	p := "/var/lib/apkg"
	base := "/pkg"

	if os.Getuid() == 0 {
		dl, err := ioutil.ReadDir("/mnt")
		if err == nil {
			for _, d := range dl {
				// for each dir in /mnt, check if /mnt/<d>/AZUSA exists, and if so, use that as prefix
				n := d.Name()
				if st, err := os.Stat("/mnt/" + n + "/AZUSA"); err == nil && st.IsDir() {
					p = "/mnt/" + n + "/AZUSA/apkg"
					break
				}
			}
		}
	} else {
		h := os.Getenv("HOME")
		if h != "" {
			p = filepath.Join(h, ".cache/apkg")
			base = filepath.Join(h, "pkg")
		}
	}
	dbMain, err = apkgdb.New(apkgdb.PKG_URL_PREFIX, db, p)
	if err != nil {
		log.Printf("db: failed to load: %s", err)
		return
	}
	http.Handle("/apkgdb/"+db, dbMain)

	// mount database
	mp, err := apkgfs.New(filepath.Join(base, db), dbMain)
	if err != nil {
		fmt.Printf("Mount fail: %s\n", err)
		os.Exit(1)
	}
	go mp.Serve()
	defer mp.Unmount()

	// now that database is mounted, run updater
	go updater(base)
	listenCtrl()

	<-shutdownChan
}
