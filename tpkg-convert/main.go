package main

import (
	"flag"
	"log"
)

func main() {
	flag.Parse()

	for _, f := range flag.Args() {
		err := process(f)
		if err != nil {
			log.Printf("failed to process %s: %s", f, err)
		}
	}
}
