package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// detect location of socket
	p := "/var/lib/tpkg/tpkg.sock"
	if os.Geteuid() != 0 {
		h := os.Getenv("HOME")
		if h != "" {
			p = filepath.Join(h, ".cache/tpkg/tpkg.sock")
		}
	}

	s, _ := net.ResolveUnixAddr("unix", p)

	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.DialUnix("unix", nil, s)
			},
		},
	}

	u := "http://ctrl/"
	if len(os.Args) > 1 {
		u += os.Args[1]
	}

	resp, err := c.Get(u)
	if err != nil {
		log.Printf("request failed: %s", err)
		os.Exit(1)
	}

	defer resp.Body.Close()

	io.Copy(os.Stdout, resp.Body)
}
