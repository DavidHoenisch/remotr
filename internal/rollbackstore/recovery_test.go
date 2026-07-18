package rollbackstore_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// OS-AEC-080: the system-safety recovery seam restores an armed transaction
// after process reconstruction and blocks another mutation of that resource
// until recovery reaches a terminal state.
func TestStoreRecoversArmedTransactionAfterRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	options := rollbackstore.Options{
		Root: root, Now: func() time.Time { return now },
		KeyProvider: recoveryTestKeyProvider{},
	}
	store, err := rollbackstore.New(options)
	if err != nil {
		t.Fatal(err)
	}
	armed := rollbackstore.Record{
		Address: "base/sshd", ArtifactDigest: "sha256:authorized", Attempt: 3,
		Payload: []byte("known prior sshd state\n"), Armed: true,
	}
	if err := store.Save(ctx, armed); err != nil {
		t.Fatal(err)
	}

	restarted, err := rollbackstore.New(options)
	if err != nil {
		t.Fatal(err)
	}
	replacement := rollbackstore.Record{
		Address: armed.Address, ArtifactDigest: "sha256:replacement", Attempt: 4,
		Payload: []byte("another prior state"), Armed: true,
	}
	if err := restarted.Save(ctx, replacement); !errors.Is(err, rollbackstore.ErrArmedRecovery) {
		t.Fatalf("second mutation reservation error = %v, want armed-recovery block", err)
	}

	recovered := 0
	err = restarted.RecoverArmed(ctx, func(_ context.Context, recovery rollbackstore.Recovery) error {
		recovered++
		if recovery.Address != armed.Address || recovery.ArtifactDigest != armed.ArtifactDigest || recovery.Attempt != armed.Attempt {
			t.Fatalf("recovery key = %+v", recovery)
		}
		if !bytes.Equal(recovery.Payload, armed.Payload) {
			t.Fatalf("recovery payload = %q, want %q", recovery.Payload, armed.Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovery callback count = %d, want 1", recovered)
	}
	if err := assertNoPlaintextBelow(root, armed.Payload); err != nil {
		t.Fatal(err)
	}

	restartedAgain, err := rollbackstore.New(options)
	if err != nil {
		t.Fatal(err)
	}
	if err := restartedAgain.RecoverArmed(ctx, func(context.Context, rollbackstore.Recovery) error {
		t.Fatal("terminal recovery was replayed")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedAgain.Save(ctx, replacement); err != nil {
		t.Fatalf("new mutation remained blocked after recovery: %v", err)
	}
}

func TestStoreKeepsArmedTransactionWhenRecoveryFails(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	options := rollbackstore.Options{Root: root, KeyProvider: recoveryTestKeyProvider{}}
	store, err := rollbackstore.New(options)
	if err != nil {
		t.Fatal(err)
	}
	record := rollbackstore.Record{
		Address: "base/access", ArtifactDigest: "sha256:access", Attempt: 1,
		Payload: []byte("known access recovery"), Armed: true,
	}
	if err := store.Save(ctx, record); err != nil {
		t.Fatal(err)
	}

	restarted, err := rollbackstore.New(options)
	if err != nil {
		t.Fatal(err)
	}
	err = restarted.RecoverArmed(ctx, func(context.Context, rollbackstore.Recovery) error {
		return errors.New("provider-secret-canary")
	})
	if !errors.Is(err, rollbackstore.ErrRecoveryBlocked) {
		t.Fatalf("recovery error = %v, want fail-closed recovery block", err)
	}
	if bytes.Contains([]byte(err.Error()), []byte("provider-secret-canary")) {
		t.Fatalf("recovery error leaked provider detail: %v", err)
	}

	restartedAgain, err := rollbackstore.New(options)
	if err != nil {
		t.Fatal(err)
	}
	recovered := 0
	if err := restartedAgain.RecoverArmed(ctx, func(_ context.Context, got rollbackstore.Recovery) error {
		recovered++
		if !bytes.Equal(got.Payload, record.Payload) {
			t.Fatalf("retry payload = %q, want %q", got.Payload, record.Payload)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovery retry count = %d, want 1", recovered)
	}
}

type recoveryTestKeyProvider struct{}

func (recoveryTestKeyProvider) LoadOrCreate(context.Context, string) ([]byte, error) {
	return bytes.Repeat([]byte{0x37}, 32), nil
}
