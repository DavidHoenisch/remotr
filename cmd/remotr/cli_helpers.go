package main

import (
	"fmt"
	"os"
	"strings"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func newAdminClient(settings opconfig.Settings) (*admin.Client, error) {
	return admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
}

func requireOperatorCLI(settings opconfig.Settings, cmd string) bool {
	if settings.ServerURL == "" {
		fmt.Fprintf(os.Stderr, "%s: server URL is required (config, REMOTR_SERVER_URL, or --server-url)\n", cmd)
		return false
	}
	if !opcreds.Present(settings.StateDir) {
		fmt.Fprintf(os.Stderr, "%s: operator credentials missing in %s (run remotr bootstrap first)\n", cmd, settings.StateDir)
		return false
	}
	return true
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
