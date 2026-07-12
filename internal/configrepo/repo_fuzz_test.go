package configrepo

import (
	"crypto/sha256"
	"encoding/hex"
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
		validFleet := ValidateFleetName(fleet) == nil
		if validFleet {
			dir := filepath.Join(repo, "fleets", fleet)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "desired.yaml"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		artifact, digest, err := FleetArtifact(repo, fleet)
		if !validFleet {
			if err == nil {
				t.Fatal("expected error for invalid fleet name")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if string(artifact) != content {
			t.Fatalf("artifact = %q, want %q", artifact, content)
		}
		sum := sha256.Sum256([]byte(content))
		if digest != hex.EncodeToString(sum[:]) {
			t.Fatalf("digest = %q, want SHA-256 of content", digest)
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

func FuzzResolveArtifactPrefersEndpointOverride(f *testing.F) {
	f.Add("fleet artifact", "endpoint override", false)
	f.Add("fleet artifact", "endpoint override", true)

	f.Fuzz(func(t *testing.T, fleetArtifact, endpointArtifact string, hasOverride bool) {
		if len(fleetArtifact) > 1<<16 || len(endpointArtifact) > 1<<16 {
			return
		}
		repo := t.TempDir()
		const fleet = "demo"
		const endpointID = "11111111-1111-1111-1111-111111111111"
		fleetPath := filepath.Join(repo, "fleets", fleet, "desired.yaml")
		if err := os.MkdirAll(filepath.Dir(fleetPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fleetPath, []byte(fleetArtifact), 0o644); err != nil {
			t.Fatal(err)
		}
		if hasOverride {
			overridePath := filepath.Join(repo, "endpoints", endpointID, "desired.yaml")
			if err := os.MkdirAll(filepath.Dir(overridePath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(overridePath, []byte(endpointArtifact), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		got, _, err := ResolveArtifact(repo, fleet, endpointID)
		if err != nil {
			t.Fatal(err)
		}
		want := fleetArtifact
		if hasOverride {
			want = endpointArtifact
		}
		if string(got) != want {
			t.Fatalf("artifact = %q, want %q", got, want)
		}
	})
}
