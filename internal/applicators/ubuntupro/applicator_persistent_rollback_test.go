package ubuntupro

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func TestApplicatorProtectedAttachmentRollbackSurvivesRestartWithoutSecrets(t *testing.T) {
	ctx := context.Background()
	const (
		address       = "base/primary-subscription"
		digest        = "sha256:ubuntu-pro-artifact"
		tokenRef      = "remotr:ubuntu-pro/persistence-reference-canary@active"
		tokenMaterial = "ubuntu-pro-persistence-material-canary"
	)
	root := filepath.Join(t.TempDir(), "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint: {attachmentEnvelope(false), []byte(`{"malformed":"post-attach-contract-account-canary"}`)},
		},
		inputOutputs: map[string][][]byte{fullTokenAttachEndpoint: {attachSuccessEnvelope()}},
	}
	resource := attachedResource()
	resource.TokenRef = tokenRef
	applicator := New(resource, exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		return []byte(tokenMaterial), nil
	})
	if err := applicator.ConfigureRollback(store, address, digest); err != nil {
		t.Fatal(err)
	}
	if err := applicator.PreflightRollback(ctx); err != nil {
		t.Fatal(err)
	}
	result := executor.New().Apply(ctx, applicator)
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("Apply() result = %+v, want interrupted post-attach failure", result)
	}

	records, err := store.Records(ctx, address)
	if err != nil || len(records) != 1 || !records[0].Armed {
		t.Fatalf("rollback records = %+v, %v", records, err)
	}
	payload, err := store.Load(ctx, records[0].Address, records[0].ArtifactDigest, records[0].Attempt)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(payload)
	for _, canary := range []string{tokenRef, tokenMaterial, "contract-account-canary"} {
		if strings.Contains(string(payload), canary) {
			t.Fatalf("rollback payload exposed %q: %s", canary, payload)
		}
	}

	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	recoveryRunner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
		isAttachedEndpoint: {attachmentEnvelope(true), attachmentEnvelope(false)},
		detachEndpoint:     {detachEnvelope(false)},
	}}
	restarted := New(resource, exactUbuntuFacts(), recoveryRunner, func(context.Context, string) ([]byte, error) {
		t.Fatal("restart recovery resolved the enrollment token")
		return nil, nil
	})
	if err := restarted.ConfigureRollback(restartedStore, address, digest); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(recoveryRunner.readCalls, []string{isAttachedEndpoint, detachEndpoint, isAttachedEndpoint}) || len(recoveryRunner.inputCalls) != 0 {
		t.Fatalf("recovery endpoints = reads:%v inputs:%v", recoveryRunner.readCalls, recoveryRunner.inputCalls)
	}
	after, err := restartedStore.Records(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range after {
		if record.Armed {
			t.Fatalf("terminal recovery retained armed record: %+v", record)
		}
		if _, err := restartedStore.Load(ctx, record.Address, record.ArtifactDigest, record.Attempt); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("terminal recovery retained payload: %v", err)
		}
	}
}
