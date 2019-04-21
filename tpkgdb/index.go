package tpkgdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/petar/GoLLRB/llrb"
	"github.com/tardigradeos/tpkg/tpkgfs"
	"github.com/tardigradeos/tpkg/tpkgsig"
)

func (d *DBData) index() error {
	if string(d.data[:4]) != "TPDB" {
		return errors.New("not a tpkgdb file")
	}

	r := bytes.NewReader(d.data)
	r.Seek(4, io.SeekStart)

	// read version
	err := binary.Read(r, binary.BigEndian, &d.version)
	if err != nil {
		return err
	}
	if d.version != 1 {
		return errors.New("unsupported db version")
	}

	err = binary.Read(r, binary.BigEndian, &d.flags)
	if err != nil {
		return err
	}

	created := make([]int64, 2)
	err = binary.Read(r, binary.BigEndian, created)
	if err != nil {
		return err
	}
	d.created = time.Unix(created[0], created[1])

	log.Printf("tpkgdb: reading database generated on %s (%s ago)", d.created, time.Since(d.created))

	osarchcnt := make([]uint32, 3)
	err = binary.Read(r, binary.BigEndian, osarchcnt)
	if err != nil {
		return err
	}

	// TODO check values
	d.os = osarchcnt[0]   // 0=linux 1=darwin 2=windows ...
	d.arch = osarchcnt[1] // 0=i386 1=amd64 ...
	d.count = osarchcnt[2]

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
	headerData := d.data[:196]
	// seek at signature location
	r.Seek(196, io.SeekStart)
	err = tpkgsig.VerifyDb(headerData, r)
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
		name, err := tpkgsig.ReadVarblob(r, 256)
		if err != nil {
			return err
		}

		// read path
		path, err := tpkgsig.ReadVarblob(r, 256)
		if err != nil {
			return err
		}

		pkg.rawHeader, err = tpkgsig.ReadVarblob(r, 256)
		if err != nil {
			return err
		}
		pkg.rawSig, err = tpkgsig.ReadVarblob(r, tpkgsig.SignatureSize)
		if err != nil {
			return err
		}
		pkg.rawMeta, err = tpkgsig.ReadVarblob(r, 65536)
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

func (d *DBData) lookupInode(reqino uint64) (tpkgfs.Inode, error) {
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
