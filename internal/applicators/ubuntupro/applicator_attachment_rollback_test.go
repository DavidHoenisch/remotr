package ubuntupro

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-UPM-023 and OS-UPM-024: apply-local rollback may remove only the
// attachment created by this apply. A pre-existing attachment is never owned
// by the failed operation and must remain attached.
func TestApplicatorServiceFailureRollsBackOnlyNewAttachment(t *testing.T) {
	t.Run("new attachment", func(t *testing.T) {
		runner := &serviceLifecycleRunner{
			readOutputs: map[string][][]byte{
				isAttachedEndpoint:      {attachmentEnvelope(false), attachmentEnvelope(true), attachmentEnvelope(false)},
				enabledServicesEndpoint: {enabledServicesEnvelope()},
				detachEndpoint:          {detachEnvelope(false)},
			},
			inputOutputs: map[string][][]byte{
				fullTokenAttachEndpoint: {attachSuccessEnvelope()},
				enableEndpoint:          {failureEnvelope("service-not-entitled", "localized-entitlement-canary")},
			},
		}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}}
		result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
			return []byte("attachment-rollback-token-canary"), nil
		}))
		if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "service-not-entitled") || !strings.Contains(result.Err.Error(), "attachment rollback restored") {
			t.Fatalf("Apply() result = %+v", result)
		}
		wantReads := []string{isAttachedEndpoint, isAttachedEndpoint, enabledServicesEndpoint, dependenciesEndpoint, detachEndpoint, isAttachedEndpoint}
		if !slices.Equal(runner.readCalls, wantReads) {
			t.Fatalf("read endpoints = %v, want %v", runner.readCalls, wantReads)
		}
		if !slices.Equal(runner.inputCalls, []string{fullTokenAttachEndpoint, enableEndpoint}) {
			t.Fatalf("input endpoints = %v", runner.inputCalls)
		}
	})

	t.Run("pre-existing attachment", func(t *testing.T) {
		runner := &serviceLifecycleRunner{
			readOutputs: map[string][][]byte{
				isAttachedEndpoint:      {attachmentEnvelope(true)},
				enabledServicesEndpoint: {enabledServicesEnvelope()},
			},
			inputOutputs: map[string][][]byte{
				enableEndpoint: {failureEnvelope("service-not-entitled", "localized-entitlement-canary")},
			},
		}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}}
		result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
		if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "service-not-entitled") {
			t.Fatalf("Apply() result = %+v", result)
		}
		if slices.Contains(runner.readCalls, detachEndpoint) || slices.Contains(runner.inputCalls, fullTokenAttachEndpoint) {
			t.Fatalf("pre-existing attachment was mutated: reads=%v inputs=%v", runner.readCalls, runner.inputCalls)
		}
	})
}
