package main

import (
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/urfave/cli/v2"
)

func actionGitSync(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "git sync: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "git sync: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
	}
	if !opcreds.Present(settings.StateDir) {
		return exitErr(2, "git sync: operator credentials missing in %s (run remotr bootstrap first)", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "git sync: %v", err)
	}
	if err := client.TriggerGitSync(); err != nil {
		return exitErr(1, "git sync: %v", err)
	}
	fmt.Println("git sync ok")
	return nil
}
