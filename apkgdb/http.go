package apkgdb

import (
	"fmt"
	"net/http"

	"github.com/petar/GoLLRB/llrb"
)

func (d *DBData) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	act := r.URL.Query().Get("action")

	switch act {
	case "list":
		// return list of packages
		d.nameIdx.AscendGreaterOrEqual(&llrbString{}, func(i llrb.Item) bool {
			cur := i.(*llrbString).k
			fmt.Fprintf(w, "%s\n", cur)
			return true
		})
	default:
		fmt.Fprintf(w, "TPKGDB STATUS\n\n")
		fmt.Fprintf(w, "Name: %s\n", d.name)
		fmt.Fprintf(w, "Prefix: %s\n", d.prefix)
		fmt.Fprintf(w, "Version: %d\n", d.version)
		fmt.Fprintf(w, "Created: %s\n", d.created)
		fmt.Fprintf(w, "OS/Arch: %d/%d\n", d.os, d.arch)
		fmt.Fprintf(w, "Package count: %d\n", d.count)
	}
}
