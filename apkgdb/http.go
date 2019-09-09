package apkgdb

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"git.atonline.com/azusa/apkg/apkgsig"
	"github.com/boltdb/bolt"
)

func (d *DB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	act := r.URL.Query().Get("action")

	switch act {
	case "list":
		// return list of packages
		d.db.View(func(tx *bolt.Tx) error {
			// use p2i for correct package ordering
			return tx.Bucket([]byte("p2i")).ForEach(func(k, v []byte) error {
				_, err := fmt.Fprintf(w, "%s\n", v[32+8:])
				return err
			})
		})
	default:
		fmt.Fprintf(w, "APKGDB STATUS\n\n")
		fmt.Fprintf(w, "Name: %s\n", d.name)
		fmt.Fprintf(w, "Prefix: %s\n", d.prefix)
		fmt.Fprintf(w, "Version: %d\n", d.CurrentVersion())
		//fmt.Fprintf(w, "Package count: %d\n", d.count)
	}
}

// http client (global)
var hClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: apkgsig.CACerts()},
	},
}
