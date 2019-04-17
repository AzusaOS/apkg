package main

import (
	"encoding/hex"
	"flag"
	"log"

	"github.com/MagicalTux/hsm"
)

func main() {
	flag.Parse()
	h, err := hsm.New()
	var k hsm.Key

	if err != nil {
		log.Printf("failed to initialize HSM, signature won't be possible: %s", err)
	} else {
		ks, err := h.ListKeysByName("pkg_sign")
		if err != nil {
			log.Printf("failed to list HSM keys: %s", err)
		} else if len(ks) == 0 {
			log.Printf("failed to list HSM keys: no keys")
		} else {
			k = ks[0]
			log.Printf("found key: %s", k)
			blob, err := k.PublicBlob()
			if err == nil {
				log.Printf("Public key: %s", hex.EncodeToString(blob))
			}
		}
	}

	for _, f := range flag.Args() {
		err := process(k, f)
		if err != nil {
			log.Printf("failed to process %s: %s", f, err)
		}
	}
}
