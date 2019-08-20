package apkgdb

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"git.atonline.com/azusa/apkg/apkgfs"
	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/petar/GoLLRB/llrb"
)

func (d *DB) index(f *os.File) error {
	r := bufio.NewReader(f)
	sig := make([]byte, 4)

	var version uint32
	var flags uint64

	_, err := io.ReadFull(r, sig)
	if err != nil {
		return err
	}
	if string(sig) != "APDB" {
		return errors.New("not a apkgdb file")
	}

	// read version
	err = binary.Read(r, binary.BigEndian, &version)
	if err != nil {
		return err
	}
	if version != 1 {
		return errors.New("unsupported db version")
	}

	// read flags
	err = binary.Read(r, binary.BigEndian, &flags)
	if err != nil {
		return err
	}

	createdA := make([]int64, 2)
	err = binary.Read(r, binary.BigEndian, createdA)
	if err != nil {
		return err
	}
	created := time.Unix(createdA[0], createdA[1])

	log.Printf("apkgdb: reading database generated on %s (%s ago)", created, time.Since(created))

	osarchcnt := make([]uint32, 3)
	err = binary.Read(r, binary.BigEndian, osarchcnt)
	if err != nil {
		return err
	}

	// TODO check values
	os := osarchcnt[0]   // 0=linux 1=darwin 2=windows ...
	arch := osarchcnt[1] // 0=i386 1=amd64 ...
	count := osarchcnt[2]
	_, _, _ = os, arch, count

	name := make([]byte, 32)
	_, err = io.ReadFull(r, name)
	if err != nil {
		return err
	}

	if offt := bytes.IndexByte(name, 0); offt != -1 {
		name = name[:offt]
	}
	if string(name) != d.name {
		return fmt.Errorf("invalid database, was expecting %s but downloaded database was for %s", d.name, name)
	}

	// read location data
	dataLoc := make([]uint32, 2)
	err = binary.Read(r, binary.BigEndian, dataLoc)
	if err != nil {
		return err
	}

	dataHash := make([]byte, 32)
	_, err = r.Read(dataHash)
	if err != nil {
		return err
	}

	hash := sha256.New()
	hash.Write(d.data[dataLoc[0] : dataLoc[0]+dataLoc[1]])
	dataHashChk := hash.Sum(nil)

	if !bytes.Equal(dataHash, dataHashChk) {
		return errors.New("invalid data hash")
	}

	// grab the header only
	r.Seek(0, io.SeekStart)
	headerData := make([]byte, 196)
	err = io.ReadFull(r, headerData)
	if err != nil {
		return err
	}

	// seek at signature location
	//r.Seek(196, io.SeekStart)
	_, err = apkgsig.VerifyDb(headerData, r)
	if err != nil {
		return err
	}

	// TODO → use indices

	pkgList := make(map[uint64]*Package)

	r.Seek(int64(dataLoc[0]), io.SeekStart)
	// OK now let's read each package
	for i := uint32(0); i < d.count; i++ {
		var t uint8
		pos, _ := r.Seek(0, io.SeekCurrent)
		err = binary.Read(r, binary.BigEndian, &t)
		if err != nil {
			return err
		}
		if t != 0 {
			return errors.New("invalid data in db, couldn't open it")
		}

		pkg := &Package{
			parent:   d,
			startIno: d.inoCount,
			pos:      pos,
		}

		// let's read the package hash & other info
		pkg.hash = make([]byte, 32)
		_, err = io.ReadFull(r, pkg.hash)
		if err != nil {
			return err
		}

		// read size
		err = binary.Read(r, binary.BigEndian, &pkg.size)
		if err != nil {
			return err
		}

		var inodes uint32
		err = binary.Read(r, binary.BigEndian, &inodes)
		if err != nil {
			return err
		}
		pkg.inodes = uint64(inodes)

		// read name
		name, err := apkgsig.ReadVarblob(r, 256)
		if err != nil {
			return err
		}

		// read path
		path, err := apkgsig.ReadVarblob(r, 256)
		if err != nil {
			return err
		}

		pkg.rawHeader, err = apkgsig.ReadVarblob(r, 256)
		if err != nil {
			return err
		}
		pkg.rawSig, err = apkgsig.ReadVarblob(r, apkgsig.SignatureSize)
		if err != nil {
			return err
		}
		pkg.rawMeta, err = apkgsig.ReadVarblob(r, 65536)
		if err != nil {
			return err
		}

		pkg.name = string(name)
		pkg.path = string(path)

		pkgList[pkg.startIno] = pkg

		//log.Printf("read package %s size=%d", pkg.name, pkg.size)
		d.inoCount += uint64(pkg.inodes) + 1
		d.totalSize += pkg.size
	}

	offt, err := d.fs.AllocateInodes(d.inoCount, d.lookupInode)
	if err != nil {
		return err
	}

	// register inodes in root
	for ino, pkg := range pkgList {
		pkg.startIno = ino + offt
		d.ino.ReplaceOrInsert(pkg)
		d.nameIdx.ReplaceOrInsert(&llrbString{k: pkg.name, v: pkg})
	}

	return nil
}

func (d *DB) lookupInode(reqino uint64) (apkgfs.Inode, error) {
	var pkg *Package
	d.ino.DescendLessOrEqual(pkgindex(reqino), func(i llrb.Item) bool {
		pkg = i.(*Package)
		return false
	})
	if pkg != nil {
		return pkg.handleLookup(reqino)
	}

	return nil, os.ErrInvalid
}
