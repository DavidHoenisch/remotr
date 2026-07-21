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

func TestApplicatorNewAttachmentRollbackFailureIsBoundedAndNotRetried(t *testing.T) {
	tests := []struct {
		name       string
		statuses   [][]byte
		detach     []byte
		wantSuffix string
		wantReads  []string
	}{
		{
			name:       "stable detach failure",
			statuses:   [][]byte{attachmentEnvelope(false), attachmentEnvelope(true)},
			detach:     failureEnvelope("detach-failed", "localized-rollback-detach-canary"),
			wantSuffix: "attachment rollback failed",
			wantReads:  []string{isAttachedEndpoint, isAttachedEndpoint, enabledServicesEndpoint, dependenciesEndpoint, detachEndpoint},
		},
		{
			name:       "attached after detach success",
			statuses:   [][]byte{attachmentEnvelope(false), attachmentEnvelope(true), attachmentEnvelope(true)},
			detach:     detachEnvelope(false),
			wantSuffix: "attachment rollback check failed",
			wantReads:  []string{isAttachedEndpoint, isAttachedEndpoint, enabledServicesEndpoint, dependenciesEndpoint, detachEndpoint, isAttachedEndpoint},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &serviceLifecycleRunner{
				readOutputs: map[string][][]byte{
					isAttachedEndpoint:      test.statuses,
					enabledServicesEndpoint: {enabledServicesEnvelope()},
					detachEndpoint:          {test.detach},
				},
				inputOutputs: map[string][][]byte{
					fullTokenAttachEndpoint: {attachSuccessEnvelope()},
					enableEndpoint:          {failureEnvelope("service-not-entitled", "localized-entitlement-canary")},
				},
			}
			resource := attachedResource()
			resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}}
			result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
				return []byte("failed-attachment-rollback-token-canary"), nil
			}))
			if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "service-not-entitled") || !strings.Contains(result.Err.Error(), test.wantSuffix) {
				t.Fatalf("Apply() result = %+v", result)
			}
			if strings.Contains(result.Err.Error(), "localized-") {
				t.Fatalf("Apply() exposed localized native data: %v", result.Err)
			}
			if !slices.Equal(runner.readCalls, test.wantReads) {
				t.Fatalf("read endpoints = %v, want %v", runner.readCalls, test.wantReads)
			}
			if !slices.Equal(runner.inputCalls, []string{fullTokenAttachEndpoint, enableEndpoint}) {
				t.Fatalf("input endpoints = %v", runner.inputCalls)
			}
		})
	}
}
