package apkgdb

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	"github.com/AzusaOS/apkg/apkgsig"
	"github.com/KarpelesLab/jwt"
)

func init() {
	// force usage of go resolver since using libc's one causes random crashes on ubuntu
	net.DefaultResolver.PreferGo = true
}

func (d *DB) download(v string) (bool, error) {
	resp, err := hClient.Get(d.prefix + "db/" + d.name + "/" + d.os + "/" + d.arch + "/LATEST.jwt")
	if err != nil {
		return false, err
	}

	token, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("failed to fetch information on latest database version: %s", resp.Status)
	}

	if err != nil {
		return false, err
	}

	token = bytes.TrimSpace(token)

	if string(token) == "NEW" {
		// special case, this is a new database
		return false, nil
	}

	dec, err := jwt.ParseString(string(token))
	if err != nil {
		return false, err
	}
	kid := dec.GetKeyId()
	kidName := apkgsig.DbKeyName(kid)
	if kidName == "" {
		return false, errors.New("unknown key used for jwt signature")
	}

	// decode ed25519 key
	tmpv, err := base64.RawURLEncoding.DecodeString(kid)
	if err != nil {
		return false, err
	}
	publicKey := ed25519.PublicKey(tmpv)

	err = dec.Verify(jwt.VerifyAlgo(jwt.EdDSA), jwt.VerifySignature(publicKey))
	if err != nil {
		return false, err
	}

	// jwt is valid
	version := dec.Payload().GetString("ver")
	if version == "" {
		return false, errors.New("invalid version in signed jwt")
	}

	//log.Printf("apkgdb: got database descriptor to version %s signed by %s", version, kidName)

	resp = nil

	if v != "" {
		if v == version {
			// no update needed
			return false, nil
		}

		// check for delta
		log.Printf("apkgdb: Downloading %s database delta to version %s ...", d.name, version)

		resp, err = hClient.Get(d.prefix + "db/" + d.name + "/" + d.os + "/" + d.arch + "/" + v + "-" + string(version) + ".bin")
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			log.Printf("apkgdb: Delta download failed with error %s, will download full database", resp.Status)
			// fallback to downloading the whole db
			resp = nil
		}
	}

	if resp == nil {
		log.Printf("apkgdb: Downloading %s database version %s ...", d.name, version)

		resp, err = hClient.Get(d.prefix + "db/" + d.name + "/" + d.os + "/" + d.arch + "/" + string(version) + ".bin")
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return false, fmt.Errorf("failed to fetch latest database: %s", resp.Status)
		}
	}

	out, err := ioutil.TempFile("", "apkg")
	if err != nil {
		return false, err
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		os.Remove(out.Name())

		return false, err
	}

	out.Seek(0, io.SeekStart)

	err = d.index(out)
	out.Close()
	os.Remove(out.Name())

	return true, err
}

func (d *DB) update() error {
	_, err := d.download(d.CurrentVersion())
	return err
}

func (d *DB) updateThread(updateReq bool) {
	if updateReq {
		// perform an update now
		err := d.update()
		if err != nil {
			log.Printf("apkgdb: update failed: %s", err)
		}
	}

	// keep running & check for updates
	t := time.NewTicker(1 * time.Hour)
	for {
		select {
		case <-t.C:
			err := d.update()
			if err != nil {

				log.Printf("apkgdb: update failed: %s", err)
			}
		case <-d.upd:
			err := d.update()
			if err != nil {
				log.Printf("apkgdb: update failed: %s", err)
			}
		}
	}
}

func (d *DB) Update() {
	select {
	case d.upd <- struct{}{}:
	default:
	}
}
