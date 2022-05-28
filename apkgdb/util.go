package apkgdb

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// from sys/personality.h
	PER_LINUX32 = 0x0008
)

func bytesDup(v []byte) []byte {
	// simple function to duplicate a []byte because boltdb
	if len(v) == 0 {
		return nil
	}
	r := make([]byte, len(v))
	copy(r, v)
	return r
}

func getPersonality(pid uint32) uint32 {
	fn := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "personality")
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return 0
	}

	// 00000000  30 30 30 30 30 30 30 30  0a                       |00000000.|
	// with PER_LINUX32:
	// 00000000  30 30 30 30 30 30 30 38  0a                       |00000008.|
	hexVal := strings.TrimSpace(string(data))
	val, err := strconv.ParseUint(hexVal, 16, 32)
	if err != nil {
		return 0
	}
	return uint32(val)
}

func is32bitsProcess(pid uint32) bool {
	if getPersonality(pid)&PER_LINUX32 == PER_LINUX32 {
		// this has the PER_LINUX32 flag set (eg. via: setarch i686)
		return true
	}

	// this is a bit of a hack. ... haha
	fn := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "maps")
	fp, err := os.Open(fn)
	if err != nil {
		// can't check
		return false
	}
	defer fp.Close()

	// typical 32bits map:
	// 08048000-0811e000 r-xp 00000000 00:38 2662298
	// typical 64bits map:
	// 55e330a00000-55e330a08000 r-xp 00000000 fd:00 37913015

	// sadly I haven't found a better way to differenciate 32bits and 64bits processes yet

	// read all maps
	fpb := bufio.NewReader(fp)
	for {
		lin, err := fpb.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// not a single >32bits map!
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
