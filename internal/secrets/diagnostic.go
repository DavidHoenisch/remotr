package secrets

import (
	"context"
	"errors"
)

// ResolutionError is safe to retain in Apply results, reports, and audit
// records. It deliberately does not unwrap the provider error because backend
// diagnostics can contain resolved material.
type ResolutionError struct {
	kind string
}

func (e *ResolutionError) Error() string {
	if e == nil {
		return "secret resolution failed"
	}
	return "secret resolution failed (" + e.kind + ")"
}

// RedactedResolutionError converts an untrusted provider failure into typed,
// bounded diagnostic metadata without preserving its text or error chain.
func RedactedResolutionError(err error) error {
	if err == nil {
		return nil
	}
	var redacted *ResolutionError
	if errors.As(err, &redacted) {
		return redacted
	}
	kind := "provider"
	switch {
	case errors.Is(err, context.Canceled):
		kind = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		kind = "deadline"
	case errors.Is(err, ErrUnauthorized):
		kind = "unauthorized"
	case errors.Is(err, ErrInvalidReference):
		kind = "invalid-reference"
	}
	return &ResolutionError{kind: kind}
}

// IsResolutionUnauthorized reports whether a redacted provider failure is an
// authorization denial. Callers may use this classification to retain safe,
// non-mutating evidence, but must not treat it as permission to expose or use
// secret material.
func IsResolutionUnauthorized(err error) bool {
	var redacted *ResolutionError
	return errors.As(err, &redacted) && redacted.kind == "unauthorized"
}
