package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DavidHoenisch/remotr/internal/hubcatalog"
	"github.com/urfave/cli/v3"
)

func hubCommand() *cli.Command {
	return &cli.Command{
		Name:     "hub",
		Category: catConfig,
		Usage:    "Remotr Hub catalog utilities",
		Commands: []*cli.Command{
			{
				Name:  "snippet",
				Usage: "work with Hub configuration snippets",
				Commands: []*cli.Command{
					{
						Name:      "import",
						Usage:     "copy a Hub catalog snippet into a configuration repository module",
						ArgsUsage: "<entry-id>",
						Description: withExamples("",
							"remotr hub snippet import base-packages-debian-arch",
							"remotr hub snippet import ssh-hardening -o modules/sshd-hardening.yaml"),
						Action: actionHubSnippetImport,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "out", Aliases: []string{"o"}, Usage: "output path relative to config repo (default: modules/<entry-id>.yaml)"},
							&cli.StringFlag{Name: "hub-root", Usage: "path to hub/ directory (auto-detected when run from source tree)"},
							&cli.StringFlag{Name: "catalog", Usage: "path to catalog.json (default: <hub-root>/data/catalog.json)"},
							&cli.BoolFlag{Name: "json", Usage: "output result as JSON"},
						},
					},
				},
			},
		},
	}
}

func actionHubSnippetImport(ctx context.Context, c *cli.Command) error {
	if c.NArg() != 1 {
		return exitErr(2, "hub snippet import: requires exactly one entry id argument")
	}
	dir := "."
	if root := findConfigRepoRoot("."); root != "" {
		dir = root
	} else if root := findHubRoot("."); root != "" {
		dir = "."
	}

	res, err := hubcatalog.ImportSnippet(ctx, hubcatalog.ImportOptions{
		EntryID:     c.Args().First(),
		OutPath:     c.String("out"),
		RepoRoot:    dir,
		HubRoot:     c.String("hub-root"),
		CatalogPath: c.String("catalog"),
	})
	if err != nil {
		return exitErr(1, "hub snippet import: %v", err)
	}

	if c.Bool("json") {
		return encodeJSON(res)
	}
	fmt.Printf("imported %q -> %s\n", res.EntryID, res.OutPath)
	fmt.Printf("source: %s\n", res.SnippetSrc)
	return nil
}

func findHubRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "data", "catalog.json")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "snippets")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
