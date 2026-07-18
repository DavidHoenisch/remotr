package rollbackstore_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// OS-AEC-069: the provider-contract reservation seam blocks before mutation
// when the complete protected recovery cannot fit, without evicting the
// already armed transaction.
func TestStoreReservesCompleteRecoveryBeforeMutation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: root, MaxBytes: 6 << 10, Now: func() time.Time {
			return time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)
		},
		KeyProvider: recoveryTestKeyProvider{},
		AvailableBytes: func(string) (int64, error) {
			return 6 << 10, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstPayload := bytes.Repeat([]byte{0x41}, 256)
	first, err := store.Reserve(ctx, rollbackstore.ReservationRequest{
		Address: "base/sshd", ArtifactDigest: "sha256:first", Attempt: 1,
		PayloadBytes: int64(len(firstPayload)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Arm(ctx, firstPayload); err != nil {
		t.Fatal(err)
	}

	mutated := false
	secondPayload := bytes.Repeat([]byte{0x42}, 256)
	second, err := store.Reserve(ctx, rollbackstore.ReservationRequest{
		Address: "base/network", ArtifactDigest: "sha256:second", Attempt: 1,
		PayloadBytes: int64(len(secondPayload)),
	})
	if !errors.Is(err, rollbackstore.ErrCapacity) || second != nil {
		t.Fatalf("second reservation = %v, %v; want capacity refusal", second, err)
	}
	if err == nil {
		mutated = true
	}
	if mutated {
		t.Fatal("provider mutation ran without protected rollback capacity")
	}
	got, err := store.Load(ctx, "base/sshd", "sha256:first", 1)
	if err != nil || !bytes.Equal(got, firstPayload) {
		t.Fatalf("first armed recovery = %q, %v", got, err)
	}
}

func TestReservationAccountsForProtectedRecordOverhead(t *testing.T) {
	ctx := context.Background()
	request := rollbackstore.ReservationRequest{
		Address: "base/file", ArtifactDigest: "sha256:file", Attempt: 1, PayloadBytes: 128,
	}
	tests := []struct {
		name       string
		allowance  int64
		available  int64
		wantNoRoom bool
	}{
		{name: "payload alone excludes ciphertext and metadata", allowance: 1, available: request.PayloadBytes, wantNoRoom: true},
		{name: "small explicit allowance fits", allowance: 1, available: 2 << 10},
		{name: "filesystem allowance is reserved", allowance: 4 << 10, available: 2 << 10, wantNoRoom: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := rollbackstore.New(rollbackstore.Options{
				Root: t.TempDir(), MaxBytes: 1 << 20, FilesystemAllowance: test.allowance,
				KeyProvider: recoveryTestKeyProvider{},
				AvailableBytes: func(string) (int64, error) {
					return test.available, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			reservation, err := store.Reserve(ctx, request)
			if test.wantNoRoom {
				if reservation != nil || !errors.Is(err, rollbackstore.ErrCapacity) {
					t.Fatalf("reservation = %v, %v; want complete-footprint refusal", reservation, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			reservation.Release()
		})
	}
}

func TestReservationReleasesCapacityAfterOversizedArm(t *testing.T) {
	ctx := context.Background()
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: t.TempDir(), MaxBytes: 1 << 20, KeyProvider: recoveryTestKeyProvider{},
		AvailableBytes: func(string) (int64, error) { return 1 << 20, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := rollbackstore.ReservationRequest{
		Address: "base/file", ArtifactDigest: "sha256:file", Attempt: 1, PayloadBytes: 4,
	}
	reservation, err := store.Reserve(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Arm(ctx, []byte("too large")); !errors.Is(err, rollbackstore.ErrCapacity) {
		t.Fatalf("oversized arm error = %v, want capacity refusal", err)
	}
	request.PayloadBytes = int64(len("new payload"))
	retry, err := store.Reserve(ctx, request)
	if err != nil {
		t.Fatalf("failed arm retained reservation: %v", err)
	}
	retry.Release()
}

func TestReservationHoldsCapacityUntilRelease(t *testing.T) {
	ctx := context.Background()
	store, err := rollbackstore.New(rollbackstore.Options{
		Root: t.TempDir(), MaxBytes: 6 << 10, KeyProvider: recoveryTestKeyProvider{},
		AvailableBytes: func(string) (int64, error) { return 6 << 10, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Reserve(ctx, rollbackstore.ReservationRequest{
		Address: "base/one", ArtifactDigest: "sha256:one", Attempt: 1, PayloadBytes: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondRequest := rollbackstore.ReservationRequest{
		Address: "base/two", ArtifactDigest: "sha256:two", Attempt: 1, PayloadBytes: 256,
	}
	if second, err := store.Reserve(ctx, secondRequest); second != nil || !errors.Is(err, rollbackstore.ErrCapacity) {
		t.Fatalf("overcommitted reservation = %v, %v", second, err)
	}
	first.Release()
	second, err := store.Reserve(ctx, secondRequest)
	if err != nil {
		t.Fatalf("released capacity remained unavailable: %v", err)
	}
	second.Release()
}
