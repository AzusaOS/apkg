package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"git.atonline.com/azusa/apkg/apkgdb"
	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/MagicalTux/hsm"
)

type fileKey struct {
	arch string
	os   string
}

type dbFile struct {
	f     *os.File
	name  string
	path  string
	stamp string
	arch  string
	os    string

	ino uint64
	cnt uint32

	idxFN  map[string]int64
	idxIno map[uint64]int64

	w    io.Writer
	hash hash.Hash
}

func processDb(name string, k hsm.Key) error {
	// instanciate db
	tempDir, err := ioutil.TempDir("", "apkgidx")
	if err != nil {
		return err
	}

	dir := filepath.Join(os.Getenv("HOME"), "projects/apkg-tools/repo/apkg/dist", name)
	files := make(map[fileKey]*apkgdb.DB)

	err = filepath.Walk(dir, func(fpath string, info os.FileInfo, err error) error {
		if !info.Mode().IsRegular() {
			return nil
		}
		if !strings.HasSuffix(fpath, ".apkg") {
			return nil
		}

		rpath := strings.TrimLeft(strings.TrimPrefix(fpath, dir), "/")

		if info.Size() == 0 {
			// special case: remove package
			log.Printf("Removing: %s", rpath)
			// core/symlinks/core.symlinks.0.0.2.linux.amd64-5d569d7.apkg
			if r := regexp.MustCompile(".*\\.([a-z]+)\\.([a-z0-9]+)-([a-f0-9]{7})\\.apkg$").FindStringSubmatch(rpath); r != nil {
				fk := fileKey{arch: r[2], os: r[1]}
				db, ok := files[fk]
				if !ok {
					// invoking db here will cause download of the whole db as currently known
					db, err = apkgdb.NewOsArch(apkgdb.PKG_URL_PREFIX, name, path.Join(tempDir, fk.os, fk.arch), fk.os, fk.arch)
					if err != nil {
						return err
					}

					files[fk] = db
				}

				err = db.RemovePackage(strings.TrimSuffix(filepath.Base(rpath), "-"+r[3]+".apkg"))
				if err != nil {
					return err
				}
			}
			return nil
		}

		f, err := os.Open(fpath)
		if err != nil {
			return err
		}
		defer f.Close()

		log.Printf("Indexing: %s", rpath)
		p, err := (*apkgdb.DB)(nil).OpenPackage(f)
		if err != nil {
			return err
		}

		var meta pkgMeta
		err = p.Meta(&meta)
		if err != nil {
			return err
		}

		fk := fileKey{arch: meta.Arch, os: meta.Os}
		db, ok := files[fk]
		if !ok {
			// invoking db here will cause download of the whole db as currently known
			db, err = apkgdb.NewOsArch(apkgdb.PKG_URL_PREFIX, name, path.Join(tempDir, meta.Os, meta.Arch), meta.Os, meta.Arch)
			if err != nil {
				return err
			}

			files[fk] = db
		}

		err = db.AddPackage(rpath, info, p)
		if err != nil {
			log.Printf("failed to index package: %s", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, db := range files {
		err = db.ExportAndUpload(k)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *dbFile) init(now time.Time) error {
	log.Printf("Initializing database file %s", db.path)
	// write header to file
	db.f.Write([]byte("APDB"))
	binary.Write(db.f, binary.BigEndian, uint32(0x00000001)) // version
	binary.Write(db.f, binary.BigEndian, uint64(0))          // flags
	binary.Write(db.f, binary.BigEndian, uint64(now.Unix()))
	binary.Write(db.f, binary.BigEndian, uint64(now.Nanosecond()))

	var os uint32
	var arch uint32
	switch db.os {
	case "linux":
		os = 0
	case "darwin":
		os = 1
	case "windows":
		os = 2
	default:
		return errors.New("unsupported os")
	}

	switch db.arch {
	case "386":
		arch = 0
	case "amd64":
		arch = 1
	default:
		return errors.New("unsupported arch")
	}

	binary.Write(db.f, binary.BigEndian, os)
	binary.Write(db.f, binary.BigEndian, arch)
	// 40 (pkg count)
	binary.Write(db.f, binary.BigEndian, uint32(0)) // offset 40: number of packages (filled at the end)

	nameBuf := make([]byte, 32)
	copy(nameBuf, db.name)
	db.f.Write(nameBuf)

	// SHA256('') = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	emptyHash, _ := hex.DecodeString("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	// 76
	binary.Write(db.f, binary.BigEndian, uint32(196+apkgsig.SignatureSize)) // location of data
	binary.Write(db.f, binary.BigEndian, uint32(0))                         // length of data
	db.f.Write(emptyHash)                                                   // hash of data
	// 110
	binary.Write(db.f, binary.BigEndian, uint32(0)) // location of id index
	binary.Write(db.f, binary.BigEndian, uint32(0)) // length of id index
	db.f.Write(emptyHash)                           // hash of id index
	// 156
	binary.Write(db.f, binary.BigEndian, uint32(0)) // location of name index
	binary.Write(db.f, binary.BigEndian, uint32(0)) // length of name index
	db.f.Write(emptyHash)                           // hash of name index

	n, _ := db.f.Seek(0, io.SeekCurrent)

	if n != 196 {
		return errors.New("invalid header length")
	}

	db.f.Write(make([]byte, apkgsig.SignatureSize)) // reserved space for signature

	db.hash = sha256.New()
	db.w = io.MultiWriter(db.f, db.hash)

	return nil
}

func (db *dbFile) index(rpath string, info os.FileInfo, p *pkginfo) {
	// write package to list & store position details
	pos, _ := db.f.Seek(0, io.SeekCurrent)
	db.idxFN[p.meta.FullName] = pos
	db.idxIno[db.ino] = pos
	db.ino += uint64(p.meta.Inodes) + 1
	db.cnt += 1

	db.w.Write([]byte{0}) // package
	db.w.Write(p.headerHash[:])
	binary.Write(db.w, binary.BigEndian, uint64(info.Size()))
	binary.Write(db.w, binary.BigEndian, p.meta.Inodes)

	apkgsig.WriteVarblob(db.w, []byte(p.meta.FullName))
	apkgsig.WriteVarblob(db.w, []byte(rpath))
	apkgsig.WriteVarblob(db.w, p.rawHeader)
	apkgsig.WriteVarblob(db.w, p.rawSig)
	apkgsig.WriteVarblob(db.w, p.rawMeta)
}

func (db *dbFile) finalize(k hsm.Key) error {
	// compute hash, etc
	pos, _ := db.f.Seek(0, io.SeekCurrent)
	hash := db.hash.Sum(nil)

	// write to header
	db.w = nil
	db.hash = nil

	db.f.Seek(40, io.SeekStart)
	binary.Write(db.f, binary.BigEndian, db.cnt) // pkg count

	db.f.Seek(76, io.SeekStart) // length of data, data starts at 196+128
	var start uint32
	binary.Read(db.f, binary.BigEndian, &start)             // should be reading 196+128
	binary.Write(db.f, binary.BigEndian, uint32(pos)-start) // write length of data
	db.f.Write(hash)                                        // write hash of data

	// TODO: index, etc

	// compute header signature
	header := make([]byte, 196)
	_, err := db.f.ReadAt(header, 0)
	if err != nil {
		return err
	}

	log.Printf("Signing %s...", db.path)

	sigB := &bytes.Buffer{}
	vInt := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(vInt, 0x0001) // Signature type 1 = ed25519
	sigB.Write(vInt[:n])

	sig_pub, err := k.PublicBlob()
	if err != nil {
		return err
	}

	apkgsig.WriteVarblob(sigB, sig_pub)

	// use raw hash for ed25519
	sig_blob, err := k.Sign(rand.Reader, header, crypto.Hash(0))
	if err != nil {
		return err
	}
	apkgsig.WriteVarblob(sigB, sig_blob)

	// verify signature
	_, err = apkgsig.VerifyDb(header, bytes.NewReader(sigB.Bytes()))
	if err != nil {
		return err
	}

	if sigB.Len() > apkgsig.SignatureSize {
		return errors.New("signature buffer not large enough!")
	}

	db.f.Seek(196, io.SeekStart)
	db.f.Write(sigB.Bytes())

	db.f.Close()

	err = os.Rename(db.path+"~", db.path)
	if err != nil {
		return err
	}

	// update LATEST.txt
	lat, err := os.Create(filepath.Join(filepath.Dir(db.path), "LATEST.txt"))
	if err != nil {
		return err
	}
	fmt.Fprintf(lat, "%s\n", db.stamp)
	lat.Close()

	return nil
}

func (db *dbFile) upload() error {
	// upload file to s3
	s3pfx := "s3:/" + path.Join("/azusa-pkg/db", db.name, db.os, db.arch)
	log.Printf("uploading files to %s", s3pfx)

	//system('aws s3 cp --cache-control max-age=31536000 '.escapeshellarg($db_path.'/'.$datestamp.'.bin').' '.escapeshellarg($s3_prefix.'/'.$datestamp.'.bin'));
	cmd1 := exec.Command("aws", "s3", "cp", "--cache-control", "max-age=31536000", db.path, s3pfx+"/"+db.stamp+".bin")
	cmd1.Stdout = os.Stdout
	cmd1.Stderr = os.Stderr
	err := cmd1.Run()
	if err != nil {
		return err
	}

	//system('aws s3 cp --cache-control max-age=60 '.escapeshellarg($db_path.'/LATEST.txt').' '.escapeshellarg($s3_prefix.'/LATEST.txt'));
	cmd2 := exec.Command("aws", "s3", "cp", "--cache-control", "max-age=60", filepath.Dir(db.path)+"/LATEST.txt", s3pfx+"/LATEST.txt")
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr

	return cmd2.Run()
}
