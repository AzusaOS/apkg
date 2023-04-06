package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/KarpelesLab/squashfs"
	"github.com/MagicalTux/hsm"
)

const HEADER_LEN = 124

func process(k hsm.Key, filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	s, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := s.Size()

	log.Printf("preparing %s ...", filename)
	sb, err := squashfs.New(f)
	if err != nil {
		return err
	}

	// compute hash table
	var hashtable []byte

	reserveIno := sb.InodeCnt
	blockSize := int64(4096)
	blocks := 0

	// try to find a good ratio for block size vs table size
	for ((fileSize / blockSize) > 1500) && (blockSize < 131072) {
		blockSize = blockSize << 1
	}

	buf := make([]byte, blockSize)
	for i := int64(0); i < fileSize; i += blockSize {
		n, err := f.ReadAt(buf, i)
		if err != nil {
			if !(err == io.EOF && n != 0) {
				return err
			}
		}
		h := sha256.Sum256(buf[:n])
		hashtable = append(hashtable, h[:]...)
		blocks += 1
	}

	tableHash := sha256.Sum256(hashtable)
	log.Printf("table len = %d bytes (%d blocks)", len(hashtable), blocks)
	log.Printf("table hash = %s", hex.EncodeToString(tableHash[:]))

	filename_f := strings.TrimSuffix(filepath.Base(filename), ".squashfs")

	fn_a := strings.Split(filename_f, ".")
	// cat.name.subcat.1.2.3.linux.amd64

	arch_s := fn_a[len(fn_a)-1]
	os_s := fn_a[len(fn_a)-2]
	fn_a = fn_a[:len(fn_a)-2]

	cat_s := fn_a[0]
	name_s := fn_a[1]
	subcat_s := fn_a[2]

	fn_v := fn_a[3:]
	fn_a = fn_a[:3]

	tmp := fn_a
	names := []string{strings.Join(tmp, ".")}
	for i := 0; i < len(fn_v); i++ {
		tmp = append(tmp, fn_v[i])
		names = append(names, strings.Join(tmp, "."))
	}

	// fetch created from squashfs mkfs date (superblock ModTime?)
	created := time.Unix(int64(sb.ModTime), 0)

	// scan squashfs file for the following kind of files:
	// pkgconfig/*.pc (if subcat_s = dev)
	// bin/* (with +x) (if subcat_s = core|dev)
	// sbin/* (with +x) (if subcat_s = core|dev)
	// lib/* (with +x, or symlinks) (if subcat_s = libs)
	// lib32|64/* (with +x, or symlinks) (if subcat_s = libs)
	// those are to be added to metadata in "provides"

	// Also scan & include actual file content of:
	// /.ld.so.cache (if subcat_s = libs)

	// we also have a special case of some packages that need to provide symlinks to be shared in virtual views, for stuff like python modules.
	// we do that through a special .virtual folder at the squashfs root that can contain directories (virtual module names), each containign symlinks
	// example: .virtual/python-modules-3.2.1/pythonmodulename → ../../pythonmodulename

	provides := make(map[string]any)
	var provideGlob []string

	virtual := make(map[string]map[string]string)
	if virtList, _ := sb.ReadDir(".virtual"); len(virtList) > 0 {
		for _, pkginfo := range virtList {
			if !pkginfo.IsDir() {
				continue
			}
			pkg := pkginfo.Name()
			sub := make(map[string]string)
			symList, _ := sb.ReadDir(path.Join(".virtual", pkg))
			for _, syminfo := range symList {
				sym := syminfo.Name()
				// all these should be symlinks, we'll ignore if not
				// also, symlinks must start with "../../" as these must be local to the package (../../ will be removed)
				tgt, err := sb.Readlink(path.Join(".virtual", pkg, sym))
				if err == nil && strings.HasPrefix(string(tgt), "../../") {
					sub[sym] = strings.TrimPrefix(string(tgt), "../../")
					log.Printf("provides virtual: %s %s → %s", pkg, sym, sub[sym])
				}
			}
			if len(sub) > 0 {
				virtual[pkg] = sub
			}
		}
	}

	// we define metadata now so we can add to it as we check subcat_s
	metadata := map[string]any{
		"full_name":  filename_f,
		"name":       strings.Join(fn_a, "."),
		"version":    strings.Join(fn_v, "."),
		"names":      names,
		"os":         os_s,
		"arch":       arch_s,
		"category":   cat_s,
		"base_name":  name_s,
		"subcat":     subcat_s,
		"size":       s.Size(),
		"hash":       hex.EncodeToString(tableHash[:]),
		"blocks":     blocks,
		"block_size": blockSize,
		"inodes":     reserveIno,
		"created":    []int64{created.Unix(), int64(created.Nanosecond())},
	}
	if len(virtual) > 0 {
		metadata["virtual"] = virtual
	}

	switch subcat_s {
	case "core":
		provideGlob = append(provideGlob, "bin/*", "sbin/*", "udev/*")
	case "libs":
		// check for /.ld.so.cache
		if buf, err := fs.ReadFile(sb, ".ld.so.cache"); err == nil {
			// This is a special case where we include the whole ld.so.cache content in metadata
			// TODO check length and prevent file from growing too much
			metadata["ld.so.cache"] = base64.StdEncoding.EncodeToString(buf)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("while reading .ld.so.cache: %w", err)
		}
		if arch_s == "amd64" {
			provideGlob = append(provideGlob, "lib32/*", "lib64/*")
		} else {
			provideGlob = append(provideGlob, "lib/*")
		}
	case "dev":
		provideGlob = append(provideGlob, "pkgconfig/*.pc", "cmake/*", "bin/*", "sbin/*")
	case "mod":
		provideGlob = append(provideGlob, "pkgconfig/*.pc", "cmake/*", "bin/*", "sbin/*", "lib/python*/site-packages/*")
	case "doc":
		provideGlob = append(provideGlob, "man/man*/*", "man/*/man*/*", "info/*")
	}

	for _, glob := range provideGlob {
		matches, err := fs.Glob(sb, glob)
		if err != nil {
			return fmt.Errorf("while glob %s: %w", glob, err)
		}
		for _, match := range matches {
			if _, ok := provides[match]; ok {
				continue
			}
			// grab file stats
			st, err := sb.Lstat(match)
			if err == nil {
				if st.Mode().Type() == fs.ModeSymlink {
					// read symlink
					v, err := sb.Readlink(match)
					if err == nil {
						if strings.IndexByte(v, '/') == -1 {
							// only store info about symlinks in the same directory
							log.Printf("provides: %s %s -> ", st.Mode(), match, v)
							provides[match] = map[string]any{"symlink": v}
						}
					}
				} else {
					log.Printf("provides: %s %s", st.Mode(), match)
					provides[match] = map[string]any{"size": st.Size(), "mode": st.Mode()}
				}
			}
		}
	}

	metadata["provides"] = provides

	metadataJson, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if len(metadataJson) > 1024*1024 {
		return fmt.Errorf("metadata too long, should be below 1MB")
	}
	metadataHash := sha256.Sum256(metadataJson)

	jsonDebugOut := &bytes.Buffer{}
	json.Indent(jsonDebugOut, metadataJson, "", "\t")
	log.Printf("JSON data for this package:\n%s", jsonDebugOut.Bytes())

	metadataLen := len(metadataJson)
	signOffset := HEADER_LEN + metadataLen + len(hashtable)
	padding := 512 - (signOffset % 512)
	if padding < apkgsig.SignatureSize {
		padding += 512
	}
	signbuf := make([]byte, padding)
	dataOffset := signOffset + padding

	log.Printf("signature at %d, data at %d", signOffset, dataOffset)

	header := &bytes.Buffer{}
	header.Write([]byte("APKG"))
	binary.Write(header, binary.BigEndian, uint32(1)) // version
	binary.Write(header, binary.BigEndian, uint64(0)) // flags
	binary.Write(header, binary.BigEndian, uint64(created.Unix()))
	binary.Write(header, binary.BigEndian, uint64(created.Nanosecond()))
	binary.Write(header, binary.BigEndian, uint32(HEADER_LEN)) // MetaData offset int32
	binary.Write(header, binary.BigEndian, uint32(metadataLen))
	header.Write(metadataHash[:])
	binary.Write(header, binary.BigEndian, uint32(HEADER_LEN+metadataLen)) // Hash descriptor offset
	binary.Write(header, binary.BigEndian, uint32(len(hashtable)))
	header.Write(tableHash[:])
	binary.Write(header, binary.BigEndian, uint32(signOffset))
	binary.Write(header, binary.BigEndian, uint32(dataOffset))
	binary.Write(header, binary.BigEndian, uint32(blockSize))

	if header.Len() != HEADER_LEN {
		return errors.New("invalid header length")
	}

	// generate signature
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
	sig_blob, err := k.Sign(rand.Reader, header.Bytes(), crypto.Hash(0))
	if err != nil {
		return err
	}
	apkgsig.WriteVarblob(sigB, sig_blob)

	// verify signature
	_, err = apkgsig.VerifyPkg(header.Bytes(), bytes.NewReader(sigB.Bytes()))
	if err != nil {
		return err
	}

	if sigB.Len() > len(signbuf) {
		return errors.New("signature buffer not large enough!")
	}

	copy(signbuf, sigB.Bytes())

	headerHash := sha256.Sum256(header.Bytes())
	headerHashHex := hex.EncodeToString(headerHash[:])

	// generate output filename
	out := filepath.Join(os.Getenv("HOME"), "projects/apkg-tools/repo/apkg/dist/main", strings.Join(fn_a, "/"), filename_f+"-"+headerHashHex[:7]+".apkg")
	log.Printf("out filename = %s", out)

	err = os.MkdirAll(filepath.Dir(out), 0755)
	if err != nil {
		return err
	}

	outf, err := os.Create(out)
	if err != nil {
		return err
	}
	defer outf.Close()

	// write stuff
	_, err = outf.Write(header.Bytes())
	if err != nil {
		return err
	}
	_, err = outf.Write(metadataJson)
	if err != nil {
		return err
	}
	_, err = outf.Write(hashtable)
	if err != nil {
		return err
	}
	_, err = outf.Write(signbuf)
	if err != nil {
		return err
	}

	f.Seek(0, io.SeekStart)
	_, err = io.Copy(outf, f)
	if err != nil {
		return err
	}

	return nil
}
