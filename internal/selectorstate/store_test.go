package selectorstate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/selectorstate"
)

func TestStoreRoundTripsBoundedPrivateOwnershipState(t *testing.T) {
	dir := t.TempDir()
	store := selectorstate.Store{StateDir: dir, Key: "workstation/policy"}
	want := map[string]struct{}{"bob": {}, "alice": {}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("Load() = %v", got)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "interactive-policy-owners"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("state entries = %v, %v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
	path := filepath.Join(dir, "interactive-policy-owners", entries[0].Name())
	if err := os.WriteFile(path, []byte(`{"version":1,"users":["../root"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("Load() accepted corrupt ownership state")
	}
}
