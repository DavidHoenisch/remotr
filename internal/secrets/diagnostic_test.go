package secrets_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func TestRedactedResolutionErrorClassifiesFailuresWithoutRetainingDiagnostics(t *testing.T) {
	canary := testsupport.SecretCanary("resolution-diagnostic")
	tests := []struct {
		name string
		err  error
		kind string
	}{
		{name: "provider", err: fmt.Errorf("provider emitted %s", canary), kind: "provider"},
		{name: "canceled", err: fmt.Errorf("%s: %w", canary, context.Canceled), kind: "canceled"},
		{name: "deadline", err: fmt.Errorf("%s: %w", canary, context.DeadlineExceeded), kind: "deadline"},
		{name: "unauthorized", err: fmt.Errorf("%s: %w", canary, secrets.ErrUnauthorized), kind: "unauthorized"},
		{name: "invalid reference", err: fmt.Errorf("%s: %w", canary, secrets.ErrInvalidReference), kind: "invalid-reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := secrets.RedactedResolutionError(test.err)
			var typed *secrets.ResolutionError
			if !errors.As(err, &typed) {
				t.Fatalf("RedactedResolutionError() = %T, want *secrets.ResolutionError", err)
			}
			if got := err.Error(); strings.Contains(got, canary) || got != "secret resolution failed ("+test.kind+")" {
				t.Fatalf("redacted diagnostic = %q", got)
			}
		})
	}
	if err := secrets.RedactedResolutionError(nil); err != nil {
		t.Fatalf("RedactedResolutionError(nil) = %v", err)
	}
}
