package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/urfave/cli/v3"
)

func actionConfigCompose(_ context.Context, c *cli.Command) error {
	dir := c.Args().First()
	if dir == "" {
		if root := findConfigRepoRoot("."); root != "" {
			dir = root
		} else {
			dir = "."
		}
	}
	if c.NArg() > 1 {
		return exitErr(2, "config compose: unexpected arguments")
	}
	var stdout string
	if c.Bool("print") {
		if c.IsSet("stdout") {
			return exitErr(2, "config compose: use only one of --print and --stdout")
		}
		stdout = "desired"
	} else {
		var err error
		stdout, err = resolveComposeStdout(c)
		if err != nil {
			return exitErr(2, "config compose: %v", err)
		}
	}
	if stdout != "" && (c.Bool("check") || c.Bool("dry-run")) {
		return exitErr(2, "config compose: --stdout is mutually exclusive with --check and --dry-run")
	}
	if c.Bool("check") && c.Bool("dry-run") {
		return exitErr(2, "config compose: --check and --dry-run are mutually exclusive")
	}

	res, err := configcompose.Compose(configcompose.Options{
		RepoRoot: dir,
		Fleet:    c.String("fleet"),
		Check:    c.Bool("check"),
		DryRun:   c.Bool("dry-run"),
		Stdout:   stdout,
	})
	if err != nil {
		return exitErr(1, "config compose: %v", err)
	}

	if stdout != "" {
		return printComposeStdout(c, res)
	}

	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(res); err != nil {
			return exitErr(1, "config compose: %v", err)
		}
		if len(res.Issues) > 0 || len(res.Stale) > 0 {
			return exitComposeFailure(res)
		}
		return nil
	}

	for _, path := range res.Written {
		fmt.Printf("WROTE %s\n", path)
	}
	if !c.Bool("dry-run") {
		for _, path := range res.OK {
			fmt.Printf("OK    %s\n", path)
		}
	}
	for _, diff := range res.Diffs {
		fmt.Printf("DIFF  %s\n", diff.Path)
		fmt.Println(diff.Text)
	}
	for _, path := range res.Stale {
		fmt.Printf("STALE %s\n", path)
	}
	for _, issue := range res.Issues {
		fmt.Printf("ERR   %s: %s\n", issue.Path, issue.Message)
	}
	if len(res.Issues) > 0 || len(res.Stale) > 0 {
		if c.Bool("dry-run") && len(res.Issues) == 0 {
			return exitErr(1, "config compose: dry-run — %d file(s) would change", len(res.Stale))
		}
		return exitComposeFailure(res)
	}
	switch {
	case c.Bool("dry-run"):
		fmt.Println("config compose: dry-run — no changes")
	case c.Bool("check"):
		fmt.Println("config compose: ok")
	case len(res.Written) == 0:
		fmt.Println("config compose: nothing to compose")
	default:
		fmt.Println("config compose: ok")
	}
	return nil
}

func resolveComposeStdout(c *cli.Command) (string, error) {
	if !c.IsSet("stdout") {
		return "", nil
	}
	mode := strings.TrimSpace(strings.ToLower(c.String("stdout")))
	if mode == "" {
		mode = "desired"
	}
	switch mode {
	case "desired", "crons", "all":
		return mode, nil
	default:
		return "", fmt.Errorf("--stdout must be desired, crons, or all")
	}
}

func printComposeStdout(c *cli.Command, res configcompose.Result) error {
	if resolveFormat(c) == formatJSON {
		if err := encodeJSON(res); err != nil {
			return exitErr(1, "config compose: %v", err)
		}
		if len(res.Issues) > 0 {
			return exitErr(1, "config compose: %d issue(s)", len(res.Issues))
		}
		return nil
	}
	for _, issue := range res.Issues {
		fmt.Fprintf(os.Stderr, "ERR   %s: %s\n", issue.Path, issue.Message)
	}
	if len(res.Issues) > 0 {
		return exitErr(1, "config compose: %d issue(s)", len(res.Issues))
	}
	for i, rendered := range res.Rendered {
		if len(res.Rendered) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("--- %s ---\n", rendered.Path)
		}
		fmt.Print(rendered.Content)
		if rendered.Content != "" && !strings.HasSuffix(rendered.Content, "\n") {
			fmt.Println()
		}
	}
	return nil
}

func exitComposeFailure(res configcompose.Result) error {
	switch {
	case len(res.Issues) > 0 && len(res.Stale) > 0:
		return exitErr(1, "config compose: %d issue(s), %d stale file(s)", len(res.Issues), len(res.Stale))
	case len(res.Issues) > 0:
		return exitErr(1, "config compose: %d issue(s)", len(res.Issues))
	default:
		return exitErr(1, "config compose: %d stale file(s)", len(res.Stale))
	}
}
