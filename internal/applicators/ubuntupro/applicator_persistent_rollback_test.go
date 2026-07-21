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
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func TestApplicatorProtectedAttachmentRollbackCleansSuccessfulTransaction(t *testing.T) {
	ctx := context.Background()
	const canary = "successful-ubuntu-pro-rollback-token-canary"
	root := filepath.Join(t.TempDir(), "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	runner := &serviceLifecycleRunner{
		readOutputs:  map[string][][]byte{isAttachedEndpoint: {attachmentEnvelope(false), attachmentEnvelope(true)}},
		inputOutputs: map[string][][]byte{fullTokenAttachEndpoint: {attachSuccessEnvelope()}},
	}
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		return []byte(canary), nil
	})
	if err := applicator.ConfigureRollback(store, "base/successful-subscription", "sha256:successful-artifact"); err != nil {
		t.Fatal(err)
	}
	if result := executor.New().Apply(ctx, applicator); result.Status != executor.Changed {
		t.Fatalf("Apply() result = %+v", result)
	}
	records, err := store.Records(ctx, "base/successful-subscription")
	if err != nil || len(records) != 1 || records[0].Armed {
		t.Fatalf("terminal records = %+v, %v", records, err)
	}
	if _, err := store.Load(ctx, records[0].Address, records[0].ArtifactDigest, records[0].Attempt); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal acknowledgment retained rollback payload: %v", err)
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), canary) {
			t.Fatalf("rollback store file %s retained token material", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicatorProtectedAttachmentRollbackRejectsMalformedSnapshot(t *testing.T) {
	ctx := context.Background()
	const address = "base/malformed-subscription"
	root := filepath.Join(t.TempDir(), "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, rollbackstore.Record{
		Address: address, ArtifactDigest: "sha256:malformed-artifact", Attempt: 1,
		Payload: []byte(`{"version":99,"token":"rollback-token-canary"}`), Armed: true,
	}); err != nil {
		t.Fatal(err)
	}
	runner := &serviceLifecycleRunner{}
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, nil)
	if err := applicator.ConfigureRollback(store, address, "sha256:malformed-artifact"); err != nil {
		t.Fatal(err)
	}
	err = applicator.Revert(ctx)
	if !errors.Is(err, rollbackstore.ErrRecoveryBlocked) {
		t.Fatalf("Revert() error = %v, want recovery blocked", err)
	}
	if len(runner.readCalls) != 0 || len(runner.inputCalls) != 0 {
		t.Fatalf("malformed recovery reached process boundary: reads=%v inputs=%v", runner.readCalls, runner.inputCalls)
	}
	records, err := store.Records(ctx, address)
	if err != nil || len(records) != 1 || !records[0].Armed {
		t.Fatalf("failed recovery records = %+v, %v", records, err)
	}
}

func TestApplicatorProtectedServiceRollbackSurvivesRestartInReverseOrder(t *testing.T) {
	ctx := context.Background()
	const (
		address = "base/service-subscription"
		digest  = "sha256:service-artifact"
	)
	root := filepath.Join(t.TempDir(), "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope()},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint: {
				serviceTransitionEnvelope([]string{"esm-apps"}, nil),
				serviceTransitionEnvelope([]string{"ros"}, nil),
				failureEnvelope("service-not-entitled", "localized-third-service-canary"),
			},
			disableEndpoint: {failureEnvelope("restore-failed", "localized-immediate-rollback-canary")},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{
		{Name: "esm-apps", State: models.UbuntuProServiceEnabled},
		{Name: "ros", State: models.UbuntuProServiceEnabled},
		{Name: "ros-updates", State: models.UbuntuProServiceEnabled},
	}
	applicator := New(resource, exactUbuntuFacts(), runner, nil)
	if err := applicator.ConfigureRollback(store, address, digest); err != nil {
		t.Fatal(err)
	}
	if err := applicator.PreflightRollback(ctx); err != nil {
		t.Fatal(err)
	}
	result := executor.New().Apply(ctx, applicator)
	if result.Status != executor.Failed || result.RollbackClass != executor.RollbackBestEffort || result.Err == nil || !strings.Contains(result.Err.Error(), "rollback failed") {
		t.Fatalf("Apply() result = %+v", result)
	}
	records, err := store.Records(ctx, address)
	if err != nil || len(records) != 1 || !records[0].Armed {
		t.Fatalf("rollback records = %+v, %v", records, err)
	}

	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	recoveryRunner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			enabledServicesEndpoint: {enabledServicesEnvelope("esm-apps", "ros"), enabledServicesEnvelope()},
		},
		inputOutputs: map[string][][]byte{
			disableEndpoint: {
				serviceTransitionEnvelope(nil, []string{"ros"}),
				serviceTransitionEnvelope(nil, []string{"esm-apps"}),
			},
		},
	}
	restarted := New(resource, exactUbuntuFacts(), recoveryRunner, func(context.Context, string) ([]byte, error) {
		t.Fatal("service recovery resolved the enrollment token")
		return nil, nil
	})
	if err := restarted.ConfigureRollback(restartedStore, address, digest); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(recoveryRunner.readCalls, []string{enabledServicesEndpoint, enabledServicesEndpoint}) || !slices.Equal(recoveryRunner.inputCalls, []string{disableEndpoint, disableEndpoint}) {
		t.Fatalf("recovery endpoints = reads:%v inputs:%v", recoveryRunner.readCalls, recoveryRunner.inputCalls)
	}
	wantInputs := [][]byte{[]byte(`{"service":"ros","purge":false}`), []byte(`{"service":"esm-apps","purge":false}`)}
	if len(recoveryRunner.inputs) != len(wantInputs) {
		t.Fatalf("recovery inputs = %q", recoveryRunner.inputs)
	}
	for index := range wantInputs {
		if !slices.Equal(recoveryRunner.inputs[index], wantInputs[index]) {
			t.Fatalf("recovery input %d = %s, want %s", index, recoveryRunner.inputs[index], wantInputs[index])
		}
	}
	after, err := restartedStore.Records(ctx, address)
	if err != nil || len(after) != 1 || after[0].Armed {
		t.Fatalf("terminal records = %+v, %v", after, err)
	}
}

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
