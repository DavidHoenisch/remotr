package rollbackstore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
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

// OS-AEC-084: sensitive rollback payloads remain available for recovery while
// generic metadata output receives only an approved classified projection.
func TestSensitiveRollbackMetadataOmitsPayloadFingerprintAndSerializesClassified(t *testing.T) {
	ctx := context.Background()
	const canary = "rollback-metadata-secret-canary"
	payload := []byte(canary)
	payloadDigest := sha256.Sum256(payload)
	now := time.Date(2026, 7, 18, 13, 0, 0, 0, time.UTC)
	root := t.TempDir()
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: root, KeyProvider: recoveryTestKeyProvider{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, rollbackstore.Record{
		Address: "base/secret", ArtifactDigest: "sha256:desired", Attempt: 1,
		Payload: payload, Armed: true, Sensitive: true, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	rawEnvelope, err := os.ReadFile(findEnvelope(t, root))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{payload, []byte(hex.EncodeToString(payloadDigest[:])), []byte(`"checksum"`)} {
		if bytes.Contains(rawEnvelope, forbidden) {
			t.Fatalf("rollback envelope metadata exposed sensitive payload evidence %q", forbidden)
		}
	}

	records, err := store.Records(ctx, "base/secret")
	if err != nil || len(records) != 1 {
		t.Fatalf("Records() = %+v, %v", records, err)
	}
	encoded, err := json.Marshal(records[0])
	if err != nil {
		t.Fatal(err)
	}
	var classified executor.SafeSummary
	if err := json.Unmarshal(encoded, &classified); err != nil {
		t.Fatalf("rollback metadata is not a classified summary: %v", err)
	}
	if err := classified.Validate(); err != nil || len(classified.Fields) == 0 {
		t.Fatalf("classified rollback metadata = %+v, %v", classified, err)
	}
	if bytes.Contains(encoded, payload) {
		t.Fatalf("classified rollback metadata leaked payload: %s", encoded)
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
