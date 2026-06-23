package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestLabelFromFlagOrArg(t *testing.T) {
	if got, ok := labelFromFlagOrArg("from-flag", []string{"positional"}); !ok || got != "from-flag" {
		t.Fatalf("flag precedence: got %q ok=%v", got, ok)
	}
	if got, ok := labelFromFlagOrArg("", []string{"positional"}); !ok || got != "positional" {
		t.Fatalf("positional: got %q ok=%v", got, ok)
	}
	if _, ok := labelFromFlagOrArg("", nil); ok {
		t.Fatal("expected missing label")
	}
}

func TestResolveFleetFromFlag(t *testing.T) {
	cmd := &cli.Command{
		Flags: []cli.Flag{fleetArgFlag()},
	}
	if err := cmd.Set("fleet", "engineering"); err != nil {
		t.Fatal(err)
	}
	fleet, err := resolveFleet(cmd, "test")
	if err != nil {
		t.Fatal(err)
	}
	if fleet != "engineering" {
		t.Fatalf("got %q", fleet)
	}
}

func TestResolveFleetFromConfig(t *testing.T) {
	t.Setenv("REMOTR_FLEET", "")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("fleet: production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := &cli.Command{
		Flags: []cli.Flag{
			fleetArgFlag(),
			&cli.StringFlag{Name: "config", Value: cfgPath},
		},
	}
	fleet, err := resolveFleet(cmd, "test")
	if err != nil {
		t.Fatal(err)
	}
	if fleet != "production" {
		t.Fatalf("got %q", fleet)
	}
}

func TestResolveEndpointFromFlag(t *testing.T) {
	cmd := &cli.Command{
		Flags: []cli.Flag{endpointIDFlag()},
	}
	if err := cmd.Set("endpoint", "laptop-01"); err != nil {
		t.Fatal(err)
	}
	endpointID, err := resolveEndpointID(cmd, "test")
	if err != nil {
		t.Fatal(err)
	}
	if endpointID != "laptop-01" {
		t.Fatalf("got %q", endpointID)
	}
}
