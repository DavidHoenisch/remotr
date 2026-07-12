//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/providermatrix"
)

func main() {
	matrixPath := flag.String("matrix", "test/provider-matrix.yaml", "provider evidence matrix")
	claim := providermatrix.Claim{}
	flag.StringVar(&claim.Provider, "provider", "", "provider name")
	flag.StringVar(&claim.Distribution, "distribution", "", "distribution ID")
	flag.StringVar(&claim.Release, "release", "", "distribution release")
	flag.StringVar(&claim.Architecture, "architecture", "", "architecture")
	flag.StringVar(&claim.Backend, "backend", "", "provider backend")
	flag.StringVar(&claim.ContractRevision, "contract-revision", "", "provider contract revision")
	flag.StringVar(&claim.Environment, "environment", "", "container or vm")
	flag.Parse()

	if claim.Provider == "" || claim.Distribution == "" || claim.Release == "" || claim.Architecture == "" || claim.Backend == "" || claim.ContractRevision == "" || claim.Environment == "" {
		fmt.Fprintln(os.Stderr, "usage: specify provider, distribution, release, architecture, backend, contract-revision, and environment")
		os.Exit(2)
	}
	matrix, err := providermatrix.Load(*matrixPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "provider advertisement gate:", err)
		os.Exit(1)
	}
	if !providermatrix.Advertised(matrix, claim) {
		fmt.Fprintln(os.Stderr, "provider advertisement gate: no matching passing provider-matrix evidence")
		os.Exit(1)
	}
}
