package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// MaxCheckSubresults bounds resource fan-out details in memory and telemetry.
const MaxCheckSubresults = 32

// CheckStatus classifies the observable result of a resource Check.
type CheckStatus string

const (
	Compliant   CheckStatus = "compliant"
	Drifted     CheckStatus = "drifted"
	Unsupported CheckStatus = "unsupported"
	CheckFailed CheckStatus = "check_failed"
	Deferred    CheckStatus = "deferred"
)

// ReasonCode is a stable, machine-readable explanation for a Check outcome.
// Resource-specific codes may be added when they use lower snake case.
type ReasonCode string

const (
	ReasonCompliant           ReasonCode = "compliant"
	ReasonStateDrift          ReasonCode = "state_drift"
	ReasonProviderUnavailable ReasonCode = "provider_unavailable"
	ReasonProbeFailed         ReasonCode = "probe_failed"
	ReasonDeferred            ReasonCode = "deferred"
	ReasonPreflightReady      ReasonCode = "preflight_ready"
	ReasonPreflightFailed     ReasonCode = "preflight_failed"
)

// RedactedSummary contains an already-redacted, human-readable state summary.
// Callers must not place secret values in this type.
type RedactedSummary string

// CheckSubresult is one bounded, already-redacted child outcome for a resource
// that fans out across users or other named targets.
type CheckSubresult struct {
	Target          string
	Status          CheckStatus
	ReasonCode      ReasonCode
	DesiredSummary  RedactedSummary
	ObservedSummary RedactedSummary
}

// CheckResult is the typed, resource-independent outcome of Check.
type CheckResult struct {
	Status              CheckStatus
	ReasonCode          ReasonCode
	DesiredSummary      RedactedSummary
	ObservedSummary     RedactedSummary
	Subresults          []CheckSubresult
	SubresultsTruncated bool
	Actual              any
	Err                 error
}

// Validate confirms that a CheckResult can be safely handled by a consumer
// that recognizes every declared Check status.
func (r CheckResult) Validate() error {
	switch r.Status {
	case Compliant, Drifted, Unsupported, CheckFailed, Deferred:
	default:
		return fmt.Errorf("executor: unknown check status %q", r.Status)
	}
	if !isStableReasonCode(r.ReasonCode) {
		return fmt.Errorf("executor: invalid check reason code %q", r.ReasonCode)
	}
	if r.Status == CheckFailed && r.Err == nil {
		return errors.New("executor: check_failed result requires an error")
	}
	if len(r.Subresults) > MaxCheckSubresults {
		return fmt.Errorf("executor: check result has %d subresults (maximum %d)", len(r.Subresults), MaxCheckSubresults)
	}
	for i, subresult := range r.Subresults {
		if strings.TrimSpace(subresult.Target) == "" {
			return fmt.Errorf("executor: check subresult %d requires a target", i+1)
		}
		switch subresult.Status {
		case Compliant, Drifted, Unsupported, CheckFailed, Deferred:
		default:
			return fmt.Errorf("executor: check subresult %d has unknown status %q", i+1, subresult.Status)
		}
		if !isStableReasonCode(subresult.ReasonCode) {
			return fmt.Errorf("executor: check subresult %d has invalid reason code %q", i+1, subresult.ReasonCode)
		}
	}
	return nil
}

func isStableReasonCode(code ReasonCode) bool {
	if len(code) == 0 || code[0] < 'a' || code[0] > 'z' {
		return false
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// StructuredChecker is implemented by handlers that can return all Check
// outcomes. Handler.State remains supported while providers migrate.
type StructuredChecker interface {
	Check(context.Context) CheckResult
}

// Check returns a handler's structured result. Legacy Handler implementations
// retain their existing behavior by mapping State's boolean to compliant or
// drifted without deriving potentially sensitive text from the actual value.
func Check(ctx context.Context, handler Handler) CheckResult {
	if checker, ok := handler.(StructuredChecker); ok {
		return checker.Check(ctx)
	}
	actual, compliant := handler.State(ctx)
	if compliant {
		return CheckResult{Status: Compliant, ReasonCode: ReasonCompliant, Actual: actual}
	}
	return CheckResult{Status: Drifted, ReasonCode: ReasonStateDrift, Actual: actual}
}
