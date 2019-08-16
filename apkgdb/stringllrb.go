package apkgdb

import (
	"strings"

	"github.com/petar/GoLLRB/llrb"
)

type llrbString struct {
	k string
	v *Package
}

func (s *llrbString) Less(than llrb.Item) bool {
	return strings.Compare(s.k, than.(*llrbString).k) < 0
}
