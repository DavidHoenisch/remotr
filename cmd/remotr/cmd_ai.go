package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	aibundle "github.com/DavidHoenisch/remotr/ai"
	"github.com/DavidHoenisch/remotr/internal/aisetup"
	"github.com/urfave/cli/v3"
)

func aiCommand() *cli.Command {
	return &cli.Command{
		Name:     "ai",
		Category: catSetup,
		Usage:    "install AI agent skills for operating Remotr",
		Description: withExamples(`Install the remotr operator skill bundle for Claude Code, Cursor, Pi, and compatible agents.`,
			"remotr ai setup --agent claude",
			"remotr ai upgrade --agent pi",
			"remotr ai list"),
		Commands: []*cli.Command{
			{
				Name:  "setup",
				Usage: "install the bundled remotr AI skill for an agent",
				Description: withExamples("Copy the skill shipped with this remotr binary into the agent skills directory.",
					"remotr ai setup --agent claude",
					"remotr ai setup --agent pi --scope project"),
				Action: actionAISetup,
				Flags:  aiInstallFlags(),
			},
			{
				Name:  "upgrade",
				Usage: "download and install the latest remotr AI skill from GitHub",
				Description: withExamples("Fetch ai/remotr-agent from a GitHub release and install it.",
					"remotr ai upgrade --agent claude",
					"remotr ai upgrade --agent claude --version v0.2.2"),
				Action: actionAIUpgrade,
				Flags: append(aiInstallFlags(),
					&cli.StringFlag{Name: "version", Usage: "release tag to install (default: latest stable)"},
					&cli.StringFlag{Name: "repo", Usage: "GitHub owner/repo (default: DavidHoenisch/remotr)"},
				),
			},
			{
				Name:   "list",
				Usage:  "list supported AI agents and install status",
				Action: actionAIList,
				Flags: append(outputFlags(),
					&cli.StringFlag{Name: "scope", Value: "user", Usage: "install scope to check: user or project"},
				),
			},
		},
	}
}

func aiInstallFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "agent", Usage: "AI agent runtime: claude, cursor, or pi", Required: true},
		&cli.StringFlag{Name: "scope", Value: "user", Usage: "install scope: user (default) or project"},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing installation"},
	}
}

func actionAISetup(_ context.Context, c *cli.Command) error {
	target, err := resolveAITarget(c)
	if err != nil {
		return err
	}
	manifest, err := aisetup.Install(aisetup.InstallOptions{
		Target:        target,
		Source:        aibundle.RemotrAgent,
		SourceRoot:    aibundle.BundleRoot,
		SourceLabel:   "embedded",
		SourceVersion: version,
		Force:         c.Bool("force"),
	})
	if err != nil {
		return exitErr(1, "ai setup: %v", err)
	}
	return printAIInstallResult(c, manifest, "installed")
}

func actionAIUpgrade(ctx context.Context, c *cli.Command) error {
	target, err := resolveAITarget(c)
	if err != nil {
		return err
	}

	bundle, err := aisetup.FetchFromGitHub(ctx, aisetup.FetchOptions{
		Repo:    c.String("repo"),
		Version: c.String("version"),
	})
	if err != nil {
		return exitErr(1, "ai upgrade: %v", err)
	}
	defer func() { _ = bundle.Close() }()

	manifest, err := aisetup.Install(aisetup.InstallOptions{
		Target:        target,
		Source:        bundle.FS(),
		SourceRoot:    ".",
		SourceLabel:   "github",
		SourceVersion: bundle.Tag,
		Force:         true,
	})
	if err != nil {
		return exitErr(1, "ai upgrade: %v", err)
	}
	return printAIInstallResult(c, manifest, "upgraded")
}

func actionAIList(_ context.Context, c *cli.Command) error {
	scope, err := aisetup.ParseScope(c.String("scope"))
	if err != nil {
		return exitErr(2, "ai list: %v", err)
	}

	type row struct {
		Agent       string `json:"agent"`
		Scope       string `json:"scope"`
		Path        string `json:"path"`
		Installed   bool   `json:"installed"`
		Version     string `json:"bundle_version,omitempty"`
		Source      string `json:"source,omitempty"`
		InstalledAt string `json:"installed_at,omitempty"`
	}
	var rows []row
	for _, agent := range aisetup.SupportedAgents() {
		target, err := aisetup.ResolveTarget(agent, scope)
		if err != nil {
			return exitErr(1, "ai list: %v", err)
		}
		display, _ := aisetup.DefaultDisplayPath(agent, scope)
		item := row{
			Agent:     string(agent),
			Scope:     string(scope),
			Path:      display,
			Installed: false,
		}
		ok, err := target.Installed()
		if err != nil {
			return exitErr(1, "ai list: %v", err)
		}
		item.Installed = ok
		if ok {
			if manifest, err := aisetup.ReadManifest(target.InstallDir); err == nil {
				item.Version = manifest.BundleVersion
				item.Source = manifest.Source
				item.InstalledAt = manifest.InstalledAt
			}
		}
		rows = append(rows, item)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(rows)
	}

	for _, item := range rows {
		status := "not installed"
		if item.Installed {
			status = "installed"
			if item.Version != "" {
				status += " (bundle " + item.Version + ")"
			}
		}
		fmt.Printf("%-8s %-8s %s\n", item.Agent, status, item.Path)
	}
	return nil
}

func resolveAITarget(c *cli.Command) (aisetup.Target, error) {
	agent, err := aisetup.ParseAgent(c.String("agent"))
	if err != nil {
		return aisetup.Target{}, exitErr(2, "ai: %v", err)
	}
	scope, err := aisetup.ParseScope(c.String("scope"))
	if err != nil {
		return aisetup.Target{}, exitErr(2, "ai: %v", err)
	}
	return aisetup.ResolveTarget(agent, scope)
}

func printAIInstallResult(c *cli.Command, manifest aisetup.InstallManifest, verb string) error {
	if resolveFormat(c) == formatJSON {
		return encodeJSON(manifest)
	}
	display := manifest.InstallDir
	if home, err := os.UserHomeDir(); err == nil {
		display = strings.Replace(display, home, "~", 1)
	}
	writeOK(c, "%s remotr AI skill for %s", verb, manifest.Agent)
	writeInfo("       %s\n", display)
	if manifest.BundleVersion != "" {
		writeInfo("       bundle version: %s\n", manifest.BundleVersion)
	}
	if manifest.SourceVersion != "" {
		writeInfo("       source: %s (%s)\n", manifest.Source, manifest.SourceVersion)
	}
	return nil
}
