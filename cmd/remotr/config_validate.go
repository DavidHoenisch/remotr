package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/urfave/cli/v2"
)

func actionConfigValidate(c *cli.Context) error {
	dir := c.Args().First()
	if dir == "" {
		dir = "."
	}
	if c.NArg() > 1 {
		return exitErr(2, "config validate: unexpected arguments")
	}

	res, err := configrepo.ValidateRepository(dir)
	if err != nil {
		return exitErr(1, "config validate: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			return exitErr(1, "config validate: %v", err)
		}
		if len(res.Issues) > 0 {
			return exitErr(1, "config validate: %d issue(s)", len(res.Issues))
		}
		return nil
	}

	for _, ok := range res.OK {
		fmt.Printf("OK  %s\n", ok)
	}
	for _, issue := range res.Issues {
		fmt.Printf("ERR %s: %s\n", issue.Path, issue.Message)
	}
	if len(res.Issues) > 0 {
		return exitErr(1, "config validate: %d issue(s)", len(res.Issues))
	}
	fmt.Println("config validate: ok")
	return nil
}
