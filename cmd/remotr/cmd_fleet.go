package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

func actionFleetList(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "fleet list: %v", err)
	}
	if err := requireOperatorCLI(settings, "fleet list"); err != nil {
		return err
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "fleet list: %v", err)
	}
	fleets, err := client.ListFleets()
	if err != nil {
		return exitErr(1, "fleet list: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(fleets); err != nil {
			return exitErr(1, "fleet list: %v", err)
		}
		return nil
	}

	if len(fleets) == 0 {
		fmt.Println("no fleets configured")
		return nil
	}
	for _, fleet := range fleets {
		fmt.Println(fleet)
	}
	return nil
}

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

	client, err := newAdminClient(settings)
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
