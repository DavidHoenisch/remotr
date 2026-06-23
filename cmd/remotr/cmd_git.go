package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/urfave/cli/v3"
)

func actionGitSync(ctx context.Context, c *cli.Command) error {
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
	err = runWithSpinner(ctx, c, "syncing configuration repository", func(ctx context.Context) error {
		return client.TriggerGitSync()
	})
	if err != nil {
		return apiErr(c, "git sync", err)
	}
	if c.Bool("json") {
		return encodeJSON(map[string]string{"status": "ok"})
	}
	fmt.Println("git sync ok")
	return nil
}
