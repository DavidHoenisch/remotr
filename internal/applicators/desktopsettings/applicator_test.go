package desktopsettings_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/desktopsettings"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-IUP-004: GVariant type is part of observed state; textual similarity
// must not make a string compliant with a requested boolean.
func TestApplicator_nativeBooleanDoesNotMatchString(t *testing.T) {
	home := filepath.Join(t.TempDir(), "alice")
	runner := &scriptedRunner{stdout: []byte("'true'\n")}
	provider := desktopsettings.New(models.DesktopSettingResource{
		Name: "animations", Provider: models.DesktopSettingProviderDconf, Scope: models.DesktopSettingScopeUser,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice"}},
		Path:     "/org/gnome/desktop/interface/enable-animations",
		Value:    models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true},
	}, runner)
	provider.ListUsers = func() ([]interactiveuser.Account, error) {
		return []interactiveuser.Account{{Username: "alice", UID: 1000, GID: 1000, HomeDir: home}}, nil
	}

	check := provider.Check(context.Background())
	if check.Status != executor.Drifted || len(check.Subresults) != 1 || check.Subresults[0].Status != executor.Drifted {
		t.Fatalf("Check() = %+v, want native-type drift", check)
	}
}

// OS-IUP-002: both user providers persist through a transient private bus and
// do not require a pre-existing graphical login session.
func TestApplicator_appliesForLoggedOutUserWithExactArgv(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider models.DesktopSettingProvider
		path     string
		schema   string
		key      string
	}{
		{name: "dconf", provider: models.DesktopSettingProviderDconf, path: "/org/gnome/desktop/interface/enable-animations"},
		{name: "gsettings", provider: models.DesktopSettingProviderGSettings, schema: "org.gnome.desktop.interface", key: "enable-animations"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "alice")
			runner := &statefulRunner{value: "false"}
			provider := desktopsettings.New(models.DesktopSettingResource{
				Name: "animations", Provider: test.provider, Scope: models.DesktopSettingScopeUser,
				Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice"}},
				Path:     test.path, Schema: test.schema, Key: test.key,
				Value: models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true},
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
			if runner.value != "true" || len(runner.calls) < 3 {
				t.Fatalf("runner state=%q calls=%+v", runner.value, runner.calls)
			}
			for _, call := range runner.calls {
				joined := strings.Join(call.Args, " ")
				if call.Name != "runuser" || !strings.Contains(joined, "-u alice -- env HOME="+home+" dbus-run-session --") || strings.Contains(joined, "DISPLAY=") {
					t.Fatalf("unsafe or session-dependent argv: %+v", call)
				}
			}
		})
	}
}

func TestApplicator_systemMandatoryDconfWritesOverrideAndLock(t *testing.T) {
	runner := &statefulRunner{}
	provider := desktopsettings.New(models.DesktopSettingResource{
		Name: "animations", Provider: models.DesktopSettingProviderDconf, Scope: models.DesktopSettingScopeSystem,
		Level:    models.DesktopSettingLevelMandatory,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		Path:     "/org/gnome/desktop/interface/enable-animations",
		Value:    models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: false},
	}, runner)
	provider.ConfigDir = filepath.Join(t.TempDir(), "remotr.d")
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	config, err := os.ReadFile(filepath.Join(provider.ConfigDir, "90-remotr-animations"))
	if err != nil || string(config) != "[org/gnome/desktop/interface]\nenable-animations=false\n" {
		t.Fatalf("config = %q err=%v", config, err)
	}
	lock, err := os.ReadFile(filepath.Join(provider.ConfigDir, "locks", "90-remotr-animations"))
	if err != nil || string(lock) != "/org/gnome/desktop/interface/enable-animations\n" {
		t.Fatalf("lock = %q err=%v", lock, err)
	}
	if len(runner.calls) != 1 || runner.calls[0].Name != "dconf" || strings.Join(runner.calls[0].Args, " ") != "update" {
		t.Fatalf("calls = %+v", runner.calls)
	}
}

func TestApplicator_authoritativeSelectorCleansPreviouslyOwnedDepartedUser(t *testing.T) {
	root := t.TempDir()
	accounts := []interactiveuser.Account{
		{Username: "alice", UID: 1000, GID: 1000, HomeDir: filepath.Join(root, "alice")},
		{Username: "bob", UID: 1001, GID: 1001, HomeDir: filepath.Join(root, "bob")},
	}
	runner := &perUserSettingRunner{values: map[string]string{"alice": "false", "bob": "false"}}
	resource := models.DesktopSettingResource{
		ResourceMeta: models.ResourceMeta{Ownership: models.OwnershipAuthoritative},
		Name:         "animations", Provider: models.DesktopSettingProviderGSettings, Scope: models.DesktopSettingScopeUser,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice", "bob"}},
		Schema:   "org.gnome.desktop.interface", Key: "enable-animations",
		Value: models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true},
	}
	first := desktopsettings.New(resource, runner)
	first.StateDir, first.StateKey = root, "workstation/animations"
	first.ListUsers = func() ([]interactiveuser.Account, error) { return accounts, nil }
	if result := first.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("initial ApplyResult() = %+v", result)
	}

	resource.Selector.Usernames = []string{"alice"}
	second := desktopsettings.New(resource, runner)
	second.StateDir, second.StateKey = root, "workstation/animations"
	second.ListUsers = func() ([]interactiveuser.Account, error) { return accounts, nil }
	if check := second.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("selector transition Check() = %+v", check)
	}
	if result := second.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("cleanup ApplyResult() = %+v", result)
	}
	if runner.values["alice"] != "true" || runner.values["bob"] != "false" {
		t.Fatalf("per-user values after cleanup = %+v", runner.values)
	}
	if check := second.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
}

func TestInteractivePolicyIntegration_loggedInAndLoggedOutUsersConverge(t *testing.T) {
	root := t.TempDir()
	accounts := []interactiveuser.Account{
		{Username: "alice", UID: 1000, GID: 1000, HomeDir: filepath.Join(root, "active-session-alice")},
		{Username: "bob", UID: 1001, GID: 1001, HomeDir: filepath.Join(root, "logged-out-bob")},
	}
	runner := &perUserSettingRunner{values: map[string]string{"alice": "false", "bob": "false"}}
	provider := desktopsettings.New(models.DesktopSettingResource{
		Name: "animations", Provider: models.DesktopSettingProviderGSettings, Scope: models.DesktopSettingScopeUser,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		Schema:   "org.gnome.desktop.interface", Key: "enable-animations",
		Value: models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true},
	}, runner)
	provider.ListUsers = func() ([]interactiveuser.Account, error) { return accounts, nil }
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant || len(check.Subresults) != 2 {
		t.Fatalf("second Check() = %+v", check)
	}
	for _, username := range []string{"alice", "bob"} {
		found := false
		for _, call := range runner.calls {
			joined := strings.Join(call.Args, " ")
			if strings.Contains(joined, "-u "+username+" --") && strings.Contains(joined, "dbus-run-session -- gsettings") {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing transient persistent-policy call for %s: %+v", username, runner.calls)
		}
	}
}

func TestInteractivePolicyIntegration_oneUserFailureAggregates(t *testing.T) {
	root := t.TempDir()
	accounts := []interactiveuser.Account{
		{Username: "alice", UID: 1000, GID: 1000, HomeDir: filepath.Join(root, "alice")},
		{Username: "bob", UID: 1001, GID: 1001, HomeDir: filepath.Join(root, "bob")},
		{Username: "carol", UID: 1002, GID: 1002, HomeDir: filepath.Join(root, "carol")},
	}
	runner := &perUserSettingRunner{
		values:   map[string]string{"alice": "true", "bob": "true", "carol": "true"},
		failures: map[string]error{"carol": errors.New("session bus unavailable")},
	}
	provider := desktopsettings.New(models.DesktopSettingResource{
		Name: "animations", Provider: models.DesktopSettingProviderGSettings, Scope: models.DesktopSettingScopeUser,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		Schema:   "org.gnome.desktop.interface", Key: "enable-animations",
		Value: models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true},
	}, runner)
	provider.ListUsers = func() ([]interactiveuser.Account, error) { return accounts, nil }
	check := provider.Check(context.Background())
	if check.Status != executor.CheckFailed || len(check.Subresults) != 3 {
		t.Fatalf("Check() = %+v", check)
	}
	if check.Subresults[0].Status != executor.Compliant || check.Subresults[1].Status != executor.Compliant || check.Subresults[2].Target != "carol" || check.Subresults[2].Status != executor.CheckFailed {
		t.Fatalf("subresults = %+v", check.Subresults)
	}
}

type scriptedRunner struct{ stdout []byte }

func (r *scriptedRunner) Run(string, ...string) ([]byte, []byte, error) { return r.stdout, nil, nil }

type statefulRunner struct {
	value string
	calls []executil.MockCall
}

type perUserSettingRunner struct {
	values   map[string]string
	failures map[string]error
	calls    []executil.MockCall
}

func (r *perUserSettingRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	user := ""
	for i, arg := range args {
		if arg == "-u" && i+1 < len(args) {
			user = args[i+1]
		}
	}
	for _, arg := range args {
		switch arg {
		case "get", "read":
			if err := r.failures[user]; err != nil {
				return nil, nil, err
			}
			return []byte(r.values[user] + "\n"), nil, nil
		case "set", "write":
			r.values[user] = args[len(args)-1]
			return nil, nil, nil
		case "reset":
			r.values[user] = "false"
			return nil, nil, nil
		}
	}
	return nil, nil, nil
}

func (r *statefulRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	if name == "runuser" {
		for _, arg := range args {
			switch arg {
			case "read", "get":
				return []byte(r.value + "\n"), nil, nil
			case "write", "set":
				r.value = args[len(args)-1]
				return nil, nil, nil
			}
		}
	}
	return nil, nil, nil
}
