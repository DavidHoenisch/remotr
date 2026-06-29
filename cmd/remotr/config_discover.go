package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/urfave/cli/v3"
)

func actionConfigDiscover(_ context.Context, c *cli.Command) error {
	dir := c.Args().First()
	if dir == "" {
		if root := findConfigRepoRoot("."); root != "" {
			dir = root
		} else {
			dir = "."
		}
	}
	if c.NArg() > 1 {
		return exitErr(2, "config discover: unexpected arguments")
	}

	fleet := strings.TrimSpace(c.String("fleet"))
	if fleet == "" {
		return exitErr(2, "config discover: --fleet is required")
	}

	summary, err := configcompose.DiscoverFleet(dir, fleet)
	if err != nil {
		return exitErr(1, "config discover: %v", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(summary)
	}

	fmt.Printf("Manifest: %s (kind: manifest)\n", summary.Manifest)
	fmt.Println("Modules:")
	for _, m := range summary.Modules {
		fmt.Printf("  - %s (kind: module)\n", m)
	}
	fmt.Println("Applications:")
	for _, a := range summary.Applications {
		fmt.Printf("  - %s (kind: application)\n", a)
	}
	fmt.Println("Crons:")
	for _, cr := range summary.Crons {
		fmt.Printf("  - %s (kind: crons)\n", cr)
	}
	return nil
}
