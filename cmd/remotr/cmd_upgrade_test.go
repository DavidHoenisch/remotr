package main

import (
	"context"
	"strings"
	"testing"
)

func TestApp_upgradeHelp(t *testing.T) {
	app := newApp()
	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"remotr", "upgrade", "--help"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "--check") || !strings.Contains(out, "--version") {
		t.Fatalf("help = %q", out)
	}
}
