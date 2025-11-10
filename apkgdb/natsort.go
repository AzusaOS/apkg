package apkgdb

import (
	"sort"
	"strings"
)

type natsortSlice []string

func (s natsortSlice) Len() int {
	return len(s)
}

func (s natsortSlice) Less(a, b int) bool {
	return natsortCompare(s[a], s[b])
}

func (s natsortSlice) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

// Sort sorts a list of strings in a natural order
func natSort(l []string) {
	sort.Sort(natsortSlice(l))
}

// Compare returns true if the first string precedes the second one according to natural order
func natsortCompare(a, b string) bool {
	ln_a := len(a)
	ln_b := len(b)
	posa := 0
	posb := 0

	for {
		if ln_a <= posa {
			if ln_b <= posb {
				// eof on both at the same time (equal)
				return false
			}
			return true
		} else if ln_b <= posb {
			// eof on b
			return false
		}

		av, bv := a[posa], b[posb]
		digita := av >= '0' && av <= '9'
		digitb := bv >= '0' && bv <= '9'

		if digita && digitb {
			// go into numeric mode
			intlna := 1
			intlnb := 1
			for {
				if posa+intlna >= ln_a {
					break
				}
				x := a[posa+intlna]
				if x >= '0' && x <= '9' {
					intlna += 1
				} else {
					break
				}
				// if digits start with zeroes, ignore it
				if av == '0' {
					posa += 1
					intlna -= 1
					av = x
				}
			}
			for {
				if posb+intlnb >= ln_b {
					break
				}
				x := b[posb+intlnb]
				if x >= '0' && x <= '9' {
					intlnb += 1
				} else {
					break
				}
				// if digits start with zeroes, ignore it
				if bv == '0' {
					posb += 1
					intlnb -= 1
					bv = x
				}
			}
			if intlnb > intlna {
				// length of a value is longer, means it's a bigger number
				return true
			} else if intlna > intlnb {
				return false
			}
			// both have same length, let's compare as string
			v := strings.Compare(a[posa:posa+intlna], b[posb:posb+intlnb])
			if v < 0 {
				return true
			} else if v > 0 {
				return false
			}
			// equale
			posa += intlna
			posb += intlnb
			continue
		}

		if av == bv {
			posa += 1
			posb += 1
			continue
		}

		// digits come after (require for proper version comparison)
		if digitb {
			return true
		} else if digita {
			return false
		}

		return av < bv
	}
}
