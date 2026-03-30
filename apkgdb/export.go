package apkgdb

import (
	"bytes"
	"crypto/rand"
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

	"github.com/AzusaOS/apkg/apkgsig"
	"github.com/KarpelesLab/hsm"
	"github.com/KarpelesLab/jwt"
	bolt "go.etcd.io/bbolt"
)

// ExportAndUpload exports the database to a binary file, signs it,
// and uploads it to the configured S3 bucket.
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

	if _, err := f.Write([]byte("APDB")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.BigEndian, uint32(0x00000001)); err != nil { // version
		return err
	}
	if err := binary.Write(f, binary.BigEndian, uint64(0)); err != nil { // flags
		return err
	}
	if err := binary.Write(f, binary.BigEndian, uint64(now.Unix())); err != nil {
		return err
	}
	if err := binary.Write(f, binary.BigEndian, uint64(now.Nanosecond())); err != nil {
		return err
	}

	fos := ParseOS(d.os)
	farch := ParseArch(d.arch)

	if err := binary.Write(f, binary.BigEndian, fos); err != nil {
		return err
	}
	if err := binary.Write(f, binary.BigEndian, farch); err != nil {
		return err
	}
	// 40 (pkg count)
	if err := binary.Write(f, binary.BigEndian, uint32(0)); err != nil { // offset 40: number of packages (filled at the end of export)
		return err
	}

	nameBuf := make([]byte, 32)
	copy(nameBuf, d.name)
	if _, err := f.Write(nameBuf); err != nil {
		return err
	}

	// SHA256('') = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	emptyHash, _ := hex.DecodeString("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	// 76
	for _, v := range []interface{}{
		uint32(196 + apkgsig.SignatureSize), uint32(0), // data location + length
	} {
		if err := binary.Write(f, binary.BigEndian, v); err != nil {
			return err
		}
	}
	if _, err := f.Write(emptyHash); err != nil { // hash of data
		return err
	}
	// 110
	for _, v := range []interface{}{uint32(0), uint32(0)} { // id index location + length
		if err := binary.Write(f, binary.BigEndian, v); err != nil {
			return err
		}
	}
	if _, err := f.Write(emptyHash); err != nil { // hash of id index
		return err
	}
	// 156
	for _, v := range []interface{}{uint32(0), uint32(0)} { // name index location + length
		if err := binary.Write(f, binary.BigEndian, v); err != nil {
			return err
		}
	}
	if _, err := f.Write(emptyHash); err != nil { // hash of name index
		return err
	}

	n, _ := f.Seek(0, io.SeekCurrent)

	if n != 196 {
		return errors.New("invalid header length")
	}

	if _, err := f.Write(make([]byte, apkgsig.SignatureSize)); err != nil { // reserved space for signature
		return err
	}

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

		if err := p2pB.ForEach(func(k, v []byte) error {
			h := v[:32]

			// load info
			pkg := pkgB.Get(h)

			// write package data to disk
			for _, b := range [][]byte{{0}, h, pkg[1:9], pkg[17+4 : 25]} {
				if _, err := w.Write(b); err != nil {
					return err
				}
			}

			for _, b := range [][]byte{pkg[25:], pathB.Get(h), headerB.Get(h), sigB.Get(h), metaB.Get(h)} {
				if err := apkgsig.WriteVarblob(w, b); err != nil {
					return err
				}
			}

			count += 1
			datasize += binary.BigEndian.Uint64(pkg[1:9])
			return nil
		}); err != nil {
			return err
		}

		// Write pin entries (type 0x01) after packages
		pinsB := tx.Bucket([]byte("pins"))
		if pinsB != nil {
			return pinsB.ForEach(func(k, v []byte) error {
				// key is "channel\x00prefix", value is version
				if _, err := w.Write([]byte{0x01}); err != nil {
					return err
				}
				sep := bytes.IndexByte(k, 0x00)
				if sep == -1 {
					return nil // malformed key, skip
				}
				ch := k[:sep]
				pfx := k[sep+1:]
				for _, b := range [][]byte{ch, pfx, v} {
					if err := apkgsig.WriteVarblob(w, b); err != nil {
						return err
					}
				}
				return nil
			})
		}

		return nil
	})

	unlk()

	if err != nil {
		return err
	}

	pos, _ := f.Seek(0, io.SeekCurrent) // size of file
	finalHash := hash.Sum(nil)

	w = nil
	hash = nil

	if _, err = f.Seek(40, io.SeekStart); err != nil {
		return err
	}
	if err = binary.Write(f, binary.BigEndian, count); err != nil { // pkg count
		return err
	}

	if _, err = f.Seek(76, io.SeekStart); err != nil { // length of data, data starts at 196+128
		return err
	}
	var start uint32
	if err = binary.Read(f, binary.BigEndian, &start); err != nil { // should be reading 196+128
		return err
	}
	if err = binary.Write(f, binary.BigEndian, uint32(pos)-start); err != nil { // write length of data
		return err
	}
	if _, err = f.Write(finalHash); err != nil {
		return err
	}

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

	if _, err = f.Seek(196, io.SeekStart); err != nil {
		return err
	}
	if _, err = f.Write(sigB); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

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
	if err := lat.Close(); err != nil {
		return err
	}

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
	for k, v := range map[string]string{"ver": stamp, "arch": d.arch, "os": d.os, "name": d.name} {
		if err = token.Payload().Set(k, v); err != nil {
			return err
		}
	}
	if err = token.Header().Set("kid", base64.RawURLEncoding.EncodeToString(sig_pub)); err != nil {
		return err
	}
	tokenString, err := token.Sign(rand.Reader, k)
	if err != nil {
		return err
	}
	fmt.Fprintf(lat, "%s\n", tokenString)
	if err := lat.Close(); err != nil {
		return err
	}

	// upload database
	s3pfxCf := "s3:/" + path.Join("/azusa/db", d.name, d.os, d.arch)
	log.Printf("apkgdb: uploading files to cf:%s", s3pfxCf)

	commands := [][]string{
		[]string{"aws", "s3", "--profile", "cf", "cp", "--cache-control", "max-age=31536000", fn, s3pfxCf + "/" + stamp + ".bin"},
		[]string{"aws", "s3", "--profile", "cf", "cp", "--cache-control", "max-age=60", filepath.Join(d.path, "/LATEST.txt"), s3pfxCf + "/LATEST.txt"},
		[]string{"aws", "s3", "--profile", "cf", "cp", "--cache-control", "max-age=60", "--content-type", "text/plain", filepath.Join(d.path, "LATEST.jwt"), s3pfxCf + "/LATEST.jwt"},
	}

	for _, c := range commands {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
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
