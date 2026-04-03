package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/AzusaOS/apkg/apkgdb"
	"github.com/AzusaOS/apkg/apkgfs"
)

var (
	dbMain       *apkgdb.DB
	fuseFS       *apkgfs.PkgFS
	shutdownChan = make(chan struct{})
	channel      = flag.String("channel", "stable", "release channel for version resolution (use \"latest\" for newest)")
)

func shutdown() {
	log.Println("apkg: shutting down...")
	close(shutdownChan)
}

func gracefulRestart() {
	exec, err := os.Executable()
	if err != nil {
		log.Printf("apkg: graceful restart failed: cannot determine executable: %s", err)
		return
	}
	if fuseFS == nil || fuseFS.FuseServer() == nil {
		log.Printf("apkg: graceful restart not available (no FUSE server)")
		return
	}
	log.Printf("apkg: performing graceful restart...")
	if err := fuseFS.FuseServer().GracefulExec(exec); err != nil {
		log.Printf("apkg: graceful restart failed: %s", err)
	}
}

func setupSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	signal.Notify(c, syscall.SIGHUP)
	signal.Notify(c, syscall.SIGUSR2)

	go func() {
		for sig := range c {
			switch sig {
			case os.Interrupt, syscall.SIGTERM:
				shutdown()
				return
			case syscall.SIGHUP:
				// reload
				go dbMain.Update()
			case syscall.SIGUSR2:
				// graceful restart
				go gracefulRestart()
			}
		}
	}()
}

func setRlimit() {
	var rLimit syscall.Rlimit
	rLimit.Cur = 65536
	rLimit.Max = 65536
	_ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
}

func main() {
	flag.Parse()

	log.Printf("apkg: Starting apkg daemon built on %s", DATE_TAG)
	setRlimit()
	setupSignals()

	db := "main"
	var err error

	// instanciate database
	p := "/var/lib/apkg"
	base := "/pkg"

	if os.Getuid() == 0 {
		dl, err := os.ReadDir("/mnt")
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
	dbMain.SetChannel(*channel)
	http.Handle("/apkgdb/"+db, dbMain)

	// mount database
	mp, err := apkgfs.New(filepath.Join(base, db), dbMain)
	if err != nil {
		fmt.Printf("Mount fail: %s\n", err)
		os.Exit(1)
	}
	fuseFS = mp

	dbMain.SetNotifyTarget(mp)
	go mp.Serve()
	defer mp.Unmount()

	// now that database is mounted, run updater
	go updater(base)
	listenCtrl()

	<-shutdownChan
}
