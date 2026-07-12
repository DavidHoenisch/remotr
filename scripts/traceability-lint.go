//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/traceability"
)

func main() {
	openspecRoot := flag.String("openspec", "openspec", "OpenSpec root")
	registry := flag.String("registry", "test/traceability-prefixes.yaml", "prefix registry")
	manifest := flag.String("manifest", "test/traceability.yaml", "traceability manifest")
	flag.Parse()
	issues, err := traceability.Lint(*openspecRoot, *registry, *manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traceability lint:", err)
		os.Exit(1)
	}
	loaded, err := traceability.LoadManifest(*manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traceability lint:", err)
		os.Exit(1)
	}
	featureIssues, err := traceability.LintGodogFeatures("test/acceptance/features", loaded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traceability lint:", err)
		os.Exit(1)
	}
	issues = append(issues, featureIssues...)
	for _, issue := range issues {
		fmt.Fprintln(os.Stderr, issue)
	}
	if len(issues) > 0 {
		os.Exit(1)
	}
}
