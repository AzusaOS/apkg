package main

import (
	"flag"
	"log"

	"github.com/MagicalTux/hsm"
)

func main() {
	flag.Parse()
	h, err := hsm.New()
	if err != nil {
		log.Printf("failed to initialize HSM, signature won't be possible: %s", err)
	} else {
		ks, err := h.ListKeysByName("pkg_sign")
		if err != nil {
			log.Printf("failed to list HSM keys: %s", err)
		} else {
			for _, k := range ks {
				log.Printf("found key: %s", k)
			}
		}
	}

	for _, f := range flag.Args() {
		err := process(h, f)
		if err != nil {
			log.Printf("failed to process %s: %s", f, err)
		}
	}
}
