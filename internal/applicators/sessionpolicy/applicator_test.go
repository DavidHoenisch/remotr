package sessionpolicy_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/sessionpolicy"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-IUP-005: lock and idle policy are first-class fields that converge
// through the desktop provider instead of arbitrary commands.
func TestApplicator_convergesStructuredLockAndIdlePolicy(t *testing.T) {
	runner := newSessionRunner()
	home := filepath.Join(t.TempDir(), "alice")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	provider := sessionpolicy.New(models.SessionPolicyResource{
		Name: "workstation-session", Provider: models.DesktopSettingProviderGSettings,
		Selector:    models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice"}},
		LockEnabled: boolPointer(true), IdleTimeoutSeconds: uint32Pointer(300), LockDelaySeconds: uint32Pointer(5),
	}, runner)
	provider.ListUsers = func() ([]interactiveuser.Account, error) {
		return []interactiveuser.Account{{Username: "alice", UID: 1000, GID: 1000, HomeDir: home}}, nil
	}

	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v", check)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if len(result.Activation) != 1 || result.Activation[0].Kind != executor.ActivationLogoutRequired {
		t.Fatalf("ApplyResult() activation = %+v, want logout-required", result.Activation)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
}

// OS-AEC-098: changing only a default application requires the affected
// application to be restarted; it must not claim that a logout is required.
func TestApplicator_defaultApplicationChangeReportsApplicationRestart(t *testing.T) {
	runner := newSessionRunner()
	home := filepath.Join(t.TempDir(), "alice")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	provider := sessionpolicy.New(models.SessionPolicyResource{
		Name: "default-browser", Provider: models.DesktopSettingProviderGSettings,
		Selector:            models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice"}},
		DefaultApplications: map[string]string{"text/html": "browser.desktop"},
	}, runner)
	provider.ListUsers = func() ([]interactiveuser.Account, error) {
		return []interactiveuser.Account{{Username: "alice", UID: 1000, GID: 1000, HomeDir: home}}, nil
	}

	result := provider.ApplyResult(context.Background())
	want := []executor.ActivationSignal{{Kind: executor.ActivationApplicationRestart, Target: "browser.desktop"}}
	if result.Status != executor.Changed || !slices.Equal(result.Activation, want) {
		t.Fatalf("ApplyResult() = %+v, want changed with activation %+v", result, want)
	}
}

func TestApplicator_convergesProxyLockdownAndDefaultApplications(t *testing.T) {
	runner := newSessionRunner()
	home := filepath.Join(t.TempDir(), "alice")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	provider := sessionpolicy.New(models.SessionPolicyResource{
		Name: "workstation-session", Provider: models.DesktopSettingProviderGSettings,
		Selector:             models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice"}},
		Proxy:                &models.SessionProxyPolicy{Mode: models.SessionProxyManual, HTTPHost: "proxy.example.test", HTTPPort: 8080, IgnoreHosts: []string{"localhost"}},
		DisableUserSwitching: boolPointer(true), DisableLogout: boolPointer(true), DisableCommandLine: boolPointer(true),
		DefaultApplications: map[string]string{"text/html": "browser.desktop", "application/pdf": "viewer.desktop"},
	}, runner)
	provider.ListUsers = func() ([]interactiveuser.Account, error) {
		return []interactiveuser.Account{{Username: "alice", UID: 1000, GID: 1000, HomeDir: home}}, nil
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	if runner.applications["text/html"] != "browser.desktop" || runner.applications["application/pdf"] != "viewer.desktop" {
		t.Fatalf("default applications = %+v", runner.applications)
	}
	want := []string{"runuser", "-u", "alice", "--", "env", "HOME=" + home, "dbus-run-session", "--", "xdg-mime", "default", "browser.desktop", "text/html"}
	if !containsCall(runner.calls, want) {
		t.Fatalf("missing exact default-application command %q in %q", want, runner.calls)
	}
}

// OS-AEC-098: unsafe state belonging to one selected user is isolated without
// preventing a later safe user from receiving the declared default application.
func TestApplicator_rejectsUnsafeDefaultApplicationStateAndContinuesLaterUsers(t *testing.T) {
	root := t.TempDir()
	unsafeHome := filepath.Join(root, "alice")
	safeHome := filepath.Join(root, "bob")
	outside := filepath.Join(root, "outside")
	for _, path := range []string{unsafeHome, safeHome, outside} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(unsafeHome, ".config")); err != nil {
		t.Fatal(err)
	}
	runner := newPerUserSessionRunner()
	provider := sessionpolicy.New(models.SessionPolicyResource{
		Name: "default-browser", Provider: models.DesktopSettingProviderGSettings,
		Selector:            models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice", "bob"}},
		DefaultApplications: map[string]string{"text/html": "browser.desktop"},
	}, runner)
	provider.ListUsers = func() ([]interactiveuser.Account, error) {
		return []interactiveuser.Account{
			{Username: "alice", UID: 1000, GID: 1000, HomeDir: unsafeHome},
			{Username: "bob", UID: 1001, GID: 1001, HomeDir: safeHome},
		}, nil
	}

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "alice") {
		t.Fatalf("ApplyResult() = %+v, want isolated alice failure", result)
	}
	if got := runner.applications["bob"]["text/html"]; got != "browser.desktop" {
		t.Fatalf("bob default application = %q, want browser.desktop", got)
	}
	for _, call := range runner.calls {
		if len(call) > 2 && call[0] == "runuser" && call[2] == "alice" {
			t.Fatalf("unsafe user command was invoked: %q", call)
		}
	}
}

func boolPointer(value bool) *bool       { return &value }
func uint32Pointer(value uint32) *uint32 { return &value }

type sessionRunner struct {
	values       map[string]string
	applications map[string]string
	calls        [][]string
}

func newSessionRunner() *sessionRunner {
	return &sessionRunner{values: map[string]string{}, applications: map[string]string{}}
}

func (r *sessionRunner) Run(command string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{command}, args...))
	for i, arg := range args {
		switch arg {
		case "gsettings":
			if i+3 >= len(args) {
				continue
			}
			operation, schema, key := args[i+1], args[i+2], args[i+3]
			identity := schema + "/" + key
			switch operation {
			case "get":
				return []byte(r.values[identity] + "\n"), nil, nil
			case "set":
				if i+4 >= len(args) {
					return nil, nil, nil
				}
				r.values[identity] = args[i+4]
				return nil, nil, nil
			}
		case "xdg-mime":
			if i+3 >= len(args) {
				continue
			}
			switch args[i+1] {
			case "query":
				return []byte(r.applications[args[i+3]] + "\n"), nil, nil
			case "default":
				r.applications[args[i+3]] = args[i+2]
				return nil, nil, nil
			}
		}
	}
	return nil, nil, nil
}

func containsCall(calls [][]string, want []string) bool {
	for _, call := range calls {
		if slices.Equal(call, want) {
			return true
		}
	}
	return false
}

type perUserSessionRunner struct {
	applications map[string]map[string]string
	calls        [][]string
}

func newPerUserSessionRunner() *perUserSessionRunner {
	return &perUserSessionRunner{applications: map[string]map[string]string{}}
}

func (r *perUserSessionRunner) Run(command string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{command}, args...))
	if command != "runuser" || len(args) < 2 {
		return nil, nil, nil
	}
	username := args[1]
	if r.applications[username] == nil {
		r.applications[username] = map[string]string{}
	}
	for i, arg := range args {
		if arg != "xdg-mime" || i+3 >= len(args) {
			continue
		}
		mime := args[i+3]
		switch args[i+1] {
		case "query":
			return []byte(r.applications[username][mime] + "\n"), nil, nil
		case "default":
			r.applications[username][mime] = args[i+2]
			return nil, nil, nil
		}
	}
	return nil, nil, nil
}
