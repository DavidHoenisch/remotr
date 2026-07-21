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
