package logrotate_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/logrotate"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func testPolicy() models.LogrotateResource {
	return models.LogrotateResource{
		Name:       "remotr-agent",
		Paths:      []string{"/var/log/remotr/*.log"},
		Cadence:    models.LogrotateDaily,
		Retention:  intPointer(14),
		Compress:   boolPointer(true),
		Create:     &models.LogrotateCreate{Mode: "0640", Owner: "root", Group: "adm"},
		Shared:     boolPointer(true),
		PostRotate: &models.LogrotateScript{Command: []string{"/usr/bin/systemctl", "reload", "remotr-agent.service"}},
	}
}

func TestApplicatorUsesExactDebugValidationAndShellQuotesScriptArgv(t *testing.T) {
	root := t.TempDir()
	fragmentsDir := filepath.Join(root, "logrotate.d")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(root, "logrotate.conf")
	if err := os.WriteFile(mainConfig, []byte("include "+fragmentsDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := testPolicy()
	policy.PostRotate.Command = []string{"/usr/bin/logger", "it's rotated"}
	runner := &captureRunner{}
	provider := logrotate.New(policy, runner)
	provider.FragmentsDir, provider.MainConfig = fragmentsDir, mainConfig
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if len(runner.calls) != 1 || runner.calls[0].Name != "logrotate" || len(runner.calls[0].Args) != 2 ||
		runner.calls[0].Args[0] != "--debug" || runner.calls[0].Args[1] == mainConfig {
		t.Fatalf("validation calls = %#v", runner.calls)
	}
	content, err := os.ReadFile(filepath.Join(fragmentsDir, "remotr-remotr-agent"))
	if err != nil || !strings.Contains(string(content), `'it'"'"'s rotated'`) {
		t.Fatalf("shell-quoted fragment = %q err=%v", content, err)
	}
}

// OS-AEC-054: a declared log path may be installed before its producer writes
// the first matching file; that operational absence must not invalidate the
// otherwise complete staged configuration.
func TestApplicatorAllowsUnmaterializedLogPaths(t *testing.T) {
	root := t.TempDir()
	fragmentsDir := filepath.Join(root, "logrotate.d")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(root, "logrotate.conf")
	if err := os.WriteFile(mainConfig, []byte("include "+fragmentsDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := logrotate.New(testPolicy(), &captureRunner{})
	provider.FragmentsDir, provider.MainConfig = fragmentsDir, mainConfig
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v, want changed", result)
	}
	content, err := os.ReadFile(filepath.Join(fragmentsDir, "remotr-remotr-agent"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "\n  missingok\n") {
		t.Fatalf("unmaterialized-path fragment = %q, want missingok", content)
	}
}

func TestApplicatorRedactsNativeValidationDiagnosticsBeforeMutation(t *testing.T) {
	root := t.TempDir()
	fragmentsDir := filepath.Join(root, "logrotate.d")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(root, "logrotate.conf")
	if err := os.WriteFile(mainConfig, []byte("include "+fragmentsDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(fragmentsDir, "remotr-remotr-agent")
	previous := "/var/log/remotr/*.log {\n  daily\n  rotate 1\n  missingok\n}\n"
	if err := os.WriteFile(activePath, []byte(previous), 0o644); err != nil {
		t.Fatal(err)
	}
	canary := testsupport.SecretCanary("logrotate-native-diagnostic")
	provider := logrotate.New(testPolicy(), &failingRunner{stderr: []byte(canary + " invalid policy")})
	provider.FragmentsDir, provider.MainConfig = fragmentsDir, mainConfig
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("ApplyResult() = %+v, want failed", result)
	}
	if strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("native validation diagnostic leaked secret canary: %v", result.Err)
	}
	assertFile(t, activePath, previous)
}

// OS-LSM-030: candidate fragments are validated as part of an isolated copy
// of the complete logrotate configuration before active state changes.
func TestApplicatorRejectsInvalidFullConfigurationBeforeActivation(t *testing.T) {
	root := t.TempDir()
	fragmentsDir := filepath.Join(root, "logrotate.d")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(root, "logrotate.conf")
	if err := os.WriteFile(mainConfig, []byte("weekly\ninclude "+fragmentsDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(fragmentsDir, "remotr-remotr-agent")
	if err := os.WriteFile(activePath, []byte("/var/log/remotr/*.log {\n  rotate 1\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := logrotate.New(testPolicy(), nil)
	provider.FragmentsDir, provider.MainConfig = fragmentsDir, mainConfig
	rollbackRoot := filepath.Join(root, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.ConfigureRollback(store, "base/logrotate-agent", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	provider.ValidateEffective = func(_ context.Context, stagedMain, stagedDir, stagedPath string) error {
		if stagedMain == mainConfig || stagedDir == fragmentsDir || stagedPath == activePath {
			t.Fatal("full-config validation must use an isolated staged tree")
		}
		main, err := os.ReadFile(stagedMain)
		if err != nil || !strings.Contains(string(main), "include "+stagedDir) {
			t.Fatalf("staged main config = %q err=%v", main, err)
		}
		fragment, err := os.ReadFile(stagedPath)
		if err != nil || !strings.Contains(string(fragment), "create 0640 root adm") ||
			!strings.Contains(string(fragment), "'/usr/bin/systemctl' 'reload' 'remotr-agent.service'") {
			t.Fatalf("staged fragment = %q err=%v", fragment, err)
		}
		return errors.New("logrotate rejected effective configuration")
	}

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "rejected") {
		t.Fatalf("invalid ApplyResult() = %+v", result)
	}
	assertFile(t, activePath, "/var/log/remotr/*.log {\n  rotate 1\n}\n")

	provider.ValidateEffective = func(context.Context, string, string, string) error { return nil }
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || len(result.Activation) != 0 {
		t.Fatalf("valid ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := logrotate.New(testPolicy(), nil)
	restarted.FragmentsDir, restarted.MainConfig = fragmentsDir, mainConfig
	if err := restarted.ConfigureRollback(restartedStore, "base/logrotate-agent", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFile(t, activePath, "/var/log/remotr/*.log {\n  rotate 1\n}\n")
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q err=%v, want %q", path, got, err, want)
	}
}

func boolPointer(value bool) *bool { return &value }
func intPointer(value int) *int    { return &value }

type captureRunner struct{ calls []executil.MockCall }

func (r *captureRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	return nil, nil, nil
}

type failingRunner struct{ stderr []byte }

func (r *failingRunner) Run(string, ...string) ([]byte, []byte, error) {
	return nil, r.stderr, errors.New("native validation failed")
}
