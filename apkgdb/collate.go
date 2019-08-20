package apkgdb

import "strings"

// collatedVersion returns collation version of version v for indexing
func collatedVersion(v string) (r []byte) {
	for len(v) > 0 {
		n := strings.IndexAny(v, "0123456789")
		if n == -1 {
			r = append(r, []byte(v)...)
			return
		}

		// append
		r = append(r, []byte(v[:n])...)
		v = v[n:]

		// calculate how many digits
		var i int
		for i = 1; i <= 32; i++ {
			if v[i] < '0' || v[i] > '9' {
				break
			}
		}

		// add digits len in string
		r = append(r, byte(i-1))
		r = append(r, []byte(v[:i])...)
		v = v[i:]
	}
	return
}
