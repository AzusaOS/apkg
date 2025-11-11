package main

import (
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AzusaOS/apkg/apkgdb"
	"github.com/KarpelesLab/hsm"
)

type fileKey struct {
	arch string
	os   string
}

func processDb(name string, k hsm.Key) error {
	// instanciate db
	tempDir, err := os.MkdirTemp("", "apkgidx")
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
			if r := regexp.MustCompile(`.*\.([a-z]+)\.([a-z0-9]+)-([a-f0-9]{7})\.apkg$`).FindStringSubmatch(rpath); r != nil {
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
		p, err := apkgdb.OpenPackage(f)
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
