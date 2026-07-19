package apparmor_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/apparmor"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-LSM-003: parser validation occurs against a same-directory stage before
// the active profile file or loaded mode changes.
func TestApplicatorRejectsInvalidStagedProfileBeforeActivation(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "remotr-service")
	if err := os.WriteFile(activePath, []byte("profile service { /old r, }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	applicator := apparmor.New(models.AppArmorProfileResource{
		Name: "service", Profile: "service", Content: "profile service { invalid syntax }\n", Mode: models.AppArmorComplain,
	}, runner)
	applicator.ProfilesDir = dir
	applicator.DisableDir = filepath.Join(dir, "disable")
	applicator.ObserveMode = func(context.Context, string) (models.AppArmorMode, error) { return models.AppArmorEnforce, nil }
	applicator.ValidateStage = func(_ context.Context, staged string) error {
		_, _, _ = runner.Run("apparmor_parser", "-Q", "-T", staged)
		return errors.New("parser rejected staged profile")
	}

	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.RollbackClass != executor.RollbackNone || result.Err == nil || !strings.Contains(result.Err.Error(), "parser rejected") {
		t.Fatalf("ApplyResult() = %+v, want failed with no rollback", result)
	}
	got, readErr := os.ReadFile(activePath)
	if readErr != nil || string(got) != "profile service { /old r, }\n" {
		t.Fatalf("active profile = %q err=%v", got, readErr)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "apparmor_parser" || !slices.Equal(runner.Calls[0].Args[:2], []string{"-Q", "-T"}) || runner.Calls[0].Args[2] == activePath {
		t.Fatalf("parser calls = %#v", runner.Calls)
	}
}

func TestApplicatorConvergesEnforceComplainAndDisabledWithExactParserArgv(t *testing.T) {
	for _, test := range []struct {
		mode models.AppArmorMode
		argv []string
	}{
		{mode: models.AppArmorEnforce, argv: []string{"-r", "-W"}},
		{mode: models.AppArmorComplain, argv: []string{"-r", "-W", "-C"}},
		{mode: models.AppArmorDisabled, argv: []string{"-R"}},
	} {
		t.Run(string(test.mode), func(t *testing.T) {
			dir := t.TempDir()
			activePath := filepath.Join(dir, "remotr-service")
			if err := os.WriteFile(activePath, []byte("profile service { /old r, }\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			current := models.AppArmorEnforce
			runner := &modeRunner{mode: &current, desired: test.mode}
			applicator := apparmor.New(models.AppArmorProfileResource{
				Name: "service", Profile: "service", Content: "profile service { /new r, }\n", Mode: test.mode,
			}, runner)
			applicator.ProfilesDir = dir
			applicator.DisableDir = filepath.Join(dir, "disable")
			applicator.ObserveMode = func(context.Context, string) (models.AppArmorMode, error) { return current, nil }

			if result := applicator.ApplyResult(context.Background()); result.Status != executor.Changed || result.RollbackClass != executor.RollbackNone {
				t.Fatalf("ApplyResult() = %+v", result)
			}
			if check := applicator.Check(context.Background()); check.Status != "compliant" {
				t.Fatalf("second Check() = %+v", check)
			}
			if result := applicator.ApplyResult(context.Background()); result.Status != executor.NoChange || result.RollbackClass != executor.RollbackNone {
				t.Fatalf("idempotent ApplyResult() = %+v", result)
			}
			if len(runner.calls) != 2 || runner.calls[0].Name != "apparmor_parser" || !slices.Equal(runner.calls[0].Args[:2], []string{"-Q", "-T"}) {
				t.Fatalf("parser calls = %#v", runner.calls)
			}
			wantActivation := append(append([]string(nil), test.argv...), activePath)
			if runner.calls[1].Name != "apparmor_parser" || !slices.Equal(runner.calls[1].Args, wantActivation) {
				t.Fatalf("activation argv = %#v, want %#v", runner.calls[1], wantActivation)
			}
		})
	}
}

type modeRunner struct {
	mode    *models.AppArmorMode
	desired models.AppArmorMode
	calls   []executil.MockCall
}

func (r *modeRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	if len(args) > 0 && args[0] != "-Q" {
		*r.mode = r.desired
	}
	return nil, nil, nil
}
