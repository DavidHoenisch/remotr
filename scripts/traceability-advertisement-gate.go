//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/traceability"
)

func main() {
	change := flag.String("change", "", "source OpenSpec change")
	capability := flag.String("capability", "", "source OpenSpec capability")
	manifestPath := flag.String("manifest", "test/traceability.yaml", "traceability manifest")
	flag.Parse()
	if *change == "" || *capability == "" {
		fmt.Fprintln(os.Stderr, "usage: -change and -capability are required")
		os.Exit(2)
	}
	manifest, err := traceability.LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "advertisement gate:", err)
		os.Exit(1)
	}
	issues := traceability.AdvertisementIssues(manifest, *change, *capability, runSelector)
	for _, issue := range issues {
		fmt.Fprintln(os.Stderr, issue)
	}
	if len(issues) > 0 {
		os.Exit(1)
	}
}

func runSelector(selector string) error {
	parts := strings.SplitN(strings.TrimPrefix(selector, "go-test:"), ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("selector runner unavailable")
	}
	output, err := exec.Command("go", "test", "-mod=vendor", parts[0], "-run", parts[1], "-count=1").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
