package apkgdb

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func bytesDup(v []byte) []byte {
	// simple function to duplicate a []byte because boltdb
	r := make([]byte, len(v))
	copy(r, v)
	return r
}

func is32bitsProcess(pid uint32) bool {
	// this is a bit of a hack. ... haha
	fn := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "maps")
	fp, err := os.Open(fn)
	if err != nil {
		// can't check
		return false
	}
	defer fp.Close()

	// read all maps
	fpb := bufio.NewReader(fp)
	for {
		lin, err := fpb.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// not a single 32bits map!
				return true
			}
			return false
		}

		pos := strings.IndexByte(lin, '-')
		if pos > 8 {
			return false
		}
	}
}
