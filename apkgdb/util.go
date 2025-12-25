package apkgdb

func bytesDup(v []byte) []byte {
	// simple function to duplicate a []byte because boltdb
	if len(v) == 0 {
		return nil
	}
	r := make([]byte, len(v))
	copy(r, v)
	return r
}
