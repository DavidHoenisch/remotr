package providercontract_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	adapter "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/test/providercontract"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func TestRunConvergence(t *testing.T) {
	providercontract.RunConvergence(t, providercontract.Fixture{
		Compliant: func(t *testing.T) adapter.Provider {
			t.Helper()
			return newProvider(t, true)
		},
		Drifted: func(t *testing.T) adapter.Provider {
			t.Helper()
			return newProvider(t, false)
		},
	})
}

func TestRunAbsence(t *testing.T) {
	providercontract.RunAbsence(t, providercontract.AbsenceFixture{
		Absent: func(t *testing.T) adapter.Provider {
			t.Helper()
			return newProvider(t, true)
		},
		Present: func(t *testing.T) adapter.Provider {
			t.Helper()
			return newProvider(t, false)
		},
	})
}

func TestRunNegativeChecks(t *testing.T) {
	probeFailure := errors.New("probe failed")
	validationFailure := errors.New("invalid configuration")
	providercontract.RunNegativeChecks(t, providercontract.NegativeFixture{
		Unsupported: func(t *testing.T) adapter.Provider {
			t.Helper()
			return staticProvider{observation: adapter.Observation{Status: adapter.Unsupported}}
		},
		ProbeFailure: func(t *testing.T) adapter.Provider {
			t.Helper()
			return staticProvider{observation: adapter.Observation{Status: adapter.CheckFailed, Err: probeFailure}}
		},
		Validate: func(t *testing.T) error {
			t.Helper()
			return validationFailure
		},
	})
}

func TestRunOperationSafety(t *testing.T) {
	lockContention := errors.New("provider lock is held")
	providercontract.RunOperationSafety(t, providercontract.OperationFixture{
		LockContended: func(t *testing.T) adapter.Provider {
			t.Helper()
			return staticProvider{apply: adapter.ApplyResult{Status: adapter.Failed, Err: lockContention}}
		},
		Canceled: func(t *testing.T) adapter.Provider {
			t.Helper()
			return contextProvider{}
		},
		TimedOut: func(t *testing.T) adapter.Provider {
			t.Helper()
			return contextProvider{}
		},
		Concurrent: func(t *testing.T) adapter.Provider {
			t.Helper()
			return &concurrentProvider{}
		},
	})
}

func TestRunActivation(t *testing.T) {
	providercontract.RunActivation(t, providercontract.ActivationFixture{
		Activator: func(t *testing.T) providercontract.Activator {
			t.Helper()
			return orderingActivator{}
		},
		Requested: []string{"files", "systemd", "files"},
		Want:      []string{"files", "systemd"},
	})
}

func TestRunRedactionCanary(t *testing.T) {
	providercontract.RunRedactionCanary(t, providercontract.RedactionFixture{
		Redact: func(t *testing.T, diagnostic string) string {
			t.Helper()
			key, value, _ := strings.Cut(diagnostic, "=")
			return key + "=[REDACTED:" + strconv.Itoa(len(value)) + "]"
		},
		Canary: testsupport.SecretCanary("provider-diagnostic"),
	})
}

func TestRunRollback(t *testing.T) {
	rollbackFailure := errors.New("rollback failed")
	providercontract.RunRollback(t, providercontract.RollbackFixture{
		Reverted: func(t *testing.T) adapter.Provider {
			t.Helper()
			return staticProvider{rollback: adapter.RollbackResult{Status: adapter.Reverted}}
		},
		NoRollback: func(t *testing.T) adapter.Provider {
			t.Helper()
			return staticProvider{rollback: adapter.RollbackResult{Status: adapter.NoRollback}}
		},
		Failure: func(t *testing.T) adapter.Provider {
			t.Helper()
			return staticProvider{rollback: adapter.RollbackResult{Status: adapter.RollbackFailed, Err: rollbackFailure}}
		},
	})
}

func newProvider(t *testing.T, compliant bool) adapter.Provider {
	t.Helper()
	provider, err := adapter.New(&statefulHandler{compliant: compliant})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type statefulHandler struct {
	compliant bool
}

func (*statefulHandler) Name() string        { return "stateful" }
func (*statefulHandler) Description() string { return "stateful provider" }
func (h *statefulHandler) State(context.Context) (any, bool) {
	return h.compliant, h.compliant
}
func (h *statefulHandler) Apply(context.Context) error {
	if h.compliant {
		return appErr.ErrStateAlreadyMet
	}
	h.compliant = true
	return nil
}
func (*statefulHandler) Revert(context.Context) error { return appErr.ErrNoOp }

type staticProvider struct {
	observation adapter.Observation
	apply       adapter.ApplyResult
	rollback    adapter.RollbackResult
}

func (p staticProvider) Name() string                                    { return "static" }
func (p staticProvider) Description() string                             { return "static provider" }
func (p staticProvider) Check(context.Context) adapter.Observation       { return p.observation }
func (p staticProvider) Apply(context.Context) adapter.ApplyResult       { return p.apply }
func (p staticProvider) Rollback(context.Context) adapter.RollbackResult { return p.rollback }

type contextProvider struct{}

func (contextProvider) Name() string        { return "context" }
func (contextProvider) Description() string { return "context-aware provider" }
func (contextProvider) Check(context.Context) adapter.Observation {
	return adapter.Observation{Status: adapter.Drifted}
}
func (contextProvider) Apply(ctx context.Context) adapter.ApplyResult {
	return adapter.ApplyResult{Status: adapter.Failed, Err: ctx.Err()}
}
func (contextProvider) Rollback(context.Context) adapter.RollbackResult {
	return adapter.RollbackResult{Status: adapter.NoRollback}
}

type concurrentProvider struct {
	mu        sync.Mutex
	compliant bool
}

func (*concurrentProvider) Name() string        { return "concurrent" }
func (*concurrentProvider) Description() string { return "concurrent provider" }
func (p *concurrentProvider) Check(context.Context) adapter.Observation {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.compliant {
		return adapter.Observation{Status: adapter.Compliant}
	}
	return adapter.Observation{Status: adapter.Drifted}
}
func (p *concurrentProvider) Apply(context.Context) adapter.ApplyResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.compliant {
		return adapter.ApplyResult{Status: adapter.NoChange}
	}
	p.compliant = true
	return adapter.ApplyResult{Status: adapter.Changed}
}
func (*concurrentProvider) Rollback(context.Context) adapter.RollbackResult {
	return adapter.RollbackResult{Status: adapter.NoRollback}
}

type orderingActivator struct{}

func (orderingActivator) Activate(_ context.Context, requested []string) ([]string, error) {
	seen := make(map[string]struct{}, len(requested))
	activated := make([]string, 0, len(requested))
	for _, name := range requested {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		activated = append(activated, name)
	}
	return activated, nil
}
