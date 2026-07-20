package models_test

import (
	"fmt"
	"strconv"
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
	negativeBurst := -1
	for _, test := range []struct {
		name     string
		resource models.JournaldResource
	}{
		{name: "empty name", resource: models.JournaldResource{Storage: models.JournaldStorageAuto}},
		{name: "oversized name", resource: models.JournaldResource{Name: strings.Repeat("a", 128), Storage: models.JournaldStorageAuto}},
		{name: "missing settings", resource: models.JournaldResource{Name: "policy"}},
		{name: "absent with settings", resource: models.JournaldResource{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "policy", Storage: models.JournaldStorageAuto}},
		{name: "storage", resource: models.JournaldResource{Name: "policy", Storage: "disk"}},
		{name: "duration", resource: models.JournaldResource{Name: "policy", MaxRetention: "30d"}},
		{name: "system bytes", resource: models.JournaldResource{Name: "policy", SystemMaxUseBytes: &negative}},
		{name: "runtime bytes", resource: models.JournaldResource{Name: "policy", RuntimeMaxUseBytes: &negative}},
		{name: "rate burst", resource: models.JournaldResource{Name: "policy", RateLimitBurst: &negativeBurst}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.resource.Validate(); err == nil {
				t.Fatalf("Validate(%#v) succeeded", test.resource)
			}
		})
	}
}

func TestJournaldPolicyValidationAcceptsBoundaryValues(t *testing.T) {
	zeroBytes := int64(0)
	zeroBurst := 0
	for _, resource := range []models.JournaldResource{
		{Name: strings.Repeat("a", 127), Storage: models.JournaldStorageAuto},
		{Name: "zero-retention", MaxRetention: "0s"},
		{Name: "zero-limits", SystemMaxUseBytes: &zeroBytes, RuntimeMaxUseBytes: &zeroBytes, RateLimitBurst: &zeroBurst},
		{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "removed"},
	} {
		if err := resource.Validate(); err != nil {
			t.Fatalf("Validate(%#v) = %v", resource, err)
		}
	}
}

func FuzzParseCanonicalJournaldPolicy(f *testing.F) {
	f.Add("persistent", "720h", "30s", int64(1<<30), int64(256<<20), 10000)
	f.Add("volatile", "0s", "0s", int64(0), int64(0), 0)
	f.Add("invalid", "30d", "-1s", int64(-1), int64(-1), -1)
	f.Fuzz(func(t *testing.T, storage, retention, rateInterval string, systemBytes, runtimeBytes int64, burst int) {
		if len(storage) > 64 || len(retention) > 64 || len(rateInterval) > 64 {
			return
		}
		document := fmt.Sprintf(`schemaVersion: 1
configurations:
  - name: fuzz
    resources:
      - kind: journald
        name: fuzz
        storage: %s
        maxRetention: %s
        systemMaxUseBytes: %d
        runtimeMaxUseBytes: %d
        rateLimitInterval: %s
        rateLimitBurst: %d
        forwardToSyslog: false
`, strconv.Quote(storage), strconv.Quote(retention), systemBytes, runtimeBytes, strconv.Quote(rateInterval), burst)
		state, err := models.ParseState(strings.NewReader(document))
		if err != nil {
			if len(err.Error()) > 1024 {
				t.Fatalf("journald parser diagnostic is unbounded: %d bytes", len(err.Error()))
			}
			return
		}
		if len(state.Configurations) != 1 || len(state.Configurations[0].Journald) != 1 {
			t.Fatalf("accepted canonical journald shape = %#v", state.Configurations)
		}
		resource := state.Configurations[0].Journald[0]
		if string(resource.Storage) != storage || resource.MaxRetention != retention || resource.RateLimitInterval != rateInterval ||
			resource.SystemMaxUseBytes == nil || *resource.SystemMaxUseBytes != systemBytes ||
			resource.RuntimeMaxUseBytes == nil || *resource.RuntimeMaxUseBytes != runtimeBytes ||
			resource.RateLimitBurst == nil || *resource.RateLimitBurst != burst ||
			resource.ForwardToSyslog == nil || *resource.ForwardToSyslog {
			t.Fatalf("accepted journald fields changed = %#v", resource)
		}
		if err := resource.Validate(); err != nil {
			t.Fatalf("parser accepted invalid journald policy: %v", err)
		}
	})
}
