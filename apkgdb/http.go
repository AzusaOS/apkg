package apkgdb

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sort"

	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/KarpelesLab/smartremote"
	bolt "go.etcd.io/bbolt"
)

func (d *DB) getPackagesList() []string {
	res := listUnsigned(d.osV, d.archV)

	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	d.dbptr.View(func(tx *bolt.Tx) error {
		// use p2p for correct package ordering
		bucket := tx.Bucket([]byte("p2p"))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(k, v []byte) error {
			res = append(res, string(v[8+32:]))
			return nil
		})
	})

	natSort(res)

	return res
}

func (d *DB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if sub := r.URL.Query().Get("sub"); d.parent == nil && sub != "" {
		// try to locate a sub database
		archos := ParseArchOS(sub)
		if !archos.IsValid() {
			http.Error(w, "Bad value for sub", http.StatusBadRequest)
			return
		}

		db, err := d.SubGet(archos)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		d = db
	}

	act := r.URL.Query().Get("action")

	switch act {
	case "list":
		// list unsigned first
		list := d.getPackagesList() // sorted
		for _, v := range list {
			fmt.Fprintf(w, "%s\n", v)
		}
	default:
		fmt.Fprintf(w, "APKGDB STATUS\n\n")
		fmt.Fprintf(w, "Name: %s\n", d.name)
		fmt.Fprintf(w, "OS: %s\n", d.os)
		fmt.Fprintf(w, "Arch: %s\n", d.arch)
		fmt.Fprintf(w, "Prefix: %s\n", d.prefix)
		fmt.Fprintf(w, "Version: %s\n", d.CurrentVersion())

		subs := d.ListSubs()
		if len(subs) > 0 {
			subs2 := make([]string, 0, len(subs))
			for _, sub := range subs {
				subs2 = append(subs2, sub.OS.String()+"."+sub.Arch.String())
			}
			sort.Strings(subs2)

			fmt.Fprintf(w, "Sub databases:\n")
			for _, sub := range subs2 {
				fmt.Fprintf(w, "  - %s\n", sub)
			}
		}
		//fmt.Fprintf(w, "Package count: %d\n", d.count)
	}
}

// http client (global)
var hClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: apkgsig.CACerts()},
	},
}

func init() {
	// make smartremote use our http client
	smartremote.DefaultDownloadManager.Client = hClient
	smartremote.DefaultDownloadManager.MaxDataJump = 16 * 1024 * 1024
}
