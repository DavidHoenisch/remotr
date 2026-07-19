package rollbackstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func TestRecordInfoSerializesOnlyClassifiedRetentionMetadata(t *testing.T) {
	expiresAt := time.Date(2026, 7, 20, 12, 30, 0, 123, time.UTC)
	record := rollbackstore.RecordInfo{
		Version: 2, State: rollbackstore.LifecycleArmed, Address: "base/file", ArtifactDigest: "sha256:abc", Attempt: 3,
		CreatedAt: expiresAt.Add(-time.Hour), Armed: true, Sensitive: true, ExpiresAt: expiresAt, PayloadAvailable: true,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var summary executor.SafeSummary
	if err := json.Unmarshal(encoded, &summary); err != nil {
		t.Fatal(err)
	}
	fields := make(map[string]executor.SafeField, len(summary.Fields))
	for _, field := range summary.Fields {
		fields[field.Path] = field
	}
	if field := fields["expires_at"]; field.Sensitivity != executor.SafeSensitiveMetadata || field.Projection != executor.SafeMetadata || field.Text != expiresAt.Format(time.RFC3339Nano) {
		t.Fatalf("classified expiration metadata = %#v", field)
	}
	if field := fields["payload_available"]; field.Sensitivity != executor.SafeSecret || field.Projection != executor.SafePresence || field.Present == nil || !*field.Present {
		t.Fatalf("classified payload availability = %#v", field)
	}

	withoutExpiry, err := (rollbackstore.RecordInfo{Address: "base/file"}).ClassifiedMetadata()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range withoutExpiry.Fields {
		if field.Path == "expires_at" {
			t.Fatalf("zero expiration was projected: %#v", field)
		}
	}
}

// OS-AEC-081: deterministic cleanup bounds per-resource attempt metadata and
// successful non-secret prior payloads while preserving armed recovery.
func TestStoreRetentionPrunesAttemptsAndSuccessfulPayloads(t *testing.T) {
	ctx := context.Background()
	clock := testsupport.NewClock(time.Date(2026, 7, 17, 14, 0, 0, 0, time.UTC))
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: t.TempDir(), MaxAttempts: 4, MaxBytes: 1 << 20, Now: clock.Now,
		FilesystemAllowance: 1, KeyProvider: recoveryTestKeyProvider{},
		AvailableBytes: func(string) (int64, error) { return 1 << 20, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 6; attempt++ {
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/file", ArtifactDigest: fmt.Sprintf("sha256:%d", attempt), Attempt: attempt,
			Payload: []byte(fmt.Sprintf("prior-%d", attempt)), Successful: true,
		}); err != nil {
			t.Fatalf("save attempt %d: %v", attempt, err)
		}
		clock.Advance(time.Minute)
	}
	if err := store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	records, err := store.Records(ctx, "base/file")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("retained attempts = %+v, want exactly 4", records)
	}
	payloads := 0
	for _, record := range records {
		if record.Attempt < 3 {
			t.Fatalf("old attempt survived deterministic pruning: %+v", record)
		}
		if record.PayloadAvailable {
			payloads++
		}
	}
	if payloads != 3 {
		t.Fatalf("retained successful payloads = %d, want 3", payloads)
	}
}

func TestStoreRetentionBoundaries(t *testing.T) {
	ctx := context.Background()
	t.Run("terminal metadata expires at the exact age boundary", func(t *testing.T) {
		clock := testsupport.NewClock(time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC))
		store := newRetentionTestStore(t, clock, rollbackstore.Options{MaxAge: time.Hour})
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/old", ArtifactDigest: "sha256:old", Attempt: 1,
			Payload: []byte("old"), Successful: true,
		}); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Hour)
		if err := store.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
		records, err := store.Records(ctx, "base/old")
		if err != nil || len(records) != 0 {
			t.Fatalf("age-boundary records = %+v, %v", records, err)
		}
	})

	t.Run("armed metadata is never age pruned", func(t *testing.T) {
		clock := testsupport.NewClock(time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC))
		store := newRetentionTestStore(t, clock, rollbackstore.Options{MaxAge: time.Hour})
		payload := []byte("armed")
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/armed", ArtifactDigest: "sha256:armed", Attempt: 1,
			Payload: payload, Armed: true,
		}); err != nil {
			t.Fatal(err)
		}
		clock.Advance(2 * time.Hour)
		if err := store.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(ctx, "base/armed", "sha256:armed", 1)
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("aged armed recovery = %q, %v", got, err)
		}
	})

	t.Run("sensitive payload expires but safe metadata remains", func(t *testing.T) {
		now := time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC)
		clock := testsupport.NewClock(now)
		store := newRetentionTestStore(t, clock, rollbackstore.Options{})
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/secret", ArtifactDigest: "sha256:secret", Attempt: 1,
			Payload: []byte("secret payload"), Armed: true, Sensitive: true, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Hour)
		if err := store.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
		records, err := store.Records(ctx, "base/secret")
		if err != nil || len(records) != 1 || records[0].State != rollbackstore.LifecycleExpired || records[0].Armed || records[0].PayloadAvailable {
			t.Fatalf("expired sensitive record = %+v, %v", records, err)
		}
	})

	t.Run("new attempt supersedes incomplete prior payload", func(t *testing.T) {
		clock := testsupport.NewClock(time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC))
		store := newRetentionTestStore(t, clock, rollbackstore.Options{})
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/staged", ArtifactDigest: "sha256:one", Attempt: 1, Payload: []byte("one"),
		}); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Minute)
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/staged", ArtifactDigest: "sha256:two", Attempt: 2, Payload: []byte("two"), Armed: true,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
		records, err := store.Records(ctx, "base/staged")
		if err != nil || len(records) != 2 {
			t.Fatalf("supersession records = %+v, %v", records, err)
		}
		if records[0].Attempt != 1 || records[0].State != rollbackstore.LifecycleSuperseded || records[0].PayloadAvailable {
			t.Fatalf("prior attempt was not safely superseded: %+v", records[0])
		}
		if records[1].Attempt != 2 || !records[1].Armed || !records[1].PayloadAvailable {
			t.Fatalf("current armed attempt changed during supersession: %+v", records[1])
		}
	})

	t.Run("disk pressure prunes eligible state but not armed state", func(t *testing.T) {
		clock := testsupport.NewClock(time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC))
		store := newRetentionTestStore(t, clock, rollbackstore.Options{MaxBytes: 6 << 10, FilesystemAllowance: 4 << 10})
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/prior", ArtifactDigest: "sha256:prior", Attempt: 1,
			Payload: bytes.Repeat([]byte{0x31}, 256), Successful: true,
		}); err != nil {
			t.Fatal(err)
		}
		reservation, err := store.Reserve(ctx, rollbackstore.ReservationRequest{
			Address: "base/current", ArtifactDigest: "sha256:current", Attempt: 1, PayloadBytes: 256,
		})
		if err != nil {
			t.Fatalf("eligible prior state was not pruned for capacity: %v", err)
		}
		reservation.Release()
		records, err := store.Records(ctx, "base/prior")
		if err != nil || len(records) != 0 {
			t.Fatalf("disk-pruned prior records = %+v, %v", records, err)
		}

		armedPayload := bytes.Repeat([]byte{0x32}, 256)
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/armed-cap", ArtifactDigest: "sha256:armed-cap", Attempt: 1,
			Payload: armedPayload, Armed: true,
		}); err != nil {
			t.Fatal(err)
		}
		blocked, err := store.Reserve(ctx, rollbackstore.ReservationRequest{
			Address: "base/blocked", ArtifactDigest: "sha256:blocked", Attempt: 1, PayloadBytes: 256,
		})
		if blocked != nil || !errors.Is(err, rollbackstore.ErrCapacity) {
			t.Fatalf("armed disk pressure reservation = %v, %v", blocked, err)
		}
		got, err := store.Load(ctx, "base/armed-cap", "sha256:armed-cap", 1)
		if err != nil || !bytes.Equal(got, armedPayload) {
			t.Fatalf("disk pressure evicted armed recovery = %q, %v", got, err)
		}
	})
}

func TestStoreRetentionCountProperty(t *testing.T) {
	ctx := context.Background()
	for total := 0; total <= 20; total++ {
		t.Run(fmt.Sprintf("attempts=%d", total), func(t *testing.T) {
			clock := testsupport.NewClock(time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC))
			store := newRetentionTestStore(t, clock, rollbackstore.Options{MaxAttempts: 10})
			for attempt := 1; attempt <= total; attempt++ {
				if err := store.Save(ctx, rollbackstore.Record{
					Address: "base/property", ArtifactDigest: fmt.Sprintf("sha256:%02d", attempt), Attempt: attempt,
					Payload: []byte{byte(attempt)}, Successful: true,
				}); err != nil {
					t.Fatal(err)
				}
				clock.Advance(time.Second)
			}
			if err := store.Cleanup(ctx); err != nil {
				t.Fatal(err)
			}
			records, err := store.Records(ctx, "base/property")
			if err != nil {
				t.Fatal(err)
			}
			if len(records) > 10 {
				t.Fatalf("retained %d attempts", len(records))
			}
			payloads := 0
			for _, record := range records {
				if record.PayloadAvailable {
					payloads++
				}
			}
			if payloads > 3 {
				t.Fatalf("retained %d successful payloads", payloads)
			}
		})
	}
}

func newRetentionTestStore(t *testing.T, clock *testsupport.Clock, overrides rollbackstore.Options) *rollbackstore.Store {
	t.Helper()
	overrides.Root = t.TempDir()
	overrides.Now = clock.Now
	if overrides.MaxBytes == 0 {
		overrides.MaxBytes = 1 << 20
	}
	if overrides.FilesystemAllowance == 0 {
		overrides.FilesystemAllowance = 1
	}
	overrides.KeyProvider = recoveryTestKeyProvider{}
	overrides.AvailableBytes = func(string) (int64, error) { return 1 << 20, nil }
	store, err := rollbackstore.New(overrides)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
