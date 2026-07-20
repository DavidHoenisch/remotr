package journald_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/journald"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func testPolicy() models.JournaldResource {
	return models.JournaldResource{
		Name:                  "retention",
		Storage:               models.JournaldStoragePersistent,
		MaxRetention:          "720h",
		SystemMaxUseBytes:     int64Pointer(1 << 30),
		RuntimeMaxUseBytes:    int64Pointer(256 << 20),
		RateLimitInterval:     "30s",
		RateLimitBurst:        intPointer(10000),
		ForwardToSyslog:       boolPointer(true),
		ForwardToKernelBuffer: boolPointer(false),
		ForwardToConsole:      boolPointer(false),
		ForwardToWall:         boolPointer(true),
	}
}

func TestApplicatorUsesExactSystemdAnalyzeValidationBoundary(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "journald.conf.d")
	mainConfig := filepath.Join(root, "journald.conf")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainConfig, []byte("[Journal]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &captureRunner{}
	provider := journald.New(testPolicy(), runner)
	provider.ConfigDir, provider.MainConfig = configDir, mainConfig
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if len(runner.calls) != 1 || runner.calls[0].Name != "systemd-analyze" || len(runner.calls[0].Args) != 3 ||
		!strings.HasPrefix(runner.calls[0].Args[0], "--root=") || runner.calls[0].Args[1] != "cat-config" || runner.calls[0].Args[2] != "systemd/journald.conf" {
		t.Fatalf("validation calls = %#v", runner.calls)
	}
}

// OS-LSM-029: changed structured limits are validated in an isolated complete
// journald tree before the active named drop-in changes, then request the
// journald service activation explicitly.
func TestApplicatorValidatesStructuredPolicyBeforeActivation(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "journald.conf.d")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(root, "journald.conf")
	if err := os.WriteFile(mainConfig, []byte("[Journal]\nStorage=auto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(configDir, "90-remotr-retention.conf")
	if err := os.WriteFile(activePath, []byte("[Journal]\nSystemMaxUse=1048576\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := journald.New(testPolicy(), nil)
	provider.ConfigDir, provider.MainConfig = configDir, mainConfig
	rollbackRoot := filepath.Join(root, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.ConfigureRollback(store, "base/journald-retention", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	provider.ValidateEffective = func(_ context.Context, stagedMain, stagedDir, stagedPath string) error {
		if stagedMain == mainConfig || stagedDir == configDir || stagedPath == activePath {
			t.Fatal("validation must use an isolated complete journald tree")
		}
		if _, err := os.Stat(stagedMain); err != nil {
			t.Fatalf("main journald config was not staged: %v", err)
		}
		content, err := os.ReadFile(stagedPath)
		if err != nil || !strings.Contains(string(content), "SystemMaxUse=1073741824") || !strings.Contains(string(content), "ForwardToSyslog=yes") {
			t.Fatalf("staged drop-in = %q err=%v", content, err)
		}
		return errors.New("systemd rejected staged journald policy")
	}

	if result := provider.ApplyResult(context.Background()); result.Status != executor.Failed || !strings.Contains(result.Err.Error(), "rejected") {
		t.Fatalf("invalid ApplyResult() = %+v", result)
	}
	assertFile(t, activePath, "[Journal]\nSystemMaxUse=1048576\n")

	provider.ValidateEffective = func(context.Context, string, string, string) error { return nil }
	result := provider.ApplyResult(context.Background())
	wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "systemd-journald.service"}}
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("valid ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := journald.New(testPolicy(), nil)
	restarted.ConfigDir, restarted.MainConfig = configDir, mainConfig
	if err := restarted.ConfigureRollback(restartedStore, "base/journald-retention", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFile(t, activePath, "[Journal]\nSystemMaxUse=1048576\n")
}

// OS-AEC-053: native cat-config composition alone does not reject malformed
// lines, so the complete staged main/drop-in tree must also pass syntax checks.
func TestApplicatorRejectsMalformedEffectiveConfigurationBeforeMutation(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "journald.conf.d")
	mainConfig := filepath.Join(root, "journald.conf")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainConfig, []byte("[Journal]\nStorage=auto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(configDir, "90-remotr-retention.conf")
	previous := "[Journal]\nSystemMaxUse=1048576\n"
	if err := os.WriteFile(activePath, []byte(previous), 0o644); err != nil {
		t.Fatal(err)
	}
	canary := testsupport.SecretCanary("journald-malformed-effective-tree")
	if err := os.WriteFile(filepath.Join(configDir, "80-invalid.conf"), []byte("[Journal]\n"+canary+" malformed line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &captureRunner{}
	provider := journald.New(testPolicy(), runner)
	provider.ConfigDir, provider.MainConfig = configDir, mainConfig
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("malformed effective configuration ApplyResult() = %+v, want failed", result)
	}
	if strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("malformed effective configuration leaked secret canary: %v", result.Err)
	}
	assertFile(t, activePath, previous)
}

func TestApplicatorValidatesCompleteSyntaxBoundaries(t *testing.T) {
	for _, test := range []struct {
		name        string
		mainConfig  []byte
		sibling     []byte
		wantFailure bool
	}{
		{name: "comments and empty reset", mainConfig: []byte("# stock comment\n; stock note\n[Journal]\nStorage=\n")},
		{name: "NUL", mainConfig: []byte("[Journal]\nStorage=auto\x00\n"), wantFailure: true},
		{name: "directive outside section", mainConfig: []byte("Storage=auto\n[Journal]\n"), wantFailure: true},
		{name: "invalid section", mainConfig: []byte("[Service]\nStorage=auto\n"), wantFailure: true},
		{name: "malformed key", mainConfig: []byte("[Journal]\n1Storage=auto\n"), wantFailure: true},
		{name: "malformed sibling", mainConfig: []byte("[Journal]\nStorage=auto\n"), sibling: []byte("[Journal]\nnot a directive\n"), wantFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			configDir := filepath.Join(root, "journald.conf.d")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			mainConfig := filepath.Join(root, "journald.conf")
			if err := os.WriteFile(mainConfig, test.mainConfig, 0o644); err != nil {
				t.Fatal(err)
			}
			activePath := filepath.Join(configDir, "90-remotr-retention.conf")
			previous := "[Journal]\nSystemMaxUse=1048576\n"
			if err := os.WriteFile(activePath, []byte(previous), 0o644); err != nil {
				t.Fatal(err)
			}
			if test.sibling != nil {
				if err := os.WriteFile(filepath.Join(configDir, "80-sibling.conf"), test.sibling, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			provider := journald.New(testPolicy(), &captureRunner{})
			provider.ConfigDir, provider.MainConfig = configDir, mainConfig
			result := provider.ApplyResult(context.Background())
			if test.wantFailure {
				if result.Status != executor.Failed || result.Err == nil {
					t.Fatalf("ApplyResult() = %+v, want failed", result)
				}
				assertFile(t, activePath, previous)
				return
			}
			if result.Status != executor.Changed {
				t.Fatalf("ApplyResult() = %+v, want changed", result)
			}
		})
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q err=%v, want %q", path, got, err, want)
	}
}

func boolPointer(value bool) *bool    { return &value }
func intPointer(value int) *int       { return &value }
func int64Pointer(value int64) *int64 { return &value }

type captureRunner struct{ calls []executil.MockCall }

func (r *captureRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	return nil, nil, nil
}
