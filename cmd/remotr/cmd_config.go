package main

import (
	"context"
	"fmt"
	"strings"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	"github.com/urfave/cli/v3"
)

func actionConfigShow(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "config show: %v", err)
	}
	if strings.EqualFold(strings.TrimSpace(c.String("format")), "plain") {
		fmt.Printf("server_url: %s\n", settings.ServerURL)
		fmt.Printf("state_dir: %s\n", settings.StateDir)
		fmt.Printf("ca: %s\n", settings.CA)
		fmt.Printf("fleet: %s\n", settings.Fleet)
		fmt.Printf("config_path: %s\n", settings.ConfigPath)
		return nil
	}
	return encodeJSON(settings)
}

func actionConfigPath(_ context.Context, _ *cli.Command) error {
	fmt.Println(opconfig.DefaultPath())
	return nil
}

func actionConfigInit(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "config init: %v", err)
	}
	if err := promptServerURL(&settings); err != nil {
		return exitErr(2, "config init: %v", err)
	}
	if settings.ServerURL == "" {
		return errServerURLMissing("config init")
	}
	if err := opconfig.Save(settings); err != nil {
		return exitErr(1, "config init: %v", err)
	}
	if c.Bool("json") {
		return encodeJSON(map[string]string{"path": opconfig.DefaultPath()})
	}
	fmt.Printf("wrote %s\n", opconfig.DefaultPath())
	return nil
}

func actionVersion(_ context.Context, _ *cli.Command) error {
	printVersionDetails()
	return nil
}
