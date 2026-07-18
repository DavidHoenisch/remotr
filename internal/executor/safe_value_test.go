package executor_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

func TestSafeSummaryRejectsRawSecretProjectionAtConstructionAndSerialization(t *testing.T) {
	unsafe := executor.SafeField{
		Path: "content", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: "secret-canary",
	}
	if _, err := executor.NewSafeSummary([]executor.SafeField{unsafe}); err == nil {
		t.Fatal("NewSafeSummary accepted secret raw-value projection")
	}
	if _, err := json.Marshal(executor.SafeSummary{Fields: []executor.SafeField{unsafe}}); err == nil {
		t.Fatal("MarshalJSON accepted hand-built secret raw-value projection")
	}
}

func TestSafeErrorDiscardsProviderMessageAndCause(t *testing.T) {
	const canary = "provider-error-secret-canary"
	safe := executor.NewSafeError("apply_failed", "provider_apply", errors.New(canary))
	if strings.Contains(safe.Error(), canary) {
		t.Fatalf("safe error retained provider message: %q", safe.Error())
	}
	if errors.Unwrap(safe) != nil {
		t.Fatal("safe error exposed a raw provider cause")
	}
}
