package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

// Stack returns a formatted stack trace of all the goroutines.
// It calls runtime.Stack with a large enough buffer to capture the entire trace.
func Stack() []byte {
	buf := make([]byte, 1024*1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}

func init() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain")

		fmt.Fprintf(w, "apkg control channel\n\n")
		fmt.Fprintf(w, "apkgdb: db related endpoints\n")
	})

	http.HandleFunc("/_stack", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write(Stack())
	})
}

func listenCtrl() {
	p := 100

	if uid := os.Getuid(); uid != 0 {
		p = 10000
	}

	lTcp, err := net.ListenTCP("tcp", &net.TCPAddr{Port: p})
	if err != nil {
		log.Printf("ctrl: failed to create control socket on port %d: %s", p, err)
		return
		// we don't make a udp listener if tcp failed
	}
	log.Printf("ctrl: control socket ready")

	go func() {
		err = http.Serve(lTcp, nil)
		if err != nil {
			log.Printf("ctrl: control socket failed: %s", err)
		}
	}()

	lUdp, err := net.ListenUDP("udp", &net.UDPAddr{Port: p})

	if err != nil {
		log.Printf("ctrl: failed to create udp listener: %s", err)
		return
	}

	// set SO_REUSEPORT (if it fails we don't really care, shouldn't anyway)
	sc, err := lUdp.SyscallConn()
	if err == nil {
		sc.Control(func(fd uintptr) {
			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		})
	}

	go udpHandler(lUdp, p)
}

func udpHandler(l *net.UDPConn, tcp_port int) {
	defer l.Close()
	buf := make([]byte, 1500)

	for {
		ln, addr, err := l.ReadFromUDP(buf)

		if err != nil {
			log.Printf("failed to read from udp: %s", err)
			return // give it up
		}

		// TODO: analyze packet & provide response
		b := buf[:ln]

		if bytes.Equal(b, []byte("DISCOVER")) {
			// send response
			res := fmt.Sprintf("tcp/%d", tcp_port) // TODO
			l.WriteToUDP([]byte(res), addr)
			continue
		}
	}
}
