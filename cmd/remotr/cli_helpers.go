package main

import (
	"fmt"
	"os"
	"strings"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/urfave/cli/v3"
)

func newAdminClient(settings opconfig.Settings) (*admin.Client, error) {
	return admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
}

func requireOperatorCLI(settings opconfig.Settings, cmd string) error {
	if settings.ServerURL == "" {
		return errServerURLMissing(cmd)
	}
	if !opcreds.Present(settings.StateDir) {
		return errCredentialsMissing(cmd, settings.StateDir)
	}
	return nil
}

func writeTokenOut(path, token string) error {
	if path == "" {
		return nil
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

func labelFromFlagOrArg(labelFlag string, args []string) (string, bool) {
	if v := strings.TrimSpace(labelFlag); v != "" {
		return v, true
	}
	if len(args) == 1 {
		return strings.TrimSpace(args[0]), true
	}
	return "", false
}

func fleetFromFlagOrArg(c *cli.Command) (string, bool) {
	if v := strings.TrimSpace(c.String("fleet")); v != "" {
		return v, true
	}
	if c.NArg() >= 1 {
		return strings.TrimSpace(c.Args().First()), true
	}
	return "", false
}

func endpointIDFromFlagOrArg(c *cli.Command) (string, bool) {
	if v := strings.TrimSpace(c.String("endpoint")); v != "" {
		return v, true
	}
	if c.NArg() >= 1 {
		return strings.TrimSpace(c.Args().First()), true
	}
	return "", false
}

func requireConfirm(c *cli.Command, cmd, resourceID string) error {
	confirm := strings.TrimSpace(c.String("confirm"))
	if confirm == "" {
		if isInteractive() {
			ok, err := promptConfirmResource(resourceID)
			if err != nil {
				return exitErr(2, "%s: %v", cmd, err)
			}
			if !ok {
				return exitErr(2, "%s: cancelled", cmd)
			}
			return nil
		}
		return errConfirmRequired(cmd, resourceID)
	}
	if confirm != resourceID {
		return exitErr(2, "%s: --confirm must match %q", cmd, resourceID)
	}
	return nil
}

func endpointIDFlag() cli.Flag {
	return &cli.StringFlag{Name: "endpoint", Usage: "endpoint id (alternative to positional argument)"}
}

func fleetArgFlag() cli.Flag {
	return &cli.StringFlag{Name: "fleet", Usage: "fleet name (alternative to positional argument)"}
}

func confirmFlag(resource string) cli.Flag {
	return &cli.StringFlag{Name: "confirm", Usage: "confirm destructive action (must match " + resource + ")"}
}

func tokenOutputFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "quiet", Usage: "do not print secret token to stdout (use with --out; default when stdout is not a TTY)"},
		&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
	}
}
