package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApp_fleetListPrintsConfiguredFleets(t *testing.T) {
	dir := t.TempDir()
	fixturesDir := filepath.Join(dir, "fixtures")
	if err := os.Mkdir(fixturesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := struct {
		Status int      `json:"status"`
		Body   []string `json:"body"`
	}{
		Status: 200,
		Body:   []string{"engineering", "platform"},
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixturesDir, "GET_v1_admin_fleets.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(dir, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operator.crt", "operator.key", "ca.crt", "state.json"} {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("REMOTR_DEMO", "1")
	t.Setenv("REMOTR_DEMO_FIXTURES", fixturesDir)
	t.Setenv("REMOTR_SERVER_URL", "https://demo.remotr.example")
	t.Setenv("REMOTR_OPERATOR_STATE_DIR", stateDir)

	stdout := captureStdout(t, func() {
		app := newApp()
		if err := app.Run([]string{"remotr", "fleet", "list"}); err != nil {
			t.Fatalf("fleet list: %v", err)
		}
	})

	if strings.TrimSpace(stdout) != "engineering\nplatform" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
