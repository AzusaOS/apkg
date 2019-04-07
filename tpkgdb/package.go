package tpkgdb

import "github.com/google/uuid"

type Package struct {
	startIno uint64
	pos      int64 // position in archive of data
	id       uuid.UUID
	hash     []byte // typically sha256
	size     uint64
	inodes   uint64
	name     string
	path     string // path relative to where db file was downloaded from
}
