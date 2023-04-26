package apkgdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"time"

	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/KarpelesLab/hsm"
	"github.com/KarpelesLab/jwt"
	"github.com/boltdb/bolt"
)

func (d *DB) ExportAndUpload(k hsm.Key) error {
	// generate a binary file with the full db, and upload it

	now := time.Now()
	stamp := now.UTC().Format("20060102150405")

	fn := path.Join(d.path, stamp+".bin")

	log.Printf("apkgdb: creating database export version %s", stamp)

	f, err := os.Create(fn)
	if err != nil {
		return err
	}

	f.Write([]byte("APDB"))
	binary.Write(f, binary.BigEndian, uint32(0x00000001)) // version
	binary.Write(f, binary.BigEndian, uint64(0))          // flags
	binary.Write(f, binary.BigEndian, uint64(now.Unix()))
	binary.Write(f, binary.BigEndian, uint64(now.Nanosecond()))

	fos := ParseOS(d.os)
	farch := ParseArch(d.arch)

	binary.Write(f, binary.BigEndian, fos)
	binary.Write(f, binary.BigEndian, farch)
	// 40 (pkg count)
	binary.Write(f, binary.BigEndian, uint32(0)) // offset 40: number of packages (filled at the end of export)

	nameBuf := make([]byte, 32)
	copy(nameBuf, d.name)
	f.Write(nameBuf)

	// SHA256('') = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	emptyHash, _ := hex.DecodeString("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	// 76
	binary.Write(f, binary.BigEndian, uint32(196+apkgsig.SignatureSize)) // location of data
	binary.Write(f, binary.BigEndian, uint32(0))                         // length of data
	f.Write(emptyHash)                                                   // hash of data
	// 110
	binary.Write(f, binary.BigEndian, uint32(0)) // location of id index
	binary.Write(f, binary.BigEndian, uint32(0)) // length of id index
	f.Write(emptyHash)                           // hash of id index
	// 156
	binary.Write(f, binary.BigEndian, uint32(0)) // location of name index
	binary.Write(f, binary.BigEndian, uint32(0)) // length of name index
	f.Write(emptyHash)                           // hash of name index

	n, _ := f.Seek(0, io.SeekCurrent)

	if n != 196 {
		return errors.New("invalid header length")
	}

	f.Write(make([]byte, apkgsig.SignatureSize)) // reserved space for signature

	hash := sha256.New()
	w := io.MultiWriter(f, hash)
	var count uint32    // packages count
	var datasize uint64 // total size

	var unlkOnce sync.Once
	unlk := func() {
		unlkOnce.Do(d.dbrw.RUnlock)
	}

	d.dbrw.RLock()
	defer unlk()

	// write files
	err = d.dbptr.View(func(tx *bolt.Tx) error {
		// get all the buckets we need
		p2pB := tx.Bucket([]byte("p2p")) // we use p2p for the foreach in order to get packages in the right order
		pkgB := tx.Bucket([]byte("pkg"))
		headerB := tx.Bucket([]byte("header"))
		sigB := tx.Bucket([]byte("sig"))
		metaB := tx.Bucket([]byte("meta"))
		pathB := tx.Bucket([]byte("path"))

		return p2pB.ForEach(func(k, v []byte) error {
			h := v[:32]

			// load info
			pkg := pkgB.Get(h)

			// write package data to disk
			w.Write([]byte{0})
			w.Write(h)
			w.Write(pkg[1:9])       // size is already bigendian in our database
			w.Write(pkg[17+4 : 25]) // inodes count, already big endian but needs to be made uint32

			apkgsig.WriteVarblob(w, pkg[25:]) // name
			apkgsig.WriteVarblob(w, pathB.Get(h))
			apkgsig.WriteVarblob(w, headerB.Get(h))
			apkgsig.WriteVarblob(w, sigB.Get(h))
			apkgsig.WriteVarblob(w, metaB.Get(h))

			count += 1
			datasize += binary.BigEndian.Uint64(pkg[1:9])
			return nil
		})
	})

	unlk()

	if err != nil {
		return err
	}

	pos, _ := f.Seek(0, io.SeekCurrent) // size of file
	finalHash := hash.Sum(nil)

	w = nil
	hash = nil

	f.Seek(40, io.SeekStart)
	binary.Write(f, binary.BigEndian, count) // pkg count

	f.Seek(76, io.SeekStart) // length of data, data starts at 196+128
	var start uint32
	binary.Read(f, binary.BigEndian, &start)             // should be reading 196+128
	binary.Write(f, binary.BigEndian, uint32(pos)-start) // write length of data
	f.Write(finalHash)

	// compute header signature
	header := make([]byte, 196)
	_, err = f.ReadAt(header, 0)
	if err != nil {
		return err
	}

	log.Printf("apkgdb: Exported %d packages (%s), signing...", count, formatSize(datasize))
	sigB, err := apkgsig.Sign(k, header)
	if err != nil {
		return err
	}

	// verify signature
	_, err = apkgsig.VerifyDb(header, bytes.NewReader(sigB))
	if err != nil {
		return err
	}

	f.Seek(196, io.SeekStart)
	f.Write(sigB)

	f.Close()

	// call index on file to check if the generated file is 100% valid
	f, err = os.Open(fn)
	if err != nil {
		return err // ???
	}

	err = d.index(f)
	f.Close()
	if err != nil {
		return err // failed to index: must be an error in file creation
	}

	// Generate LATEST.txt
	lat, err := os.Create(path.Join(d.path, "LATEST.txt"))
	if err != nil {
		return err
	}
	fmt.Fprintf(lat, "%s\n", stamp)
	lat.Close()

	// generate LATEST.jwt
	sig_pub, err := k.PublicBlob()
	if err != nil {
		return err
	}

	lat, err = os.Create(path.Join(d.path, "LATEST.jwt"))
	if err != nil {
		return err
	}
	token := jwt.New(jwt.EdDSA)
	token.Payload().Set("ver", stamp)
	token.Payload().Set("arch", d.arch)
	token.Payload().Set("os", d.os)
	token.Payload().Set("name", d.name)
	token.Header().Set("kid", base64.RawURLEncoding.EncodeToString(sig_pub))
	tokenString, err := token.Sign(k)
	if err != nil {
		return err
	}
	fmt.Fprintf(lat, "%s\n", tokenString)
	lat.Close()

	// upload database
	s3pfx := "s3:/" + path.Join("/azusa-pkg/db", d.name, d.os, d.arch)
	log.Printf("apkgdb: uploading files to %s", s3pfx)

	//system('aws s3 cp --cache-control max-age=31536000 '.escapeshellarg($db_path.'/'.$datestamp.'.bin').' '.escapeshellarg($s3_prefix.'/'.$datestamp.'.bin'));
	cmd1 := exec.Command("aws", "s3", "cp", "--cache-control", "max-age=31536000", "--content-type", "application/azusa-apkg-db", fn, s3pfx+"/"+stamp+".bin")
	cmd1.Stdout = os.Stdout
	cmd1.Stderr = os.Stderr
	err = cmd1.Run()
	if err != nil {
		return err
	}

	//system('aws s3 cp --cache-control max-age=60 '.escapeshellarg($db_path.'/LATEST.txt').' '.escapeshellarg($s3_prefix.'/LATEST.txt'));
	cmd2 := exec.Command("aws", "s3", "cp", "--cache-control", "max-age=60", "--content-type", "text/plain", filepath.Join(d.path, "LATEST.txt"), s3pfx+"/LATEST.txt")
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	err = cmd2.Run()
	if err != nil {
		return err
	}

	cmd3 := exec.Command("aws", "s3", "cp", "--cache-control", "max-age=60", "--content-type", "text/plain", filepath.Join(d.path, "LATEST.jwt"), s3pfx+"/LATEST.jwt")
	cmd3.Stdout = os.Stdout
	cmd3.Stderr = os.Stderr
	return cmd3.Run()
}

func formatSize(v uint64) string {
	var siz = [...]struct {
		unit string
		size float32
	}{
		{unit: "B", size: 1},
		{unit: "kiB", size: 1024},
		{unit: "MiB", size: 1024},
		{unit: "GiB", size: 1024},
		{unit: "TiB", size: 1024},
		{unit: "PiB", size: 1024},
	}

	if v == 0 {
		return "0 B"
	}

	vf := float32(v)
	last := siz[0]
	for _, i := range siz {
		if vf < i.size*1.5 {
			break
		}
		vf = vf / i.size
		last = i
	}

	if last.size > 1 {
		return fmt.Sprintf("%01.2f %s", vf, last.unit)
	} else {
		return fmt.Sprintf("%.0f %s", vf, last.unit)
	}
}
