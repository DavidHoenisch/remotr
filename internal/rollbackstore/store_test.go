package rollbackstore_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// OS-AEC-068, OS-AEC-069, OS-AEC-070: rollback payloads are encrypted under
// agent state, remain recoverable through the store, and are never evicted
// when the disk bound cannot reserve the next armed recovery.
func TestStoreEncryptsPayloadAndProtectsArmedRecoveryAtDiskCap(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	payload := []byte(testsupport.SecretCanary("rollback-payload"))
	store, err := rollbackstore.New(rollbackstore.Options{
		Root:     t.TempDir(),
		MaxBytes: 2048,
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	first := rollbackstore.Record{Address: "base/sshd", ArtifactDigest: "sha256:first", Attempt: 1, Payload: payload, Armed: true}
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(context.Background(), first.Address, first.ArtifactDigest, first.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, payload) {
		t.Fatalf("Load() = %q, want original payload", loaded)
	}

	if err := filepath.Walk(store.Root(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, payload) {
			t.Fatalf("durable rollback file %s contains plaintext payload", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	second := rollbackstore.Record{Address: "base/network", ArtifactDigest: "sha256:second", Attempt: 1, Payload: bytes.Repeat([]byte("x"), 1900), Armed: true}
	if err := store.Save(context.Background(), second); !errors.Is(err, rollbackstore.ErrCapacity) {
		t.Fatalf("Save() error = %v, want capacity refusal", err)
	}
	if _, err := store.Load(context.Background(), first.Address, first.ArtifactDigest, first.Attempt); err != nil {
		t.Fatalf("armed recovery was lost after capacity refusal: %v", err)
	}
}
