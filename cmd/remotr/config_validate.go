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

	if !c.Bool("skip-compose-check") {
		has, err := configcompose.HasManifests(dir)
		if err != nil {
			return exitErr(1, "config validate: %v", err)
		}
		if has {
			composeRes, err := configcompose.Compose(configcompose.Options{RepoRoot: dir, Check: true})
			if err != nil {
				return exitErr(1, "config validate: %v", err)
			}
			for _, stale := range composeRes.Stale {
				res.Issues = append(res.Issues, configrepo.ValidationIssue{
					Path:    stale,
					Message: "artifact is stale; run remotr config compose",
				})
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
	if len(res.Issues) > 0 {
		return exitErr(1, "config validate: %d issue(s)", len(res.Issues))
	}
	fmt.Println("config validate: ok")
	return nil
}
