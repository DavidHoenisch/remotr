package main

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/desktopparity"
	"github.com/urfave/cli/v3"
)

func TestDesktopParityCommandPathsIgnoreHiddenAliasesAndTerminalMechanics(t *testing.T) {
	action := func(context.Context, *cli.Command) error { return nil }
	root := &cli.Command{
		Name:                  "remotr",
		Aliases:               []string{"r"},
		EnableShellCompletion: true,
		Action:                action,
		Flags:                 []cli.Flag{&cli.BoolFlag{Name: "json"}},
		Commands: []*cli.Command{
			{Name: "endpoint", Commands: []*cli.Command{{Name: "list", Action: action}}},
			{Name: "endpoint-list", Hidden: true, Action: action},
		},
	}

	got := desktopParityCommandPaths(root)
	want := []string{"remotr", "remotr endpoint list"}
	if !slices.Equal(got, want) {
		t.Fatalf("desktopParityCommandPaths() = %q, want %q", got, want)
	}
}

func TestDesktopCLIParityInventoryMatchesCommandTree(t *testing.T) {
	inventory, err := desktopparity.Load("../../docs/reference/desktop-cli-parity.json")
	if err != nil {
		t.Fatalf("load desktop parity inventory: %v", err)
	}

	issues := desktopparity.Validate(desktopParityCommandPaths(newRootCommand()), inventory)
	if len(issues) != 0 {
		t.Fatalf("desktop parity drift:\n  %s", strings.Join(issues, "\n  "))
	}
}

func TestRemainingDeferredAuthorityWorkflowsRemainPlanned(t *testing.T) {
	inventory, err := desktopparity.Load("../../docs/reference/desktop-cli-parity.json")
	if err != nil {
		t.Fatalf("load desktop parity inventory: %v", err)
	}

	deferredTargets := map[string]string{
		"remotr admin credential stamp":  "parity-rbac-operators",
		"remotr config discover":         "parity-config-hub",
		"remotr config render":           "parity-config-hub",
		"remotr config validate":         "parity-config-hub",
		"remotr hub snippet import":      "parity-config-hub",
		"remotr init":                    "parity-config-hub",
		"remotr rbac operator list":      "parity-rbac-operators",
		"remotr rbac operator set-roles": "parity-rbac-operators",
		"remotr rbac role create":        "parity-rbac-operators",
		"remotr rbac role delete":        "parity-rbac-operators",
		"remotr rbac role list":          "parity-rbac-operators",
		"remotr rbac role show":          "parity-rbac-operators",
		"remotr rbac rule add":           "parity-rbac-operators",
		"remotr rbac rule remove":        "parity-rbac-operators",
	}

	entries := make(map[string]desktopparity.Entry, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		entries[entry.Command] = entry
	}
	for command, target := range deferredTargets {
		entry, ok := entries[command]
		if !ok {
			t.Errorf("remaining deferred workflow is unmapped: %s", command)
			continue
		}
		if entry.Status != "planned" {
			t.Errorf("%s status = %q, want planned", command, entry.Status)
		}
		if entry.TargetFeatureRelease != target {
			t.Errorf("%s target = %q, want %q", command, entry.TargetFeatureRelease, target)
		}
	}
}

func desktopParityCommandPaths(root *cli.Command) []string {
	var paths []string
	var visit func(*cli.Command, []string, bool)
	visit = func(command *cli.Command, prefix []string, hidden bool) {
		hidden = hidden || command.Hidden
		path := append(append([]string{}, prefix...), command.Name)
		if !hidden && command.Action != nil {
			paths = append(paths, strings.Join(path, " "))
		}
		for _, child := range command.Commands {
			visit(child, path, hidden)
		}
	}

	visit(root, nil, false)
	sort.Strings(paths)
	return paths
}
