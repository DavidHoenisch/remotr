package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func actionFleetList(ctx context.Context, c *cli.Command) error {
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
	var fleets []string
	err = runWithSpinner(ctx, c, "listing fleets", func(ctx context.Context) error {
		list, listErr := client.ListFleets()
		if listErr != nil {
			return listErr
		}
		fleets = list
		return nil
	})
	if err != nil {
		return apiErr(c, "fleet list", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(fleets)
	}

	if len(fleets) == 0 {
		writeInfoLine("no fleets configured")
		return nil
	}
	if resolveFormat(c) == formatTable && !c.Bool("no-headers") {
		fmt.Println("FLEET")
	}
	for _, fleet := range fleets {
		fmt.Println(fleet)
	}
	return nil
}

func actionFleetAgentUpgrade(_ context.Context, c *cli.Command) error {
	fleet, err := resolveFleet(c, "fleet agent upgrade")
	if err != nil {
		return err
	}
	ver, err := resolveVersion(c, "fleet agent upgrade")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "fleet agent upgrade: %v", err)
	}
	if settings.ServerURL == "" {
		return errServerURLMissing("fleet agent upgrade")
	}

	client, err := newAdminClient(settings)
	if err != nil {
		return exitErr(1, "fleet agent upgrade: %v", err)
	}
	n, err := client.RequestFleetAgentUpgrade(fleet, ver)
	if err != nil {
		return apiErr(c, "fleet agent upgrade", err)
	}
	fmt.Printf("upgrade requested for fleet %s to %s (%d endpoints)\n", fleet, ver, n)
	return nil
}
