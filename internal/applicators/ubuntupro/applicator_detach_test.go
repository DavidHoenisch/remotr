package ubuntupro

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func detachEnvelope(reboot bool) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"disabled":[],"reboot_required":%t},"meta":{"environment_vars":[]},"type":"DetachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, reboot))
}

// OS-UPM-025 and OS-UPM-027: explicit detach uses the versioned endpoint,
// proves unattached post-state, signals reboot, and claims no rollback.
func TestApplicatorExplicitDetachConvergesWithoutRollback(t *testing.T) {
	runner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
		isAttachedEndpoint: {attachmentEnvelope(true), attachmentEnvelope(false)},
		detachEndpoint:     {detachEnvelope(true)},
	}}
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProDetached},
		Name:         "primary-subscription",
	}
	result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
	wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackNone || result.RebootRequired != executor.RebootRequired || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("Apply() result = %+v", result)
	}
	wantReads := []string{isAttachedEndpoint, detachEndpoint, isAttachedEndpoint}
	if !slices.Equal(runner.readCalls, wantReads) || len(runner.inputCalls) != 0 {
		t.Fatalf("process endpoints = reads:%v inputs:%v, want %v", runner.readCalls, runner.inputCalls, wantReads)
	}
}

func TestApplicatorDetachNoOpAndFailureBoundaries(t *testing.T) {
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProDetached},
		Name:         "primary-subscription",
	}
	t.Run("already detached", func(t *testing.T) {
		runner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
			isAttachedEndpoint: {attachmentEnvelope(false)},
		}}
		result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
		if result.Status != executor.NoChange || result.RollbackClass != executor.RollbackNone || !slices.Equal(runner.readCalls, []string{isAttachedEndpoint}) {
			t.Fatalf("Apply() result = %+v, reads = %v", result, runner.readCalls)
		}
	})

	t.Run("stable native failure", func(t *testing.T) {
		runner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
			isAttachedEndpoint: {attachmentEnvelope(true), attachmentEnvelope(true)},
			detachEndpoint:     {failureEnvelope("detach-failed", "localized-detach-failure-canary")},
		}}
		applicator := New(resource, exactUbuntuFacts(), runner, nil)
		result := executor.New().Apply(context.Background(), applicator)
		if result.Status != executor.Failed || result.RollbackClass != executor.RollbackNone || result.Err == nil {
			t.Fatalf("Apply() result = %+v", result)
		}
		check := executor.Check(context.Background(), applicator)
		if check.Status != executor.Drifted || check.ReasonCode != executor.ReasonStateDrift {
			t.Fatalf("follow-up Check() = %+v", check)
		}
		if !slices.Equal(runner.readCalls, []string{isAttachedEndpoint, detachEndpoint, isAttachedEndpoint}) {
			t.Fatalf("read endpoints = %v", runner.readCalls)
		}
	})

	t.Run("ambiguous post-state", func(t *testing.T) {
		runner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
			isAttachedEndpoint: {attachmentEnvelope(true), attachmentEnvelope(true)},
			detachEndpoint:     {detachEnvelope(false)},
		}}
		result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
		if result.Status != executor.Failed || result.RollbackClass != executor.RollbackNone || result.Err == nil {
			t.Fatalf("Apply() result = %+v", result)
		}
		if !slices.Equal(runner.readCalls, []string{isAttachedEndpoint, detachEndpoint, isAttachedEndpoint}) {
			t.Fatalf("read endpoints = %v", runner.readCalls)
		}
	})
}
