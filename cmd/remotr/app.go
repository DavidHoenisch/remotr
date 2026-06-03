package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

func newApp() *cli.App {
	return &cli.App{
		Name:  "remotr",
		Usage: "operator CLI for Remotr (GitOps config + server registry)",
		Description: `Defaults load from ~/.config/remotr/config.yaml (override with --config or REMOTR_CONFIG).
Precedence: flags > environment > config file > built-in defaults.`,
		Flags: commonConfigFlags(),
		// Return exit errors to runApp instead of calling os.Exit during tests and Run().
		ExitErrHandler: func(*cli.Context, error) {},
		Commands: []*cli.Command{
			initCommand(),
			{
				Name:   "bootstrap",
				Usage:  "exchange one-time bootstrap token for operator credentials",
				Action: actionBootstrap,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "token", Usage: "one-time bootstrap token from server startup", Required: true},
				},
			},
			enrollCommand(),
			deploymentTopLevelCommand(),
			endpointCommand(),
			fleetCommand(),
			{
				Name:  "git",
				Usage: "trigger server configuration repository sync",
				Subcommands: []*cli.Command{
					{
						Name:   "sync",
						Usage:  "pull latest config from git remote",
						Action: actionGitSync,
					},
				},
			},
			configCommand(),
			{
				Name:    "version",
				Aliases: []string{"v"},
				Usage:   "print version",
				Action:  actionVersion,
			},
		},
	}
}

func deploymentTopLevelCommand() *cli.Command {
	return &cli.Command{
		Name:        "deployment",
		Usage:       "manage deployment tokens (alias for enroll deployment)",
		Category:    "enrollment",
		Subcommands: deploymentSubcommands(),
	}
}

func deploymentSubcommands() []*cli.Command {
	return []*cli.Command{
		deploymentCreateCommand(),
		deploymentListCommand(),
		deploymentShowCommand(),
		deploymentRevokeCommand(),
	}
}

func initCommand() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "scaffold a new configuration repository",
		ArgsUsage: "[directory]",
		Action:    actionInit,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "fleet", Value: "default", Usage: "initial fleet name (fleets/<fleet>/desired.yaml)"},
			&cli.StringFlag{Name: "policy", Value: "auto", Usage: "fleet remediation policy: auto or report"},
			&cli.BoolFlag{Name: "register-server", Usage: "register fleet in Postgres (REMOTR_DATABASE_URL or --database-url)"},
			&cli.StringFlag{Name: "database-url", Usage: "Postgres URL for --register-server (default: REMOTR_DATABASE_URL)"},
			&cli.BoolFlag{Name: "enroll", Usage: "with --register-server, create a one-time enrollment token"},
			&cli.DurationFlag{Name: "enroll-ttl", Value: defaultEnrollTTL, Usage: "enrollment token lifetime"},
			&cli.StringFlag{Name: "enroll-out", Usage: "write enrollment token to this file (mode 0600)"},
		},
	}
}

func enrollCommand() *cli.Command {
	return &cli.Command{
		Name:  "enroll",
		Usage: "create enrollment and deployment tokens",
		Subcommands: []*cli.Command{
			{
				Name:  "token",
				Usage: "one-time enrollment tokens",
				Subcommands: []*cli.Command{
					{
						Name:   "create",
						Usage:  "create a one-time enrollment token",
						Action: actionEnrollTokenCreate,
						Flags: []cli.Flag{
							&cli.DurationFlag{Name: "ttl", Value: defaultEnrollTTL, Usage: "token lifetime"},
							&cli.StringFlag{Name: "out", Usage: "write token to file (mode 0600)"},
						},
					},
				},
			},
			{
				Name:        "deployment",
				Usage:       "reusable deployment tokens",
				Subcommands: deploymentSubcommands(),
			},
		},
	}
}

func endpointCommand() *cli.Command {
	return &cli.Command{
		Name:  "endpoint",
		Usage: "list and manage enrolled endpoints",
		Subcommands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "list enrolled endpoints",
				Action: actionEndpointList,
				Flags:  []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
			},
			{
				Name:      "show",
				Usage:     "show endpoint details",
				ArgsUsage: "<endpoint-id>",
				Action:    actionEndpointShow,
				Flags:     []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
			},
			{
				Name:      "remove",
				Usage:     "unregister endpoint from server",
				ArgsUsage: "<endpoint-id>",
				Action:    actionEndpointRemove,
			},
			{
				Name:  "agent",
				Usage: "agent lifecycle on an endpoint",
				Subcommands: []*cli.Command{
					{
						Name:      "upgrade",
						Usage:     "request in-band agent upgrade on next sync",
						ArgsUsage: "<endpoint-id>",
						Action:    actionEndpointAgentUpgrade,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "version", Usage: "target remotr-agent release (e.g. v0.1.13)", Required: true},
						},
					},
				},
			},
		},
	}
}

func fleetCommand() *cli.Command {
	return &cli.Command{
		Name:  "fleet",
		Usage: "fleet-wide operations",
		Subcommands: []*cli.Command{
			{
				Name:  "agent",
				Usage: "agent lifecycle for a fleet",
				Subcommands: []*cli.Command{
					{
						Name:   "upgrade",
						Usage:  "request in-band agent upgrade for all endpoints in a fleet",
						Action: actionFleetAgentUpgrade,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "fleet", Usage: "fleet name", Required: true},
							&cli.StringFlag{Name: "version", Usage: "target remotr-agent release", Required: true},
						},
					},
				},
			},
		},
	}
}

func configCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "operator configuration and repository validation",
		Subcommands: []*cli.Command{
			{
				Name:   "show",
				Usage:  "print resolved operator settings as JSON",
				Action: actionConfigShow,
			},
			{
				Name:   "path",
				Usage:  "print default config file path",
				Action: actionConfigPath,
			},
			{
				Name:   "init",
				Usage:  "write operator config file",
				Action: actionConfigInit,
			},
			{
				Name:      "validate",
				Usage:     "validate configuration repository artifacts",
				ArgsUsage: "[directory]",
				Action:    actionConfigValidate,
				Flags:     []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "output JSON"}},
			},
		},
	}
}

func runApp() int {
	app := newApp()
	if err := app.Run(os.Args); err != nil {
		if ec, ok := err.(cli.ExitCoder); ok {
			if msg := ec.Error(); msg != "" {
				fmt.Fprintln(os.Stderr, msg)
			}
			return ec.ExitCode()
		}
		if strings.Contains(err.Error(), "Required flag") {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
