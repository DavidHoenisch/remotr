// Package providercontract contains reusable behavioral cases for Remotr
// providers. Fixtures construct providers through the exported provider
// contract rather than reaching into provider implementations.
package providercontract

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// Fixture supplies independent providers in known compliant and drifted
// states. Each factory should use a provider's supported constructor and
// controlled external boundaries such as a fake command runner or temp tree.
type Fixture struct {
	Compliant func(*testing.T) contract.Provider
	Drifted   func(*testing.T) contract.Provider
}

// AbsenceFixture supplies a provider configured to ensure a resource is
// absent. Absent is already compliant; Present must be removed by Apply.
type AbsenceFixture struct {
	Absent  func(*testing.T) contract.Provider
	Present func(*testing.T) contract.Provider
}

// NegativeFixture supplies typed public observations for cases that cannot be
// represented as ordinary drift. Validate fails before a provider is built.
type NegativeFixture struct {
	Unsupported  func(*testing.T) contract.Provider
	ProbeFailure func(*testing.T) contract.Provider
	Validate     func(*testing.T) error
}

// OperationFixture supplies providers for safety behaviors that need a
// controlled external boundary or concurrent caller.
type OperationFixture struct {
	LockContended func(*testing.T) contract.Provider
	Canceled      func(*testing.T) contract.Provider
	TimedOut      func(*testing.T) contract.Provider
	Concurrent    func(*testing.T) contract.Provider
}

// Activator executes a provider's ordered activation names at the controlled
// activation boundary.
type Activator interface {
	Activate(context.Context, []string) ([]string, error)
}

// ActivationFixture describes a requested activation sequence and its unique,
// dependency-respecting result.
type ActivationFixture struct {
	Activator func(*testing.T) Activator
	Requested []string
	Want      []string
}

// RedactionFixture supplies a provider diagnostic redactor and a synthetic
// canary that must not escape its output.
type RedactionFixture struct {
	Redact func(*testing.T, string) string
	Canary string
}

// RollbackFixture supplies the public rollback classes used by the contract.
type RollbackFixture struct {
	Reverted   func(*testing.T) contract.Provider
	NoRollback func(*testing.T) contract.Provider
	Failure    func(*testing.T) contract.Provider
}

// Suite contains every case family required before a new or changed provider
// can be advertised. Keeping one aggregate entry point prevents a provider
// migration from selecting only the convenient convergence cases.
type Suite struct {
	Convergence Fixture
	Absence     AbsenceFixture
	Negative    NegativeFixture
	Operations  OperationFixture
	Activation  ActivationFixture
	Redaction   RedactionFixture
	Rollback    RollbackFixture
}

// RunConformance executes the complete provider contract. A provider may use
// explicit unsupported/no-rollback results where its contract does not offer a
// capability, but it may not omit the observable case.
func RunConformance(t *testing.T, suite Suite) {
	t.Helper()
	t.Run("convergence", func(t *testing.T) { RunConvergence(t, suite.Convergence) })
	t.Run("absence", func(t *testing.T) { RunAbsence(t, suite.Absence) })
	t.Run("negative checks", func(t *testing.T) { RunNegativeChecks(t, suite.Negative) })
	t.Run("operation safety", func(t *testing.T) { RunOperationSafety(t, suite.Operations) })
	t.Run("activation", func(t *testing.T) { RunActivation(t, suite.Activation) })
	t.Run("redaction", func(t *testing.T) { RunRedactionCanary(t, suite.Redaction) })
	t.Run("rollback", func(t *testing.T) { RunRollback(t, suite.Rollback) })
}

// RunConvergence verifies the provider contract's common convergence path:
// compliant providers do not mutate; drifted providers mutate once, report
// compliant on the next Check, and do not mutate again.
func RunConvergence(t *testing.T, fixture Fixture) {
	t.Helper()
	if fixture.Compliant == nil || fixture.Drifted == nil {
		t.Fatal("provider contract fixture requires compliant and drifted factories")
	}

	t.Run("compliant state does not mutate", func(t *testing.T) {
		provider := fixture.Compliant(t)
		assertStatus(t, provider.Check(context.Background()), contract.Compliant)
		assertApplyStatus(t, provider.Apply(context.Background()), contract.NoChange)
	})

	t.Run("drift converges once and stays compliant", func(t *testing.T) {
		provider := fixture.Drifted(t)
		assertStatus(t, provider.Check(context.Background()), contract.Drifted)
		assertApplyStatus(t, provider.Apply(context.Background()), contract.Changed)
		assertStatus(t, provider.Check(context.Background()), contract.Compliant)
		assertApplyStatus(t, provider.Apply(context.Background()), contract.NoChange)
	})
}

// RunAbsence verifies that a provider can converge an existing resource to
// the configured absent state without repeating the removal.
func RunAbsence(t *testing.T, fixture AbsenceFixture) {
	t.Helper()
	if fixture.Absent == nil || fixture.Present == nil {
		t.Fatal("absence fixture requires absent and present factories")
	}

	t.Run("already absent does not mutate", func(t *testing.T) {
		provider := fixture.Absent(t)
		assertStatus(t, provider.Check(context.Background()), contract.Compliant)
		assertApplyStatus(t, provider.Apply(context.Background()), contract.NoChange)
	})

	t.Run("present resource is removed once", func(t *testing.T) {
		provider := fixture.Present(t)
		assertStatus(t, provider.Check(context.Background()), contract.Drifted)
		assertApplyStatus(t, provider.Apply(context.Background()), contract.Changed)
		assertStatus(t, provider.Check(context.Background()), contract.Compliant)
		assertApplyStatus(t, provider.Apply(context.Background()), contract.NoChange)
	})
}

// RunNegativeChecks verifies that unsupported capability, failed probing, and
// invalid configuration remain distinct from ordinary drift.
func RunNegativeChecks(t *testing.T, fixture NegativeFixture) {
	t.Helper()
	if fixture.Unsupported == nil || fixture.ProbeFailure == nil || fixture.Validate == nil {
		t.Fatal("negative fixture requires unsupported, probe failure, and validation factories")
	}

	t.Run("unsupported capability is typed", func(t *testing.T) {
		assertStatus(t, fixture.Unsupported(t).Check(context.Background()), contract.Unsupported)
	})

	t.Run("probe failure is typed", func(t *testing.T) {
		observation := fixture.ProbeFailure(t).Check(context.Background())
		assertStatus(t, observation, contract.CheckFailed)
		if observation.Err == nil {
			t.Fatal("failed probe did not retain an error")
		}
	})

	t.Run("invalid configuration fails before provider construction", func(t *testing.T) {
		if err := fixture.Validate(t); err == nil {
			t.Fatal("invalid configuration was accepted")
		}
	})
}

// RunOperationSafety verifies that operational failures do not masquerade as
// successful convergence and that concurrent Apply calls mutate at most once.
func RunOperationSafety(t *testing.T, fixture OperationFixture) {
	t.Helper()
	if fixture.LockContended == nil || fixture.Canceled == nil || fixture.TimedOut == nil || fixture.Concurrent == nil {
		t.Fatal("operation fixture requires lock, cancellation, timeout, and concurrency factories")
	}

	t.Run("lock contention remains a failed apply", func(t *testing.T) {
		assertFailedApply(t, fixture.LockContended(t).Apply(context.Background()), nil)
	})

	t.Run("canceled context remains a failed apply", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assertFailedApply(t, fixture.Canceled(t).Apply(ctx), context.Canceled)
	})

	t.Run("expired deadline remains a failed apply", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
		defer cancel()
		assertFailedApply(t, fixture.TimedOut(t).Apply(ctx), context.DeadlineExceeded)
	})

	t.Run("concurrent applies mutate once", func(t *testing.T) {
		provider := fixture.Concurrent(t)
		start := make(chan struct{})
		results := make(chan contract.ApplyResult, 2)
		var group sync.WaitGroup
		for range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				results <- provider.Apply(context.Background())
			}()
		}
		close(start)
		group.Wait()
		close(results)

		changed, noChange := 0, 0
		for result := range results {
			switch result.Status {
			case contract.Changed:
				changed++
			case contract.NoChange:
				noChange++
			default:
				t.Fatalf("concurrent Apply = %+v, want changed or no-change", result)
			}
			if result.Err != nil {
				t.Fatalf("concurrent Apply error = %v", result.Err)
			}
		}
		if changed != 1 || noChange != 1 {
			t.Fatalf("concurrent results = %d changed, %d no-change; want one of each", changed, noChange)
		}
		assertStatus(t, provider.Check(context.Background()), contract.Compliant)
	})
}

// RunActivation verifies that a provider's activation boundary preserves its
// requested ordering while deduplicating repeated activation work.
func RunActivation(t *testing.T, fixture ActivationFixture) {
	t.Helper()
	if fixture.Activator == nil {
		t.Fatal("activation fixture requires an activator")
	}
	activated, err := fixture.Activator(t).Activate(context.Background(), fixture.Requested)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !slices.Equal(activated, fixture.Want) {
		t.Fatalf("activated = %v, want %v", activated, fixture.Want)
	}
}

// RunRedactionCanary proves that a provider diagnostic boundary removes a
// recognizable synthetic secret before any retained output is asserted.
func RunRedactionCanary(t *testing.T, fixture RedactionFixture) {
	t.Helper()
	if fixture.Redact == nil || strings.TrimSpace(fixture.Canary) == "" {
		t.Fatal("redaction fixture requires a redactor and synthetic canary")
	}
	diagnostic := "token=" + fixture.Canary + " provider=example"
	redacted := fixture.Redact(t, diagnostic)
	if strings.Contains(redacted, fixture.Canary) {
		t.Fatalf("redacted diagnostic leaked synthetic canary: %q", redacted)
	}
}

// RunRollback verifies the three observable rollback classes: completed,
// intentionally unavailable, and failed with a retained error.
func RunRollback(t *testing.T, fixture RollbackFixture) {
	t.Helper()
	if fixture.Reverted == nil || fixture.NoRollback == nil || fixture.Failure == nil {
		t.Fatal("rollback fixture requires reverted, no-rollback, and failure providers")
	}

	t.Run("rollback completes", func(t *testing.T) {
		result := fixture.Reverted(t).Rollback(context.Background())
		if result.Status != contract.Reverted || result.Err != nil {
			t.Fatalf("Rollback = %+v, want reverted without error", result)
		}
	})

	t.Run("documented no rollback is distinct", func(t *testing.T) {
		result := fixture.NoRollback(t).Rollback(context.Background())
		if result.Status != contract.NoRollback || result.Err != nil {
			t.Fatalf("Rollback = %+v, want no-rollback without error", result)
		}
	})

	t.Run("rollback failure is retained", func(t *testing.T) {
		result := fixture.Failure(t).Rollback(context.Background())
		if result.Status != contract.RollbackFailed || result.Err == nil {
			t.Fatalf("Rollback = %+v, want failed rollback with error", result)
		}
	})
}

func assertStatus(t *testing.T, observation contract.Observation, want contract.CheckStatus) {
	t.Helper()
	if observation.Status != want {
		t.Fatalf("Check status = %q, want %q (actual=%#v, err=%v)", observation.Status, want, observation.Actual, observation.Err)
	}
}

func assertApplyStatus(t *testing.T, result contract.ApplyResult, want contract.ApplyStatus) {
	t.Helper()
	if result.Status != want || result.Err != nil {
		t.Fatalf("Apply = %+v, want status %q without error", result, want)
	}
}

func assertFailedApply(t *testing.T, result contract.ApplyResult, want error) {
	t.Helper()
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("Apply = %+v, want failed result with an error", result)
	}
	if want != nil && !errors.Is(result.Err, want) {
		t.Fatalf("Apply error = %v, want %v", result.Err, want)
	}
}
