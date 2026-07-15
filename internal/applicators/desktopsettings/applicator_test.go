package desktopsettings_test

import (
	"context"
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

type scriptedRunner struct{ stdout []byte }

func (r *scriptedRunner) Run(string, ...string) ([]byte, []byte, error) { return r.stdout, nil, nil }

type statefulRunner struct {
	value string
	calls []executil.MockCall
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
