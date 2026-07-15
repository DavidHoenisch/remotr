package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalStructuredJournaldPolicy(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: logging
    resources:
      - kind: journald
        name: retention
        storage: persistent
        maxRetention: 720h
        systemMaxUseBytes: 1073741824
        runtimeMaxUseBytes: 268435456
        rateLimitInterval: 30s
        rateLimitBurst: 10000
        forwardToSyslog: true
        forwardToKernelBuffer: false
        forwardToConsole: false
        forwardToWall: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Journald) != 1 {
		t.Fatalf("parsed state = %#v", state)
	}
	resource := state.Configurations[0].Journald[0]
	if resource.Kind != models.ResourceKindJournald || resource.Storage != models.JournaldStoragePersistent ||
		resource.SystemMaxUseBytes == nil || *resource.SystemMaxUseBytes != 1<<30 ||
		resource.ForwardToKernelBuffer == nil || *resource.ForwardToKernelBuffer {
		t.Fatalf("journald policy = %#v", resource)
	}
}

func TestJournaldPolicyValidationRejectsInvalidBoundaries(t *testing.T) {
	negative := int64(-1)
	for _, test := range []struct {
		name     string
		resource models.JournaldResource
	}{
		{name: "storage", resource: models.JournaldResource{Name: "policy", Storage: "disk"}},
		{name: "duration", resource: models.JournaldResource{Name: "policy", MaxRetention: "30d"}},
		{name: "bytes", resource: models.JournaldResource{Name: "policy", SystemMaxUseBytes: &negative}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.resource.Validate(); err == nil {
				t.Fatalf("Validate(%#v) succeeded", test.resource)
			}
		})
	}
}
