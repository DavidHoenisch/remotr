//go:build ignore

// traceability-inventory emits canonical OpenSpec scenarios as JSON.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/traceability"
)

func main() {
	root := flag.String("root", "openspec", "OpenSpec root directory")
	flag.Parse()
	scenarios, err := traceability.Inventory(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "traceability inventory: %v\n", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(scenarios); err != nil {
		fmt.Fprintf(os.Stderr, "traceability inventory: encode output: %v\n", err)
		os.Exit(1)
	}
}
