package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestFormatReplayCommand_includesSubcommandAndPositional(t *testing.T) {
	var leaf *cli.Command
	root := &cli.Command{
		Name: "remotr",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "server-url"},
		},
		Commands: []*cli.Command{{
			Name: "endpoint",
			Commands: []*cli.Command{{
				Name: "show",
				Action: func(_ context.Context, c *cli.Command) error {
					leaf = c
					return nil
				},
			}},
		}},
	}
	if err := root.Run(t.Context(), []string{"remotr", "--server-url", "https://example:8443", "endpoint", "show"}); err != nil {
		t.Fatalf("setup run: %v", err)
	}
	if leaf == nil {
		t.Fatal("leaf command not captured")
	}

	got := formatReplayCommand(leaf, []string{"laptop-01"}, nil)
	want := "remotr --server-url https://example:8443 endpoint show laptop-01"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"simple", "simple"},
		{"internal/mycli", "internal/mycli"},
		{"has space", "'has space'"},
		{"", "''"},
	}
	for _, tc := range tests {
		if got := shellQuote(tc.in); got != tc.want {
			t.Fatalf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEndpointIDsFromPositionalArgs(t *testing.T) {
	var ids []string
	var ok bool
	root := &cli.Command{
		Name: "endpoint",
		Commands: []*cli.Command{{
			Name:  "label",
			Flags: []cli.Flag{endpointIDFlag()},
			Commands: []*cli.Command{{
				Name: "set",
				Action: func(_ context.Context, c *cli.Command) error {
					ids, ok = endpointIDsFromPositionalArgs(c)
					return nil
				},
			}},
		}},
	}
	if err := root.Run(t.Context(), []string{"endpoint", "label", "set", "laptop-a", "laptop-b", "site=berlin"}); err != nil {
		t.Fatal(err)
	}
	if !ok || len(ids) != 2 {
		t.Fatalf("got %v ok=%v", ids, ok)
	}
	if ids[0] != "laptop-a" || ids[1] != "laptop-b" {
		t.Fatalf("got %v", ids)
	}
}

func TestWriteReplayHintIfNeeded_stderr(t *testing.T) {
	replayReset()
	var leaf *cli.Command
	root := &cli.Command{
		Name: "remotr",
		Commands: []*cli.Command{{
			Name: "endpoint",
			Commands: []*cli.Command{{
				Name: "show",
				Action: func(_ context.Context, c *cli.Command) error {
					leaf = c
					replayActivate(c)
					replayAddPositional("laptop-01")
					return nil
				},
			}},
		}},
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	if err := root.Run(t.Context(), []string{"remotr", "endpoint", "show"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if leaf == nil {
		t.Fatal("leaf command not captured")
	}
	writeReplayHintIfNeeded()
	_ = w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "non-interactive:") {
		t.Fatalf("stderr = %q", out)
	}
	if !strings.Contains(out, "endpoint show laptop-01") {
		t.Fatalf("stderr = %q", out)
	}
}
