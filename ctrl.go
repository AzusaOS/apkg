package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func init() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain")

		fmt.Fprintf(w, "tpkg control channel\n\n")
		fmt.Fprintf(w, "tpkgdb: db related endpoints\n")
	})
}

func listenUnix() net.Listener {
	p := "/var/lib/tpkg/tpkg.sock"
	if os.Geteuid() != 0 {
		h := os.Getenv("HOME")
		if h != "" {
			p = filepath.Join(h, ".cache/tpkg/tpkg.sock")
		}
	}

	if _, err := os.Stat(filepath.Dir(p)); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(p), 0755)
		if err != nil {
			log.Printf("ctrl: failed to create %s control socket: %s", p, err)
			return nil
		}
	}

	s, _ := net.ResolveUnixAddr("unix", p)

	os.Remove(p)

	l, err := net.ListenUnix("unix", s)
	if err != nil {
		log.Printf("ctrl: failed to create %s control socket: %s", p, err)
		return nil
	}

	log.Printf("ctrl: control socket ready at %s", p)

	go func() {
		err = http.Serve(l, nil)
		if err != nil {
			log.Printf("ctrl: control socket failed: %s", err)
		}
	}()

	return l
}
