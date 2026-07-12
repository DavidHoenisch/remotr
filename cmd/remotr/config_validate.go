package main

import (
	"context"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/urfave/cli/v3"
)

func actionConfigValidate(_ context.Context, c *cli.Command) error {
	dir := c.Args().First()
	if dir == "" {
		if root := findConfigRepoRoot("."); root != "" {
			dir = root
		} else {
			dir = "."
		}
	}
	if c.NArg() > 1 {
		return exitErr(2, "config validate: unexpected arguments")
	}

	res, err := configrepo.ValidateRepository(dir)
	if err != nil {
		return exitErr(1, "config validate: %v", err)
	}

	if !c.Bool("skip-render-check") {
		has, err := configcompose.HasManifests(dir)
		if err != nil {
			return exitErr(1, "config validate: %v", err)
		}
		if has {
			composeRes, err := configcompose.ValidateComposition(dir)
			if err != nil {
				return exitErr(1, "config validate: %v", err)
			}
			for _, issue := range composeRes.Issues {
				res.Issues = append(res.Issues, configrepo.ValidationIssue{
					Path:    issue.Path,
					Message: issue.Message,
				})
			}
		}
	}

	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(res); err != nil {
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
	for _, diagnostic := range res.Diagnostics {
		fmt.Printf("WARN %s [%s]: %s\n", diagnostic.Path, diagnostic.Code, diagnostic.Message)
	}
	if len(res.Issues) > 0 {
		return exitErr(1, "config validate: %d issue(s)", len(res.Issues))
	}
	fmt.Println("config validate: ok")
	return nil
}
