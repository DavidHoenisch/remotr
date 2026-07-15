package auditrules_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/auditrules"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-LSM-004: immutable audit mode accepts validated persistent state but
// refuses a live reload and reports that reboot is required.
func TestApplicatorPersistsValidatedRulesWithoutLiveReloadWhenImmutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "remotr-process.rules")
	if err := os.WriteFile(path, []byte("-w /old -p wa -k old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	applicator := auditrules.New(models.AuditRulesResource{
		Name: "process", Rules: []string{"-w /etc/passwd -p wa -k identity", "-a always,exit -F arch=b64 -S execve -k process"},
	}, nil)
	applicator.RulesDir = dir
	applicator.ObserveImmutable = func(context.Context) (bool, error) { return true, nil }
	applicator.ObserveLoaded = func(context.Context, []string) (bool, error) { return false, nil }
	validated := false
	applicator.ValidateEffective = func(_ context.Context, staged string) error {
		validated = filepath.Dir(staged) == dir
		return nil
	}
	loaded := false
	applicator.LoadEffective = func(context.Context) error { loaded = true; return nil }

	result := applicator.ApplyResult(context.Background())
	if !validated || loaded {
		t.Fatalf("validated=%t loaded=%t", validated, loaded)
	}
	wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}
	if result.Status != executor.Changed || result.RebootRequired != executor.RebootRequired || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "-w /etc/passwd -p wa -k identity\n-a always,exit -F arch=b64 -S execve -k process\n" {
		t.Fatalf("persistent rules = %q err=%v", got, err)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Drifted || !slices.Contains([]string{string(check.ObservedSummary)}, "persistent=true loaded=false immutable=true") {
		t.Fatalf("immutable Check() = %+v", check)
	}
}

func TestApplicatorValidatesEffectiveRulesAndLoadsMutableStateWithExactArgv(t *testing.T) {
	dir := t.TempDir()
	rules := []string{"-w /etc/passwd -p wa -k identity"}
	runner := &auditRunner{desired: rules, loaded: []string{"-w /old -p wa -k old"}}
	applicator := auditrules.New(models.AuditRulesResource{Name: "identity", Rules: rules}, runner)
	applicator.RulesDir = dir
	if result := applicator.ApplyResult(context.Background()); result.Status != executor.Changed || result.RebootRequired != executor.RebootNotRequired {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	wantPrefix := []string{"auditctl [-l]", "auditctl [-s]", "augenrules [--check]", "auditctl [-s]", "augenrules [--load]"}
	if len(runner.calls) < len(wantPrefix) || !slices.Equal(runner.calls[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("command calls = %#v, want prefix %#v", runner.calls, wantPrefix)
	}
}

func TestApplicatorEffectiveValidationFailureLeavesPersistentAndLoadedStateUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "remotr-process.rules")
	previous := "-w /old -p wa -k old\n"
	if err := os.WriteFile(path, []byte(previous), 0o640); err != nil {
		t.Fatal(err)
	}
	applicator := auditrules.New(models.AuditRulesResource{Name: "process", Rules: []string{"-w /new -p wa -k new"}}, nil)
	applicator.RulesDir = dir
	applicator.ObserveImmutable = func(context.Context) (bool, error) { return false, nil }
	applicator.ObserveLoaded = func(context.Context, []string) (bool, error) { return false, nil }
	applicator.ValidateEffective = func(context.Context, string) error { return errors.New("effective rules invalid") }
	loaded := false
	applicator.LoadEffective = func(context.Context) error { loaded = true; return nil }
	if err := applicator.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "effective rules invalid") {
		t.Fatalf("Apply() error = %v", err)
	}
	if loaded {
		t.Fatal("invalid effective rules were loaded")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != previous {
		t.Fatalf("persistent rules = %q err=%v", got, err)
	}
}

type auditRunner struct {
	desired   []string
	loaded    []string
	immutable bool
	calls     []string
}

func (r *auditRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, fmt.Sprintf("%s %v", name, args))
	switch fmt.Sprintf("%s %v", name, args) {
	case "auditctl [-l]":
		return []byte(strings.Join(r.loaded, "\n") + "\n"), nil, nil
	case "auditctl [-s]":
		if r.immutable {
			return []byte("enabled 2\n"), nil, nil
		}
		return []byte("enabled 1\n"), nil, nil
	case "augenrules [--check]":
		return nil, nil, nil
	case "augenrules [--load]":
		r.loaded = append([]string(nil), r.desired...)
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected command")
	}
}
