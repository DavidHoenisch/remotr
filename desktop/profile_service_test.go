package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProfileServiceImportsStandardOperatorConfiguration(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "operator-state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatalf("create operator state directory: %v", err)
	}
	caPath := filepath.Join(stateDir, "ca.crt")
	if err := os.WriteFile(caPath, []byte("operator-ca-canary"), 0o600); err != nil {
		t.Fatalf("write operator CA fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "operator.key"), []byte("operator-private-key-canary"), 0o600); err != nil {
		t.Fatalf("write operator key fixture: %v", err)
	}

	operatorConfigPath := filepath.Join(root, "config.yaml")
	operatorConfig := fmt.Sprintf(
		"server_url: https://remotr.example:8443\nstate_dir: %s\nca: %s\nfleet: production\n",
		stateDir,
		caPath,
	)
	if err := os.WriteFile(operatorConfigPath, []byte(operatorConfig), 0o600); err != nil {
		t.Fatalf("write standard operator config: %v", err)
	}
	settingsPath := filepath.Join(root, "desktop", "profiles.json")
	service := NewProfileService(settingsPath, operatorConfigPath)

	profiles, err := service.LoadProfiles()
	if err != nil {
		t.Fatalf("load profiles: %v", err)
	}
	want := []ConnectionProfile{{
		Name:         "Default",
		ServerURL:    "https://remotr.example:8443",
		StateDir:     stateDir,
		CAPath:       caPath,
		DefaultFleet: "production",
	}}
	if !slices.Equal(profiles, want) {
		t.Fatalf("profiles = %#v, want %#v", profiles, want)
	}
	if _, err := os.Stat(settingsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("implicit Default profile created desktop settings: %v", err)
	}
}

func TestProfileServicePersistsOnlyAllowlistedReferences(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "desktop", "profiles.json")
	stateDir := filepath.Join(root, "operator-state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatalf("create operator state directory: %v", err)
	}
	caPath := filepath.Join(stateDir, "ca.crt")
	canaries := []string{
		"operator-certificate-canary",
		"operator-private-key-canary",
		"bootstrap-token-canary",
	}
	for index, fixture := range []struct {
		name    string
		content string
	}{
		{name: "operator.crt", content: canaries[0]},
		{name: "operator.key", content: canaries[1]},
		{name: "bootstrap.token", content: canaries[2]},
	} {
		if err := os.WriteFile(filepath.Join(stateDir, fixture.name), []byte(fixture.content), 0o600); err != nil {
			t.Fatalf("write secret fixture %d: %v", index, err)
		}
	}
	if err := os.WriteFile(caPath, []byte("operator-ca-canary"), 0o600); err != nil {
		t.Fatalf("write CA fixture: %v", err)
	}

	service := NewProfileService(settingsPath, filepath.Join(root, "missing-operator-config.yaml"))
	profile := ConnectionProfile{
		Name:         "Production",
		ServerURL:    "https://remotr.example:8443",
		StateDir:     stateDir,
		CAPath:       caPath,
		DefaultFleet: "production",
	}
	if err := service.SaveProfile(profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("stat desktop settings: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("desktop settings mode = %04o, want 0600", got)
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read desktop settings: %v", err)
	}
	for _, canary := range canaries {
		if strings.Contains(string(raw), canary) {
			t.Errorf("desktop settings persisted secret canary %q", canary)
		}
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse desktop settings: %v", err)
	}
	if len(document) != 1 || document["profiles"] == nil {
		t.Fatalf("desktop settings top-level fields = %v, want only profiles", mapKeys(document))
	}
	var persisted []map[string]json.RawMessage
	if err := json.Unmarshal(document["profiles"], &persisted); err != nil {
		t.Fatalf("parse persisted profiles: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("persisted profile count = %d, want 1", len(persisted))
	}
	wantKeys := []string{"caPath", "defaultFleet", "name", "serverUrl", "stateDir"}
	if got := mapKeys(persisted[0]); !slices.Equal(got, wantKeys) {
		t.Fatalf("persisted profile fields = %v, want %v", got, wantKeys)
	}
}

func TestProfileServiceAtomicallyReplacesExistingSettings(t *testing.T) {
	root := t.TempDir()
	settingsDirectory := filepath.Join(root, "desktop")
	if err := os.Mkdir(settingsDirectory, 0o700); err != nil {
		t.Fatalf("create settings directory: %v", err)
	}
	settingsPath := filepath.Join(settingsDirectory, "profiles.json")
	oldSettings := `{"profiles":[{"name":"Production","serverUrl":"https://old.example:8443","stateDir":"/var/lib/remotr-operator","caPath":"","defaultFleet":"old"}]}`
	if err := os.WriteFile(settingsPath, []byte(oldSettings), 0o644); err != nil {
		t.Fatalf("write existing settings: %v", err)
	}

	service := NewProfileService(settingsPath, "")
	want := ConnectionProfile{
		Name:         "Production",
		ServerURL:    "https://new.example:8443",
		StateDir:     "/var/lib/remotr-operator",
		DefaultFleet: "new",
	}
	if err := service.SaveProfile(want); err != nil {
		t.Fatalf("replace profile: %v", err)
	}

	profiles, err := service.LoadProfiles()
	if err != nil {
		t.Fatalf("load replaced profile: %v", err)
	}
	if !slices.Equal(profiles, []ConnectionProfile{want}) {
		t.Fatalf("profiles after replacement = %#v, want %#v", profiles, []ConnectionProfile{want})
	}
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("stat replaced settings: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("replaced settings mode = %04o, want 0600", got)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(settingsDirectory, ".profiles-*.tmp"))
	if err != nil {
		t.Fatalf("inspect temporary settings: %v", err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary settings remain after replacement: %v", temporaryFiles)
	}
}

func TestProfileServiceRejectsInvalidProfiles(t *testing.T) {
	absoluteStateDir := filepath.Join(t.TempDir(), "operator-state")
	tests := []struct {
		name    string
		profile ConnectionProfile
		field   string
	}{
		{
			name: "empty name",
			profile: ConnectionProfile{
				ServerURL: "https://remotr.example:8443",
				StateDir:  absoluteStateDir,
			},
			field: "name",
		},
		{
			name: "missing server URL",
			profile: ConnectionProfile{
				Name:     "Production",
				StateDir: absoluteStateDir,
			},
			field: "serverUrl",
		},
		{
			name: "non-HTTPS server URL",
			profile: ConnectionProfile{
				Name:      "Production",
				ServerURL: "http://remotr.example:8080",
				StateDir:  absoluteStateDir,
			},
			field: "serverUrl",
		},
		{
			name: "server URL with embedded credentials",
			profile: ConnectionProfile{
				Name:      "Production",
				ServerURL: "https://operator:server-url-secret-canary@remotr.example:8443",
				StateDir:  absoluteStateDir,
			},
			field: "serverUrl",
		},
		{
			name: "relative state directory",
			profile: ConnectionProfile{
				Name:      "Production",
				ServerURL: "https://remotr.example:8443",
				StateDir:  "operator-state",
			},
			field: "stateDir",
		},
		{
			name: "relative CA path",
			profile: ConnectionProfile{
				Name:      "Production",
				ServerURL: "https://remotr.example:8443",
				StateDir:  absoluteStateDir,
				CAPath:    "ca.crt",
			},
			field: "caPath",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "desktop", "profiles.json")
			service := NewProfileService(settingsPath, "")

			err := service.SaveProfile(test.profile)
			var validationErr *ProfileValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("SaveProfile() error = %v, want ProfileValidationError", err)
			}
			if strings.TrimSpace(validationErr.Fields[test.field]) == "" {
				t.Errorf("validation guidance for %s is empty: %#v", test.field, validationErr.Fields)
			}
			if strings.Contains(validationErr.Error(), "server-url-secret-canary") {
				t.Errorf("validation error disclosed server URL credentials: %v", validationErr)
			}
			if _, statErr := os.Stat(settingsPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid profile changed desktop settings: %v", statErr)
			}
		})
	}
}

func TestProfileServiceRejectsOversizedSettings(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(settingsPath, bytes.Repeat([]byte{'x'}, (1<<20)+1), 0o600); err != nil {
		t.Fatalf("write oversized settings fixture: %v", err)
	}

	profiles, err := NewProfileService(settingsPath, "").LoadProfiles()
	if err == nil {
		t.Fatalf("LoadProfiles() accepted oversized settings and returned %#v", profiles)
	}
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
