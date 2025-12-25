package main

type pkgMeta struct {
	FullName string `json:"full_name"`
	Inodes   uint32 `json:"inodes"`
	Arch     string `json:"arch"`
	Os       string `json:"os"`
}
