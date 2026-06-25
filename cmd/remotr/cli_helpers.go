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
	if first, ok := commandArgFirst(c); ok {
		return first, true
	}
	return "", false
}

func endpointIDFromFlagOrArg(c *cli.Command) (string, bool) {
	if v := strings.TrimSpace(c.String("endpoint")); v != "" {
		return v, true
	}
	if first, ok := commandArgFirst(c); ok {
		return first, true
	}
	return "", false
}

func endpointLabelArgs(c *cli.Command) []string {
	args := c.Args().Slice()
	if strings.TrimSpace(c.String("endpoint")) != "" {
		return args
	}
	if len(args) > 1 {
		return args[1:]
	}
	return nil
}

func commandArgFirst(c *cli.Command) (string, bool) {
	if c == nil {
		return "", false
	}
	args := c.Args()
	if args == nil || args.Len() < 1 {
		return "", false
	}
	return strings.TrimSpace(args.First()), true
}

func resolveFleet(c *cli.Command, cmd string) (string, error) {
	if fleet, ok := fleetFromFlagOrArg(c); ok {
		return fleet, nil
	}
	settings, err := resolveSettings(c)
	if err != nil {
		return "", exitErr(2, "%s: %v", cmd, err)
	}
	fleet := settings.Fleet
	if err := promptFleet(&fleet); err != nil {
		return "", exitErr(2, "%s: %v", cmd, err)
	}
	fleet = strings.TrimSpace(fleet)
	if fleet == "" {
		return "", errFleetMissing(cmd)
	}
	return fleet, nil
}

func resolveEndpointID(c *cli.Command, cmd string) (string, error) {
	ids, err := resolveEndpointSelection(c, cmd, false)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

func resolveEndpointIDs(c *cli.Command, cmd string) ([]string, error) {
	return resolveEndpointSelection(c, cmd, true)
}

func resolveEndpointSelection(c *cli.Command, cmd string, multi bool) ([]string, error) {
	if endpointID, ok := endpointIDFromFlagOrArg(c); ok {
		return []string{endpointID}, nil
	}
	if !isInteractive() {
		return nil, errEndpointMissing(cmd)
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return nil, exitErr(2, "%s: %v", cmd, err)
	}
	if settings.ServerURL != "" && opcreds.Present(settings.StateDir) {
		client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
		if err == nil {
			endpoints, err := client.ListEndpoints()
			if err == nil && len(endpoints) > 0 {
				if multi {
					var endpointIDs []string
					if err := promptEndpointMultiSelect(&endpointIDs, endpoints); err != nil {
						return nil, exitErr(2, "%s: %v", cmd, err)
					}
					if len(endpointIDs) == 0 {
						return nil, errEndpointMissing(cmd)
					}
					return endpointIDs, nil
				}
				var endpointID string
				if err := promptEndpointSelect(&endpointID, endpoints); err != nil {
					return nil, exitErr(2, "%s: %v", cmd, err)
				}
				endpointID = strings.TrimSpace(endpointID)
				if endpointID == "" {
					return nil, errEndpointMissing(cmd)
				}
				return []string{endpointID}, nil
			}
		}
	}

	var endpointID string
	if err := promptEndpointID(&endpointID); err != nil {
		return nil, exitErr(2, "%s: %v", cmd, err)
	}
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return nil, errEndpointMissing(cmd)
	}
	return []string{endpointID}, nil
}

func resolveLabel(c *cli.Command, cmd, flagName, promptTitle string) (string, error) {
	label := strings.TrimSpace(c.String(flagName))
	if label == "" {
		if v, ok := labelFromFlagOrArg("", c.Args().Slice()); ok {
			label = v
		}
	}
	if err := promptLabel(&label, promptTitle); err != nil {
		return "", exitErr(2, "%s: %v", cmd, err)
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return "", exitErr(2, "%s: label required (--label or positional)", cmd)
	}
	return label, nil
}

func resolveVersion(c *cli.Command, cmd string) (string, error) {
	ver := strings.TrimSpace(c.String("version"))
	if err := promptLabel(&ver, "Agent version"); err != nil {
		return "", exitErr(2, "%s: %v", cmd, err)
	}
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return "", exitErr(2, "%s: --version is required", cmd)
	}
	return ver, nil
}

func errAppPackageMissing(cmd string) error {
	return exitErr(2, "%s: NAME and VERSION required", cmd)
}

func resolveAppNameVersion(c *cli.Command, cmd string) (string, string, error) {
	if c.NArg() >= 2 {
		return c.Args().Get(0), c.Args().Get(1), nil
	}
	if c.NArg() == 1 {
		return "", "", exitErr(2, "%s: VERSION required (got NAME only)", cmd)
	}
	if !isInteractive() {
		return "", "", errAppPackageMissing(cmd)
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return "", "", exitErr(2, "%s: %v", cmd, err)
	}
	if settings.ServerURL != "" && opcreds.Present(settings.StateDir) {
		client, err := newAdminClient(settings)
		if err == nil {
			items, err := client.ListAppPackages("")
			if err == nil && len(items) > 0 {
				var selected string
				if err := promptAppPackageSelect(&selected, items); err != nil {
					return "", "", exitErr(2, "%s: %v", cmd, err)
				}
				name, version := parseAppPackagePickerValue(selected)
				name = strings.TrimSpace(name)
				version = strings.TrimSpace(version)
				if name == "" || version == "" {
					return "", "", errAppPackageMissing(cmd)
				}
				return name, version, nil
			}
		}
	}

	var name, version string
	if err := promptAppPackageNameVersion(&name, &version); err != nil {
		return "", "", exitErr(2, "%s: %v", cmd, err)
	}
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" || version == "" {
		return "", "", errAppPackageMissing(cmd)
	}
	return name, version, nil
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
