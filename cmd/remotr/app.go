package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

func newRootCommand() *cli.Command {
	return &cli.Command{
		Name:                  "remotr",
		Usage:                 "operator CLI for Remotr (GitOps config + server registry)",
		Version:               version,
		Description:           rootDescription(),
		Suggest:               true,
		EnableShellCompletion: true,
		Action:                actionRootGettingStarted,
		Flags:                 commonConfigFlags(),
		ExitErrHandler:        func(_ context.Context, _ *cli.Command, _ error) {},
		Commands: []*cli.Command{
			doctorCommand(),
			upgradeCommand(),
			aiCommand(),
			initCommand(),
			bootstrapCommand(),
			enrollCommand(),
			deploymentTopLevelCommand(),
			endpointCommand(),
			diagnosticsCommand(),
			fleetCommand(),
			gitCommand(),
			logsCommand(),
			adminCommand(),
			rbacCommand(),
			configCommand(),
			hubCommand(),
			appCommand(),
			packageCommand(),
			versionCommand(),
		},
	}
}

func newApp() *cli.Command {
	return newRootCommand()
}

func rootDescription() string {
	return `Defaults load from ~/.config/remotr/config.yaml (override with --config or REMOTR_CONFIG).
Precedence: flags > environment > config file > built-in defaults.

Global flags (--server-url, --state-dir, etc.) may appear before or after the subcommand.

Exit codes: 0 success, 1 runtime/API error, 2 usage or configuration error, 4 compliance drift.

Run remotr doctor to diagnose setup. Global flags may appear before or after the subcommand.`
}

func deploymentTopLevelCommand() *cli.Command {
	return &cli.Command{
		Name:        "deployment",
		Usage:       "manage deployment tokens",
		Category:    catEnrollment,
		Description: "Canonical path for reusable deployment enrollment tokens.",
		Commands:    deploymentSubcommands(),
	}
}

func bootstrapCommand() *cli.Command {
	return &cli.Command{
		Name:     "bootstrap",
		Category: catSetup,
		Usage:    "exchange one-time bootstrap token for operator credentials",
		Description: withExamples(`Exchange the server's one-time bootstrap token for operator mTLS credentials.
Writes credentials to --state-dir and saves operator config.`,
			"remotr bootstrap --server-url https://remotr.example:8443 --ca /etc/remotr/ca.crt --token TOKEN",
			"remotr bootstrap --token TOKEN  # server-url and ca from config/env"),
		Action: actionBootstrap,
		Flags: append(tokenOutputFlags(),
			&cli.StringFlag{Name: "token", Usage: "one-time bootstrap token (use - to read from stdin)"},
		),
	}
}

func enrollCommand() *cli.Command {
	return &cli.Command{
		Name:     "enroll",
		Category: catEnrollment,
		Usage:    "create enrollment and deployment tokens",
		Commands: []*cli.Command{
			{
				Name:  "token",
				Usage: "one-time enrollment tokens",
				Commands: []*cli.Command{
					{
						Name:  "create",
						Usage: "create a one-time enrollment token",
						Description: withExamples(`Create a one-time enrollment token for a fleet.
Use --quiet with --out in scripts to avoid printing the secret to stdout.`,
							"remotr enroll token create --fleet engineering --ttl 168h",
							"remotr enroll token create --fleet engineering --out /secure/enroll.token --quiet"),
						Action: actionEnrollTokenCreate,
						Flags: append(tokenOutputFlags(),
							&cli.DurationFlag{Name: "ttl", Value: defaultEnrollTTL, Usage: "token lifetime"},
							&cli.StringFlag{Name: "out", Usage: "write token to file (mode 0600)"},
						),
					},
				},
			},
			{
				Name:        "deployment",
				Usage:       "reusable deployment tokens (alias for remotr deployment)",
				Hidden:      true,
				Commands:    deploymentSubcommands(),
			},
		},
	}
}

func endpointCommand() *cli.Command {
	return &cli.Command{
		Name:     "endpoint",
		Category: catInventory,
		Usage:    "list and manage enrolled endpoints",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "list enrolled endpoints",
				Description: withExamples("",
			"remotr endpoint list",
			"remotr endpoint list --json",
			"remotr endpoint list --format plain --no-headers"),
				Action: actionEndpointList,
				Flags:  outputFlags(),
			},
			{
				Name:      "show",
				Usage:     "show endpoint details",
				ArgsUsage: "[endpoint-id]",
				Description: withExamples("",
			"remotr endpoint show phalanx-acae925c",
			"remotr endpoint show --endpoint phalanx-acae925c --json"),
				Action: actionEndpointShow,
				Flags: append(outputFlags(),
					endpointIDFlag(),
				),
			},
			{
				Name:      "remove",
				Usage:     "unregister endpoint from server",
				ArgsUsage: "[endpoint-id]",
				Description: withExamples(`Permanently removes the endpoint from the server registry.
Does not uninstall the agent on the host. Requires --confirm matching the endpoint id.`,
					"remotr endpoint remove phalanx-acae925c --confirm phalanx-acae925c"),
				Action: actionEndpointRemove,
				Flags: []cli.Flag{
					endpointIDFlag(),
					confirmFlag("endpoint id"),
				},
			},
			{
				Name:  "agent",
				Usage: "agent lifecycle on an endpoint",
				Commands: []*cli.Command{
					{
						Name:      "upgrade",
						Usage:     "request in-band agent upgrade on next sync",
						ArgsUsage: "[endpoint-id]",
						Description: withExamples("",
			"remotr endpoint agent upgrade phalanx-acae925c --version v0.1.15"),
						Action: actionEndpointAgentUpgrade,
						Flags: []cli.Flag{
							endpointIDFlag(),
							&cli.StringFlag{Name: "version", Usage: "target remotr-agent release (e.g. v0.1.13)"},
						},
					},
				},
			},
			{
				Name:  "state",
				Usage: "compliance evidence for an endpoint",
				Commands: []*cli.Command{
					{
						Name:      "report",
						Usage:     "show the latest compliance report for an endpoint",
						ArgsUsage: "[endpoint-id]",
						Description: withExamples("",
			"remotr endpoint state report phalanx-acae925c",
			"remotr endpoint state report --endpoint phalanx-acae925c --json"),
						Action: actionEndpointStateReport,
						Flags: append(outputFlags(),
							endpointIDFlag(),
						),
					},
				},
			},
			{
				Name:  "cron",
				Usage: "scheduled job status for an endpoint",
				Commands: []*cli.Command{
					{
						Name:      "report",
						Usage:     "show cron execution status for an endpoint",
						ArgsUsage: "[endpoint-id]",
						Description: withExamples("",
			"remotr endpoint cron report phalanx-acae925c"),
						Action: actionEndpointCronReport,
						Flags: append(outputFlags(),
							endpointIDFlag(),
						),
					},
				},
			},
			endpointLabelCommand(),
		},
	}
}

func endpointLabelCommand() *cli.Command {
	return &cli.Command{
		Name:        "label",
		Usage:       "manage endpoint labels",
		Description: "Set, remove, or list arbitrary key=value labels stored on the server.",
		Commands: []*cli.Command{
			{
				Name:      "set",
				Usage:     "set or update a label on an endpoint",
				ArgsUsage: "[endpoint-id] key=value",
				Description: withExamples(`Labels are stored in the server database. Agent sync may overwrite keys the agent also reports.
When run interactively without --endpoint, choose one or more endpoints from a filterable list.
When key=value is omitted, prompts ask for the label key and value.`,
					"remotr endpoint label set phalanx-acae925c site=berlin",
					"remotr endpoint label set --endpoint phalanx-acae925c --key site --value berlin"),
				Action: actionEndpointLabelSet,
				Flags: []cli.Flag{
					endpointIDFlag(),
					&cli.StringFlag{Name: "key", Usage: "label key"},
					&cli.StringFlag{Name: "value", Usage: "label value"},
				},
			},
			{
				Name:      "unset",
				Usage:     "remove a label from an endpoint",
				ArgsUsage: "[endpoint-id] key",
				Description: withExamples(`When run interactively without --key, choose from the endpoint's existing labels.`,
					"remotr endpoint label unset phalanx-acae925c site",
					"remotr endpoint label unset --endpoint phalanx-acae925c --key site"),
				Action: actionEndpointLabelUnset,
				Flags: []cli.Flag{
					endpointIDFlag(),
					&cli.StringFlag{Name: "key", Usage: "label key"},
				},
			},
			{
				Name:      "list",
				Usage:     "list labels on an endpoint",
				ArgsUsage: "[endpoint-id]",
				Description: withExamples("",
					"remotr endpoint label list phalanx-acae925c",
					"remotr endpoint label list --endpoint phalanx-acae925c --json"),
				Action: actionEndpointLabelList,
				Flags: append(outputFlags(),
					endpointIDFlag(),
				),
			},
		},
	}
}

func fleetCommand() *cli.Command {
	return &cli.Command{
		Name:     "fleet",
		Category: catFleet,
		Usage:    "fleet-wide operations",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "list configured fleets",
				Description: withExamples("",
			"remotr fleet list",
			"remotr fleet list --json"),
				Action: actionFleetList,
				Flags:  outputFlags(),
			},
			{
				Name:  "agent",
				Usage: "agent lifecycle for a fleet",
				Commands: []*cli.Command{
					{
						Name:      "upgrade",
						Usage:     "request in-band agent upgrade for all endpoints in a fleet",
						ArgsUsage: "[fleet-name]",
						Description: withExamples("",
							"remotr fleet agent upgrade engineering --version v0.1.15",
							"remotr fleet agent upgrade --fleet engineering --version v0.1.15"),
						Action: actionFleetAgentUpgrade,
						Flags: []cli.Flag{
							fleetArgFlag(),
							&cli.StringFlag{Name: "version", Usage: "target remotr-agent release"},
						},
					},
				},
			},
			{
				Name:  "state",
				Usage: "compliance evidence for a fleet",
				Commands: []*cli.Command{
					{
						Name:      "report",
						Usage:     "show compliance reports for all endpoints in a fleet",
						ArgsUsage: "[fleet-name]",
						Description: withExamples("",
			"remotr fleet state report --fleet engineering",
			"remotr fleet state report --fleet engineering --verbose"),
						Action: actionFleetStateReport,
						Flags: append(outputFlags(),
							fleetArgFlag(),
							&cli.BoolFlag{Name: "verbose", Usage: "include full report for every endpoint"},
						),
					},
				},
			},
			{
				Name:  "cron",
				Usage: "scheduled job status for a fleet",
				Commands: []*cli.Command{
					{
						Name:      "report",
						Usage:     "show cron execution status for all endpoints in a fleet",
						ArgsUsage: "[fleet-name]",
						Description: withExamples("",
			"remotr fleet cron report --fleet engineering"),
						Action: actionFleetCronReport,
						Flags: append(outputFlags(),
							fleetArgFlag(),
							&cli.BoolFlag{Name: "verbose", Usage: "include full report for every endpoint"},
						),
					},
				},
			},
		},
	}
}

func gitCommand() *cli.Command {
	return &cli.Command{
		Name:     "git",
		Category: catGitOps,
		Usage:    "trigger server configuration repository sync",
		Commands: []*cli.Command{
			{
				Name:  "sync",
				Usage: "pull latest config from git remote",
				Description: withExamples("",
			"remotr git sync",
			"remotr git sync --json"),
				Action: actionGitSync,
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
				},
			},
		},
	}
}

func configCommand() *cli.Command {
	return &cli.Command{
		Name:     "config",
		Category: catConfig,
		Usage:    "operator configuration and repository validation",
		Commands: []*cli.Command{
			{
				Name:  "show",
				Usage: "print resolved operator settings as JSON",
				Description: withExamples("",
			"remotr config show"),
				Action: actionConfigShow,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "format", Value: "json", Usage: "output format: json, plain"},
				},
			},
			{
				Name:   "path",
				Usage:  "print default config file path",
				Action: actionConfigPath,
			},
			{
				Name:  "init",
				Usage: "write operator config file",
				Description: withExamples("",
			"remotr config init --server-url https://remotr.example:8443"),
				Action: actionConfigInit,
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
				},
			},
			{
				Name:      "compose",
				Usage:     "compose desired.yaml artifacts from manifest.yaml sources",
				ArgsUsage: "[directory]",
				Description: withExamples("",
					"remotr config compose ./remotr-config",
					"remotr config compose . --check",
					"remotr config compose . --fleet engineering",
					"remotr config compose . --fleet engineering --print",
					"remotr config compose . --fleet engineering --stdout crons"),
				Action: actionConfigCompose,
				Flags: append(outputFlags(),
					&cli.BoolFlag{Name: "check", Usage: "verify artifacts match manifests without writing"},
					&cli.BoolFlag{Name: "dry-run", Usage: "compare composed artifacts to disk and show diffs without writing (exit 1 when changes would be made)"},
					&cli.BoolFlag{Name: "print", Usage: "print composed desired.yaml to stdout (shorthand for --stdout desired; does not write files)"},
					&cli.StringFlag{Name: "stdout", Usage: "print composed artifact to stdout: desired, crons, or all (does not write files)"},
					&cli.StringFlag{Name: "fleet", Usage: "compose one fleet and endpoint manifests that extend it"},
				),
			},
			{
				Name:      "validate",
				Usage:     "validate configuration repository artifacts",
				ArgsUsage: "[directory]",
				Description: withExamples("",
			"remotr config validate ./remotr-config",
			"remotr config validate --json"),
				Action: actionConfigValidate,
				Flags: append(outputFlags(),
					&cli.BoolFlag{Name: "skip-compose-check", Usage: "do not verify composed artifacts are up to date"},
				),
			},
		},
	}
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "print version",
		Action:  actionVersion,
	}
}

func initCommand() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Category:  catSetup,
		Usage:     "scaffold a new configuration repository",
		ArgsUsage: "[directory]",
		Description: withExamples("",
			"remotr init -fleet engineering ./remotr-config",
			"remotr init --register-server --enroll --enroll-out /secure/enroll.token"),
		Action: actionInit,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "fleet", Value: "default", Usage: "initial fleet name (creates fleets/<fleet>/manifest.yaml and modules/)"},
			&cli.StringFlag{Name: "policy", Value: "auto", Usage: "fleet remediation policy: auto or report"},
			&cli.BoolFlag{Name: "register-server", Usage: "register fleet in Postgres (REMOTR_DATABASE_URL or --database-url)"},
			&cli.StringFlag{Name: "database-url", Usage: "Postgres URL for --register-server (default: REMOTR_DATABASE_URL)"},
			&cli.BoolFlag{Name: "enroll", Usage: "with --register-server, create a one-time enrollment token"},
			&cli.DurationFlag{Name: "enroll-ttl", Value: defaultEnrollTTL, Usage: "enrollment token lifetime"},
			&cli.StringFlag{Name: "enroll-out", Usage: "write enrollment token to this file (mode 0600)"},
			&cli.BoolFlag{Name: "quiet", Usage: "do not print enrollment token to stdout"},
			&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
		},
	}
}

func runApp() int {
	cmd := newRootCommand()
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		if e, ok := err.(*cliError); ok {
			fmt.Fprintln(os.Stderr, e.format(false))
			return e.ExitCode()
		}
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
