//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/providermatrix"
)

func main() {
	path := flag.String("matrix", "test/provider-matrix.yaml", "provider evidence matrix")
	flag.Parse()
	if _, err := providermatrix.Load(*path); err != nil {
		fmt.Fprintln(os.Stderr, "provider matrix:", err)
		os.Exit(1)
	}
}
