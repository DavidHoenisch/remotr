package main

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func FuzzProfileServiceRejectsUnknownPersistedFields(f *testing.F) {
	for _, seed := range []string{"secret", "privateKey", "token", "certificate"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, fieldSeed string) {
		seedBytes := []byte(fieldSeed)
		if len(seedBytes) > 64 {
			seedBytes = seedBytes[:64]
		}
		unknownField := "unexpected_" + hex.EncodeToString(seedBytes)
		document := map[string]any{
			"profiles": []map[string]any{{
				"name":         "Production",
				"serverUrl":    "https://remotr.example:8443",
				"stateDir":     "/var/lib/remotr-operator",
				"caPath":       "",
				"defaultFleet": "production",
				unknownField:   "secret-canary",
			}},
		}
		raw, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("encode bounded settings fixture: %v", err)
		}

		settingsPath := filepath.Join(t.TempDir(), "profiles.json")
		if err := os.WriteFile(settingsPath, raw, 0o600); err != nil {
			t.Fatalf("write bounded settings fixture: %v", err)
		}
		profiles, err := NewProfileService(settingsPath, "").LoadProfiles()
		if err == nil {
			t.Fatalf("LoadProfiles() accepted unknown field %q and returned %#v", unknownField, profiles)
		}
	})
}
