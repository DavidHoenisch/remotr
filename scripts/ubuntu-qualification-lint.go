//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

func main() {
	path := flag.String("manifest", "test/qualification/ubuntu-2404-applicators.yaml", "Ubuntu qualification manifest")
	flag.Parse()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ubuntu qualification registry:", err)
		os.Exit(1)
	}
	if _, err := ubuntuqualification.Load(*path, registry); err != nil {
		fmt.Fprintln(os.Stderr, "Ubuntu qualification manifest:", err)
		os.Exit(1)
	}
}
