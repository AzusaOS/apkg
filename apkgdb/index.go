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

	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/boltdb/bolt"
)

func (d *DB) index(r *os.File) error {
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

	// hash the data area
	hash := sha256.New()
	r.Seek(int64(dataLoc[0]), io.SeekStart)
	io.CopyN(hash, r, int64(dataLoc[1]))
	dataHashChk := hash.Sum(nil)

	if !bytes.Equal(dataHash, dataHashChk) {
		return errors.New("invalid data hash")
	}

	// grab the header only
	r.Seek(0, io.SeekStart)
	headerData := make([]byte, 196)
	_, err = io.ReadFull(r, headerData)
	if err != nil {
		return err
	}

	// seek at signature location
	r.Seek(196, io.SeekStart)
	_, err = apkgsig.VerifyDb(headerData, bufio.NewReader(r))
	if err != nil {
		return err
	}

	// TODO → use indices

	r.Seek(int64(dataLoc[0]), io.SeekStart)

	// let's use a limited read buffer so we don't expand over hashed area
	b := bufio.NewReader(&io.LimitedReader{R: r, N: int64(dataLoc[1])})

	if err := d.writeStart(); err != nil {
		return err
	}
	defer d.writeEnd()

	// initialize a write transaction
	err = d.dbptr.Update(func(tx *bolt.Tx) error {
		// create/get buckets
		infoB, err := tx.CreateBucketIfNotExists([]byte("info"))
		if err != nil {
			return err
		}
		p2pB, err := tx.CreateBucketIfNotExists([]byte("p2p"))
		if err != nil {
			return err
		}
		pkgB, err := tx.CreateBucketIfNotExists([]byte("pkg"))
		if err != nil {
			return err
		}
		headerB, err := tx.CreateBucketIfNotExists([]byte("header"))
		if err != nil {
			return err
		}
		sigB, err := tx.CreateBucketIfNotExists([]byte("sig"))
		if err != nil {
			return err
		}
		metaB, err := tx.CreateBucketIfNotExists([]byte("meta"))
		if err != nil {
			return err
		}
		pathB, err := tx.CreateBucketIfNotExists([]byte("path"))
		if err != nil {
			return err
		}
		ldsoB, err := tx.CreateBucketIfNotExists([]byte("ldso"))
		if err != nil {
			return err
		}

		// OK now let's read each package
		for i := uint32(0); i < count; i++ {
			var t uint8
			err = binary.Read(b, binary.BigEndian, &t)
			if err != nil {
				return err
			}
			if t != 0 {
				return fmt.Errorf("invalid data in db (invalid package type %d)", t)
			}

			// let's read the package hash & other info
			hash := make([]byte, 32)
			_, err = io.ReadFull(b, hash)
			if err != nil {
				return err
			}

			// read size
			var size uint64
			err = binary.Read(b, binary.BigEndian, &size)
			if err != nil {
				return err
			}

			var inodes uint32
			err = binary.Read(b, binary.BigEndian, &inodes)
			if err != nil {
				return err
			}

			// read name
			name, err := apkgsig.ReadVarblob(b, 256)
			if err != nil {
				return err
			}

			// read path
			path, err := apkgsig.ReadVarblob(b, 256)
			if err != nil {
				return err
			}

			rawHeader, err := apkgsig.ReadVarblob(b, 256)
			if err != nil {
				return err
			}
			rawSig, err := apkgsig.ReadVarblob(b, apkgsig.SignatureSize)
			if err != nil {
				return err
			}
			rawMeta, err := apkgsig.ReadVarblob(b, 1024*1024)
			if err != nil {
				return err
			}

			//log.Printf("apkgdb: read from db pkg %s size=%d inodes=%d", name, size, inodes)
			log.Printf("meta = %s", rawMeta)
			_ = ldsoB

			// do we already have this hash?
			exInfo := pkgB.Get(hash)
			if exInfo != nil {
				// TODO Check if same package or not
				continue
			}

			nameC := collatedVersion(string(name))
			sizeB := make([]byte, 8)
			binary.BigEndian.PutUint64(sizeB, size)
			inoCountB := make([]byte, 8)
			binary.BigEndian.PutUint64(inoCountB, uint64(inodes))

			// store stuff
			err = p2pB.Put(nameC, append(append(append([]byte(nil), hash...), inoCountB...), name...))
			if err != nil {
				return err
			}
			inoBin := make([]byte, 8)
			err = pkgB.Put(hash, append(append(append(append([]byte{0}, sizeB...), inoBin...), inoCountB...), name...))
			if err != nil {
				return err
			}
			err = headerB.Put(hash, rawHeader)
			if err != nil {
				return err
			}
			err = sigB.Put(hash, rawSig)
			if err != nil {
				return err
			}
			err = metaB.Put(hash, rawMeta)
			if err != nil {
				return err
			}
			err = pathB.Put(hash, path)
			if err != nil {
				return err
			}

			//log.Printf("read package %s size=%d", name, size)
		}

		// store version
		infoB.Put([]byte("version"), []byte(created.UTC().Format("20060102150405")))

		// cause commit to happen
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (d *DB) AddPackage(rpath string, info os.FileInfo, p *Package) error {
	if err := d.writeStart(); err != nil {
		return err
	}
	defer d.writeEnd()

	// add package to database if not exist

	// initialize a write transaction
	return d.dbptr.Update(func(tx *bolt.Tx) error {
		// create/get buckets
		p2pB, err := tx.CreateBucketIfNotExists([]byte("p2p"))
		if err != nil {
			return err
		}
		pkgB, err := tx.CreateBucketIfNotExists([]byte("pkg"))
		if err != nil {
			return err
		}
		headerB, err := tx.CreateBucketIfNotExists([]byte("header"))
		if err != nil {
			return err
		}
		sigB, err := tx.CreateBucketIfNotExists([]byte("sig"))
		if err != nil {
			return err
		}
		metaB, err := tx.CreateBucketIfNotExists([]byte("meta"))
		if err != nil {
			return err
		}
		pathB, err := tx.CreateBucketIfNotExists([]byte("path"))
		if err != nil {
			return err
		}

		exInfo := pkgB.Get(p.hash)
		if exInfo != nil {
			// we already have this package
			return errors.New("package already in database")
		}

		nameC := collatedVersion(string(p.name))
		sizeB := make([]byte, 8)
		binary.BigEndian.PutUint64(sizeB, p.size)
		inoCountB := make([]byte, 8)
		binary.BigEndian.PutUint64(inoCountB, uint64(p.inodes))

		// store all
		err = p2pB.Put(nameC, append(append(append([]byte(nil), p.hash...), inoCountB...), p.name...))
		if err != nil {
			return err
		}
		inoBin := make([]byte, 8)
		err = pkgB.Put(p.hash, append(append(append(append([]byte{0}, sizeB...), inoBin...), inoCountB...), p.name...))
		if err != nil {
			return err
		}
		err = headerB.Put(p.hash, p.rawHeader)
		if err != nil {
			return err
		}
		err = sigB.Put(p.hash, p.rawSig)
		if err != nil {
			return err
		}
		err = metaB.Put(p.hash, p.rawMeta)
		if err != nil {
			return err
		}
		err = pathB.Put(p.hash, []byte(rpath))
		if err != nil {
			return err
		}

		//log.Printf("read package %s size=%d", name, size)
		return nil
	})
}

func (d *DB) RemovePackage(name string) error {
	// lookup & remove package
	log.Printf("apkgdb: removing %s", name)

	if err := d.writeStart(); err != nil {
		return err
	}
	defer d.writeEnd()

	return d.dbptr.Update(func(tx *bolt.Tx) error {
		p2pB := tx.Bucket([]byte("p2p")) // we use p2p for the foreach in export too, removing from here is enough
		nameC := collatedVersion(name)

		return p2pB.Delete(nameC)
	})
}
