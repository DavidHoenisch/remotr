package main

import (
	"encoding/json"
	"fmt"
	"os"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	"github.com/urfave/cli/v2"
)

func actionConfigShow(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "config show: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return exitErr(1, "config show: %v", err)
	}
	return nil
}

func actionConfigPath(c *cli.Context) error {
	fmt.Println(opconfig.DefaultPath())
	return nil
}

func actionConfigInit(c *cli.Context) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "config init: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "config init: set server URL via --server-url or REMOTR_SERVER_URL")
	}
	if err := opconfig.Save(settings); err != nil {
		return exitErr(1, "config init: %v", err)
	}
	fmt.Printf("wrote %s\n", opconfig.DefaultPath())
	return nil
}

func actionVersion(c *cli.Context) error {
	if commit != "" {
		fmt.Printf("remotr %s (%s", version, commit)
		if date != "" {
			fmt.Printf(", %s", date)
		}
		fmt.Println(")")
		return nil
	}
	fmt.Printf("remotr %s\n", version)
	return nil
}
