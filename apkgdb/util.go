package apkgdb

func bytesDup(v []byte) []byte {
	// simple function to duplicate a []byte because boltdb
	r := make([]byte, len(v))
	copy(r, v)
	return r
}
