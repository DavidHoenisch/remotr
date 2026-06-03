package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
)

func runConfigValidate(args []string) int {
	fs := flag.NewFlagSet("config validate", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "output JSON")
	extraJSON, flagArgs := peelJSONFlag(args)
	_ = fs.Parse(flagArgs)

	dir := "."
	if len(fs.Args()) > 0 {
		dir = fs.Args()[0]
	}
	if len(fs.Args()) > 1 {
		fmt.Fprintln(os.Stderr, "usage: remotr config validate [directory] [--json]")
		return 2
	}

	res, err := configrepo.ValidateRepository(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config validate: %v\n", err)
		return 1
	}

	if *asJSON || extraJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(os.Stderr, "config validate: %v\n", err)
			return 1
		}
		if len(res.Issues) > 0 {
			return 1
		}
		return 0
	}

	for _, ok := range res.OK {
		fmt.Printf("OK  %s\n", ok)
	}
	for _, issue := range res.Issues {
		fmt.Printf("ERR %s: %s\n", issue.Path, issue.Message)
	}
	if len(res.Issues) > 0 {
		fmt.Fprintf(os.Stderr, "config validate: %d issue(s)\n", len(res.Issues))
		return 1
	}
	fmt.Println("config validate: ok")
	return 0
}
