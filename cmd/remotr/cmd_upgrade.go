package main

import (
	"context"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/cliupgrade"
	"github.com/urfave/cli/v3"
)

func upgradeCommand() *cli.Command {
	return &cli.Command{
		Name:     "upgrade",
		Category: catSetup,
		Usage:    "upgrade the remotr CLI to the latest release",
		Description: withExamples(`Download and install the latest stable remotr release from GitHub.
Replaces the current binary (the running process keeps the old build until you invoke remotr again).`,
			"remotr upgrade",
			"remotr upgrade --check",
			"remotr upgrade --version v0.2.2"),
		Action: actionUpgrade,
		Flags: append(outputFlags(),
			&cli.BoolFlag{Name: "check", Usage: "report whether a newer release is available without installing"},
			&cli.StringFlag{Name: "version", Usage: "install a specific release tag (default: latest stable)"},
			&cli.StringFlag{Name: "install-path", Usage: "install destination (default: path of the running remotr binary)"},
			&cli.StringFlag{Name: "repo", Usage: "GitHub repository owner/name (default: DavidHoenisch/remotr)"},
			&cli.BoolFlag{Name: "force", Usage: "reinstall even when already on the target version"},
		),
	}
}

func actionUpgrade(ctx context.Context, c *cli.Command) error {
	var res cliupgrade.Result
	err := runWithSpinner(ctx, c, "upgrading remotr CLI", func(ctx context.Context) error {
		var runErr error
		res, runErr = cliupgrade.Run(ctx, cliupgrade.Options{
			CurrentVersion: version,
			TargetVersion:  c.String("version"),
			GitHubRepo:     c.String("repo"),
			InstallPath:    c.String("install-path"),
			Force:          c.Bool("force"),
			CheckOnly:      c.Bool("check"),
		})
		return runErr
	})
	if err != nil {
		return exitErr(1, "upgrade: %v", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(res)
	}

	current := version
	if current == "" {
		current = "dev"
	}
	if res.UpToDate {
		fmt.Printf("remotr %s is up to date", current)
		if res.Latest != "" && !c.Bool("check") {
			fmt.Printf(" (latest: %s)", res.Latest)
		}
		fmt.Println()
		return nil
	}

	if c.Bool("check") {
		fmt.Printf("update available: %s -> %s\n", current, res.Target)
		return nil
	}

	if res.Installed {
		writeOK(c, "upgraded remotr to %s", res.Target)
		writeInfo("       installed to %s\n", res.InstallPath)
		writeInfo("       run `remotr version` again to confirm the new build\n")
	}
	return nil
}
