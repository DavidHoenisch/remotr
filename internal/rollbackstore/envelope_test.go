package rollbackstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// OS-AEC-080: every crash boundary either leaves no active record or one
// complete authenticated envelope recoverable after restart.
func TestStoreActivatesOneEnvelopeAtEveryDurabilityBoundary(t *testing.T) {
	ctx := context.Background()
	stop := errors.New("injected process stop")
	tests := []struct {
		point       rollbackstore.DurabilityPoint
		wantRecover bool
	}{
		{point: rollbackstore.AfterTemporaryCreate},
		{point: rollbackstore.AfterEnvelopeWrite},
		{point: rollbackstore.AfterEnvelopeSync},
		{point: rollbackstore.AfterEnvelopeActivate, wantRecover: true},
		{point: rollbackstore.AfterDirectorySync, wantRecover: true},
	}
	for _, test := range tests {
		t.Run(string(test.point), func(t *testing.T) {
			root := t.TempDir()
			injected := false
			store, err := rollbackstore.New(rollbackstore.Options{
				Root: root, KeyProvider: recoveryTestKeyProvider{},
				CrashInjector: func(point rollbackstore.DurabilityPoint) error {
					if point == test.point && !injected {
						injected = true
						return stop
					}
					return nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			payload := []byte("known envelope rollback payload")
			err = store.Save(ctx, rollbackstore.Record{
				Address: "base/envelope", ArtifactDigest: "sha256:envelope", Attempt: 1,
				Payload: payload, Armed: true,
			})
			if !errors.Is(err, stop) || !injected {
				t.Fatalf("injected save error = %v, injected=%t", err, injected)
			}

			restarted, err := rollbackstore.New(rollbackstore.Options{Root: root, KeyProvider: recoveryTestKeyProvider{}})
			if err != nil {
				t.Fatal(err)
			}
			recovered := 0
			if err := restarted.RecoverArmed(ctx, func(_ context.Context, recovery rollbackstore.Recovery) error {
				recovered++
				if !bytes.Equal(recovery.Payload, payload) {
					t.Fatalf("recovered payload = %q, want %q", recovery.Payload, payload)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			want := 0
			if test.wantRecover {
				want = 1
			}
			if recovered != want {
				t.Fatalf("recovery count = %d, want %d", recovered, want)
			}
			if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil || info.IsDir() {
					return walkErr
				}
				if info.Name() == "metadata.json" || info.Name() == "payload.bin" || filepath.Ext(info.Name()) == ".tmp" {
					t.Fatalf("obsolete or incomplete transaction file survived restart: %s", path)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEnvelopeAuthenticatesMetadataAndCiphertext(t *testing.T) {
	ctx := context.Background()
	payload := []byte("envelope-secret-canary")
	tests := []struct {
		name   string
		mutate func(*testing.T, []byte) []byte
	}{
		{
			name: "metadata",
			mutate: func(t *testing.T, raw []byte) []byte {
				t.Helper()
				before := []byte(`"created_at":"2026-07-17T17:00:00Z"`)
				after := []byte(`"created_at":"2026-07-17T17:00:01Z"`)
				changed := bytes.Replace(raw, before, after, 1)
				if bytes.Equal(changed, raw) {
					t.Fatal("fixture did not contain authenticated timestamp")
				}
				return changed
			},
		},
		{
			name: "ciphertext",
			mutate: func(t *testing.T, raw []byte) []byte {
				t.Helper()
				var envelope map[string]any
				if err := json.Unmarshal(raw, &envelope); err != nil {
					t.Fatal(err)
				}
				ciphertext := envelope["ciphertext"].(string)
				replacement := byte('A')
				if ciphertext[0] == replacement {
					replacement = 'B'
				}
				envelope["ciphertext"] = string(replacement) + ciphertext[1:]
				changed, err := json.Marshal(envelope)
				if err != nil {
					t.Fatal(err)
				}
				return changed
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			options := rollbackstore.Options{
				Root: root, KeyProvider: recoveryTestKeyProvider{},
				Now: func() time.Time { return time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC) },
			}
			store, err := rollbackstore.New(options)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save(ctx, rollbackstore.Record{
				Address: "base/tamper", ArtifactDigest: "sha256:tamper", Attempt: 1,
				Payload: payload, Armed: true,
			}); err != nil {
				t.Fatal(err)
			}
			path := findEnvelope(t, root)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, test.mutate(t, raw), 0o600); err != nil {
				t.Fatal(err)
			}
			restarted, err := rollbackstore.New(options)
			if restarted != nil || !errors.Is(err, rollbackstore.ErrRecoveryBlocked) {
				t.Fatalf("tampered startup = %v, %v", restarted, err)
			}
			if bytes.Contains([]byte(err.Error()), payload) {
				t.Fatalf("tampered startup leaked payload: %v", err)
			}
		})
	}
}

func findEnvelope(t *testing.T, root string) string {
	t.Helper()
	var found string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		if info.Name() == "transaction.envelope" {
			if found != "" {
				t.Fatal("multiple transaction envelopes found")
			}
			found = path
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatal("transaction envelope not found")
	}
	return found
}
