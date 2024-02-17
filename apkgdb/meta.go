package apkgdb

import "io/fs"

type PackageMetaFile struct {
	Mode    fs.FileMode `json:"mode,omitempty"`
	Size    int64       `json:"size,omitempty"`
	Symlink string      `json:"symlink,omitempty"`
}

type PackageMeta struct {
	BaseName  string                       `json:"base_name"`
	FullName  string                       `json:"full_name"`
	Name      string                       `json:"name"`            // x11-libs.libdrm.libs
	Names     []string                     `json:"names,omitempty"` // name + version
	Version   string                       `json:"version"`         // 2.4.115
	Arch      string                       `json:"arch"`
	OS        string                       `json:"os"`
	BlockSize int64                        `json:"block_size"`
	Blocks    int                          `json:"blocks"`
	Category  string                       `json:"category"`
	Subcat    string                       `json:"subcat"`
	Hash      string                       `json:"hash"`
	Inodes    uint32                       `json:"inodes"`
	LDSO      []byte                       `json:"ld.so.cache,omitempty"` // optional ld.so.cache file, as base64
	Provides  map[string]*PackageMetaFile  `json:"provides"`
	Size      int64                        `json:"size"`
	Virtual   map[string]map[string]string `json:"virtual,omitempty"`
	Created   []int64                      `json:"created"`
}
