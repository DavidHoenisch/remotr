package configrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzFleetArtifact(f *testing.F) {
	f.Add("test-fleet", "configurations: []\n")
	f.Add("demo", "x")

	f.Fuzz(func(t *testing.T, fleet, content string) {
		if len(fleet) > 128 || len(content) > 1<<16 {
			return
		}
		if strings.Contains(fleet, "\x00") {
			return
		}

		repo := t.TempDir()
		if fleet != "" && !strings.Contains(fleet, "..") && !strings.ContainsRune(fleet, filepath.Separator) {
			dir := filepath.Join(repo, "fleets", fleet)
			if err := os.MkdirAll(dir, 0o755); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "desired.yaml"), []byte(content), 0o644)
			}
		}

		_, _, err := FleetArtifact(repo, fleet)
		if fleet == "" || strings.Contains(fleet, "..") || strings.ContainsRune(fleet, filepath.Separator) {
			if err == nil {
				t.Fatal("expected error for invalid fleet name")
			}
		}
	})
}

func FuzzFleetArtifactPathTraversal(f *testing.F) {
	f.Add("../escape")
	f.Add("..")
	f.Add(`a/../b`)

	f.Fuzz(func(t *testing.T, fleet string) {
		if len(fleet) > 256 {
			return
		}
		repo := t.TempDir()
		_, _, err := FleetArtifact(repo, fleet)
		if strings.Contains(fleet, "..") || strings.ContainsRune(fleet, filepath.Separator) || fleet == "" {
			if err == nil {
				t.Fatalf("fleet %q should be rejected", fleet)
			}
		}
	})
}
