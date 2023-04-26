package main

import (
	"encoding/base64"
	"flag"
	"log"
	"os"

	"github.com/KarpelesLab/hsm"
)

func main() {
	flag.Parse()
	h, err := hsm.New()

	if err != nil {
		log.Printf("failed to initialize HSM: %s", err)
		os.Exit(1)
	}

	ks, err := h.ListKeysByName("pkg_sign_ed25519")
	if err != nil {
		log.Printf("failed to list HSM keys: %s", err)
		os.Exit(1)
	} else if len(ks) == 0 {
		log.Printf("failed to list HSM keys: no keys. Please generate one.")
		os.Exit(1)
	}

	k := ks[0]
	log.Printf("found key: %s", k)
	blob, err := k.PublicBlob()
	if err == nil {
		log.Printf("Public key: %s", base64.RawURLEncoding.EncodeToString(blob))
	}

	for _, f := range flag.Args() {
		err := process(k, f)
		if err != nil {
			log.Printf("failed to process %s: %s", f, err)
		}
	}
}
