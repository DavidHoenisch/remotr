package main

import (
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v2"
)

func actionFleetAgentUpgrade(c *cli.Context) error {
	fleet := strings.TrimSpace(c.String("fleet"))
	ver := strings.TrimSpace(c.String("version"))
	if fleet == "" {
		return exitErr(2, "fleet agent upgrade: --fleet is required")
	}
	if ver == "" {
		return exitErr(2, "fleet agent upgrade: --version is required")
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "fleet agent upgrade: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "fleet agent upgrade: server URL is required")
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "fleet agent upgrade: %v", err)
	}
	n, err := client.RequestFleetAgentUpgrade(fleet, ver)
	if err != nil {
		return exitErr(1, "fleet agent upgrade: %v", err)
	}
	fmt.Printf("upgrade requested for fleet %s to %s (%d endpoints)\n", fleet, ver, n)
	return nil
}
