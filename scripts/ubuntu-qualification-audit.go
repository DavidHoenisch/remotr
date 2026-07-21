//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

func main() {
	repositoryRoot := flag.String("root", ".", "repository root")
	flag.Parse()

	report, err := ubuntuqualification.LoadRepositoryAudit(*repositoryRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ubuntu qualification audit:", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "Ubuntu qualification audit:", err)
		os.Exit(1)
	}
	if !report.Umbrella.Eligible {
		os.Exit(1)
	}
}
