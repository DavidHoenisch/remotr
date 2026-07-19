package rollbackstore_test

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// FuzzRetentionCleanupPreservesArmedAndBoundsTerminalRecords exercises the
// public transaction-store seam with a deterministic clock. The generated
// history is intentionally capped so each fuzz execution has bounded disk and
// CPU cost.
func FuzzRetentionCleanupPreservesArmedAndBoundsTerminalRecords(f *testing.F) {
	f.Add(uint8(4), uint8(8), uint8(0xff), uint8(0))
	f.Add(uint8(1), uint8(20), uint8(0), uint8(2))
	f.Add(uint8(10), uint8(0), uint8(0x55), uint8(3))

	f.Fuzz(func(t *testing.T, rawMaxAttempts, rawRecordCount, successBits, rawAge uint8) {
		ctx := context.Background()
		maxAttempts := int(rawMaxAttempts%10) + 1
		recordCount := int(rawRecordCount % 21)
		maxAge := 2 * time.Hour
		now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
		store, err := rollbackstore.New(rollbackstore.Options{
			Root: t.TempDir(), MaxAttempts: maxAttempts, MaxAge: maxAge,
			MaxBytes: 4 << 20, FilesystemAllowance: 1,
			Now: func() time.Time { return now }, KeyProvider: recoveryTestKeyProvider{},
			AvailableBytes: func(string) (int64, error) { return 4 << 20, nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= recordCount; attempt++ {
			successful := successBits&(1<<uint((attempt-1)%8)) != 0
			if err := store.Save(ctx, rollbackstore.Record{
				Address: "base/file", ArtifactDigest: "sha256:retention", Attempt: attempt,
				Payload: []byte{byte(attempt), successBits}, Successful: successful,
			}); err != nil {
				t.Fatalf("save attempt %d: %v", attempt, err)
			}
			now = now.Add(time.Minute)
		}

		armedPayload := []byte("armed-recovery-must-survive-cleanup")
		if err := store.Save(ctx, rollbackstore.Record{
			Address: "base/armed", ArtifactDigest: "sha256:armed", Attempt: 1,
			Payload: armedPayload, Armed: true,
		}); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Duration(rawAge%4) * time.Hour)
		if err := store.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}

		records, err := store.Records(ctx, "base/file")
		if err != nil {
			t.Fatal(err)
		}
		if len(records) > maxAttempts {
			t.Fatalf("retained %d terminal attempts, maximum is %d", len(records), maxAttempts)
		}
		successfulPayloads := 0
		seenAttempts := make(map[int]struct{}, len(records))
		for index, record := range records {
			if _, duplicate := seenAttempts[record.Attempt]; duplicate {
				t.Fatalf("attempt %d was retained more than once", record.Attempt)
			}
			seenAttempts[record.Attempt] = struct{}{}
			if record.Successful && record.PayloadAvailable {
				successfulPayloads++
			}
			if index > 0 && records[index-1].CreatedAt.After(record.CreatedAt) {
				t.Fatalf("records are not in deterministic oldest-first order: %+v", records)
			}
			if !record.Armed && !record.CreatedAt.After(now.Add(-maxAge)) {
				t.Fatalf("expired unarmed metadata survived cleanup: %+v", record)
			}
		}
		if successfulPayloads > 3 {
			t.Fatalf("retained %d successful non-secret payloads, maximum is 3", successfulPayloads)
		}

		gotArmed, err := store.Load(ctx, "base/armed", "sha256:armed", 1)
		if err != nil || !bytes.Equal(gotArmed, armedPayload) {
			t.Fatalf("armed recovery after cleanup = %q, %v", gotArmed, err)
		}
		beforeSecondCleanup, err := store.Records(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
		afterSecondCleanup, err := store.Records(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(beforeSecondCleanup, afterSecondCleanup) {
			t.Fatalf("cleanup was not idempotent: before=%+v after=%+v", beforeSecondCleanup, afterSecondCleanup)
		}
	})
}
