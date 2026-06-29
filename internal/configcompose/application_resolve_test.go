package configcompose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAppTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveApplicationModuleRef_directAndNested(t *testing.T) {
	dir := t.TempDir()
	writeAppTestFile(t, filepath.Join(dir, "applications", "slack.yaml"), "name: slack\n")
	writeAppTestFile(t, filepath.Join(dir, "applications", "pwa", "microsoft", "teams.yaml"), "name: teams\n")

	cases := []struct {
		ref  string
		want string
	}{
		{"applications/slack.yaml", "applications/slack.yaml"},
		{"applications/slack", "applications/slack.yaml"},
		{"pwa/microsoft/teams", "applications/pwa/microsoft/teams.yaml"},
		{"applications/pwa/microsoft/teams.yaml", "applications/pwa/microsoft/teams.yaml"},
		{"slack", "applications/slack.yaml"},
		{"teams", "applications/pwa/microsoft/teams.yaml"},
	}
	for _, tc := range cases {
		got, err := resolveApplicationModuleRef(dir, tc.ref)
		if err != nil {
			t.Fatalf("ref %q: %v", tc.ref, err)
		}
		if got != tc.want {
			t.Fatalf("ref %q: got %q want %q", tc.ref, got, tc.want)
		}
	}
}

func TestResolveApplicationModuleRef_ambiguous(t *testing.T) {
	dir := t.TempDir()
	writeAppTestFile(t, filepath.Join(dir, "applications", "a", "slack.yaml"), "name: slack\n")
	writeAppTestFile(t, filepath.Join(dir, "applications", "b", "slack.yaml"), "name: slack\n")

	_, err := resolveApplicationModuleRef(dir, "slack")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveApplicationModuleRef_skipsManifest(t *testing.T) {
	dir := t.TempDir()
	writeAppTestFile(t, filepath.Join(dir, "applications", "manifest.yaml"), "modules: []\n")

	_, err := resolveApplicationModuleRef(dir, "manifest")
	if err == nil {
		t.Fatal("expected not found")
	}
}
