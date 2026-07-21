//go:build vmsafety

package desktopsettings_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/desktopsettings"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// TestDesktopSettingProviderVM exercises both registered desktopSetting rows
// on pinned Ubuntu 24.04. It covers logged-in and logged-out users, every
// advertised native type, authoritative cleanup, mandatory locks, malicious home symlink
// rejection with one-user failure isolation, preservation, and a second Check.
func TestDesktopSettingProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("desktop-setting VM provider test must run as root")
	}
	vmAssertDesktopUbuntu2404(t)

	ctx := context.Background()
	accounts := vmDesktopAccounts(t)
	stateDir := filepath.Join(t.TempDir(), "selector-state")
	vmResetDesktopQualificationState(t, accounts)
	t.Cleanup(func() { vmResetDesktopQualificationState(t, accounts) })

	stopLiveSession := vmStartDesktopLiveSession(t, accounts[0])
	vmWriteDesktopValue(t, accounts[1], models.DesktopSettingProviderDconf, "/com/remotr/qualification-preserved/value", "", "", "'preserve'")

	values := []struct {
		name     string
		value    models.DesktopSettingValue
		expected string
	}{
		{name: "boolean", value: models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true}, expected: "true"},
		{name: "string", value: models.DesktopSettingValue{Type: models.DesktopValueString, Value: "qualified"}, expected: "'qualified'"},
		{name: "int32", value: models.DesktopSettingValue{Type: models.DesktopValueInt32, Value: int64(42)}, expected: "42"},
		{name: "int64", value: models.DesktopSettingValue{Type: models.DesktopValueInt64, Value: int64(4_200_000_000)}, expected: "int64 4200000000"},
		{name: "uint32", value: models.DesktopSettingValue{Type: models.DesktopValueUint32, Value: int64(4_000_000_000)}, expected: "uint32 4000000000"},
		{name: "double", value: models.DesktopSettingValue{Type: models.DesktopValueDouble, Value: 1.5}, expected: "1.5"},
		{name: "string-list", value: models.DesktopSettingValue{Type: models.DesktopValueStringList, Value: []string{"alpha", "beta"}}, expected: "['alpha', 'beta']"},
	}
	for _, backend := range []models.DesktopSettingProvider{models.DesktopSettingProviderDconf, models.DesktopSettingProviderGSettings} {
		for _, test := range values {
			t.Run(string(backend)+"/"+test.name, func(t *testing.T) {
				resource := vmDesktopResource(backend, "vm-"+string(backend)+"-"+test.name, test.name, test.value, accounts)
				provider := vmRegisteredDesktopProvider(t, resource, stateDir, "m5-desktop/"+resource.Name)
				if check := provider.Check(ctx); check.Status != contract.Drifted || len(check.Subresults) != len(accounts) {
					t.Fatalf("initial %s/%s Check = %+v, want per-user drift", backend, test.name, check)
				}
				if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
					t.Fatalf("%s/%s Apply = %+v, want changed", backend, test.name, result)
				}
				if check := provider.Check(ctx); check.Status != contract.Compliant || len(check.Subresults) != len(accounts) {
					t.Fatalf("%s/%s second Check = %+v, want compliant", backend, test.name, check)
				}
				if result := provider.Apply(ctx); result.Status != contract.NoChange || result.Err != nil {
					t.Fatalf("%s/%s second Apply = %+v, want no change", backend, test.name, result)
				}
				for _, account := range accounts {
					if got := vmReadDesktopResource(t, account, resource); got != test.expected {
						t.Fatalf("%s %s/%s native value = %q, want %q", account.Username, backend, test.name, got, test.expected)
					}
				}
			})
		}
	}

	for _, backend := range []models.DesktopSettingProvider{models.DesktopSettingProviderDconf, models.DesktopSettingProviderGSettings} {
		t.Run(string(backend)+"/authoritative-cleanup", func(t *testing.T) {
			key := "cleanup-" + string(backend)
			resource := vmDesktopResource(backend, "vm-"+key, key, models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true}, accounts)
			resource.Ownership = models.OwnershipAuthoritative
			address := "m5-desktop/" + resource.Name
			provider := vmRegisteredDesktopProvider(t, resource, stateDir, address)
			if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
				t.Fatalf("initial authoritative %s Apply = %+v", backend, result)
			}

			resource.Selector.Usernames = []string{accounts[0].Username}
			provider = vmRegisteredDesktopProvider(t, resource, stateDir, address)
			if check := provider.Check(ctx); check.Status != contract.Drifted {
				t.Fatalf("%s selector cleanup Check = %+v, want drifted", backend, check)
			}
			if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
				t.Fatalf("%s selector cleanup Apply = %+v", backend, result)
			}
			if check := provider.Check(ctx); check.Status != contract.Compliant {
				t.Fatalf("%s selector cleanup second Check = %+v", backend, check)
			}
			if got := vmReadDesktopResource(t, accounts[0], resource); got != "true" {
				t.Fatalf("selected %s value = %q, want true", backend, got)
			}
			if got := vmReadDesktopResource(t, accounts[1], resource); got != "" && got != "false" {
				t.Fatalf("departed %s value = %q, want reset", backend, got)
			}
		})
	}

	vmTestMandatoryDesktopSetting(t, ctx, accounts, stateDir)
	stopLiveSession()
	for _, backend := range []models.DesktopSettingProvider{models.DesktopSettingProviderDconf, models.DesktopSettingProviderGSettings} {
		vmTestDesktopMaliciousHomeIsolation(t, ctx, backend, accounts, stateDir)
	}

	if got := vmReadDesktopValue(t, accounts[1], models.DesktopSettingProviderDconf, "/com/remotr/qualification-preserved/value", "", ""); got != "'preserve'" {
		t.Fatalf("unmanaged desktop setting changed: %q", got)
	}
}

func vmDesktopResource(backend models.DesktopSettingProvider, name, key string, value models.DesktopSettingValue, accounts []interactiveuser.Account) models.DesktopSettingResource {
	resource := models.DesktopSettingResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
		Name:         name, Provider: backend, Scope: models.DesktopSettingScopeUser,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{accounts[0].Username, accounts[1].Username}},
		Value:    value,
	}
	if backend == models.DesktopSettingProviderDconf {
		resource.Path = "/com/remotr/qualification-dconf/" + key
	} else {
		resource.Schema, resource.Key = "com.remotr.Qualification", key
	}
	return resource
}

func vmTestMandatoryDesktopSetting(t *testing.T, ctx context.Context, accounts []interactiveuser.Account, stateDir string) {
	t.Helper()
	for _, account := range accounts {
		vmWriteDesktopValue(t, account, models.DesktopSettingProviderGSettings, "", "com.remotr.Qualification", "mandatory", "true")
	}
	resource := models.DesktopSettingResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-mandatory", Provider: models.DesktopSettingProviderDconf, Scope: models.DesktopSettingScopeSystem,
		Level:    models.DesktopSettingLevelMandatory,
		Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		Path:     "/com/remotr/qualification/mandatory",
		Value:    models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: false},
	}
	provider := vmRegisteredDesktopProvider(t, resource, stateDir, "m5-desktop/vm-mandatory")
	if check := provider.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("mandatory system Check = %+v, want drifted", check)
	}
	if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("mandatory system Apply = %+v", result)
	}
	if check := provider.Check(ctx); check.Status != contract.Compliant {
		t.Fatalf("mandatory system second Check = %+v", check)
	}
	for _, account := range accounts {
		if got := vmReadDesktopValue(t, account, models.DesktopSettingProviderGSettings, "", "com.remotr.Qualification", "mandatory"); got != "false" {
			t.Fatalf("%s effective mandatory value = %q, want false", account.Username, got)
		}
		if got := vmRunDesktop(t, account, "gsettings", "writable", "com.remotr.Qualification", "mandatory"); got != "false" {
			t.Fatalf("%s mandatory writable = %q, want false", account.Username, got)
		}
	}

	resource.Level = models.DesktopSettingLevelDefault
	provider = vmRegisteredDesktopProvider(t, resource, stateDir, "m5-desktop/vm-mandatory")
	if check := provider.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("default transition Check = %+v, want stale-lock drift", check)
	}
	if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("default transition Apply = %+v", result)
	}
	if check := provider.Check(ctx); check.Status != contract.Compliant {
		t.Fatalf("default transition second Check = %+v", check)
	}
	for _, account := range accounts {
		if got := vmRunDesktop(t, account, "gsettings", "writable", "com.remotr.Qualification", "mandatory"); got != "true" {
			t.Fatalf("%s default writable = %q, want true", account.Username, got)
		}
		if got := vmReadDesktopValue(t, account, models.DesktopSettingProviderGSettings, "", "com.remotr.Qualification", "mandatory"); got != "true" {
			t.Fatalf("%s user value after default transition = %q, want preserved true", account.Username, got)
		}
	}
}

func vmTestDesktopMaliciousHomeIsolation(t *testing.T, ctx context.Context, backend models.DesktopSettingProvider, accounts []interactiveuser.Account, stateDir string) {
	t.Helper()
	unsafeAccount, safeAccount := accounts[0], accounts[1]
	configPath := filepath.Join(unsafeAccount.HomeDir, ".config")
	backupPath := filepath.Join(unsafeAccount.HomeDir, ".config.remotr-backup")
	external := t.TempDir()
	if err := os.Rename(configPath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, configPath); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		_ = os.Remove(configPath)
		_ = os.Rename(backupPath, configPath)
	}
	defer restore()

	key := "malicious-" + string(backend)
	resource := vmDesktopResource(backend, "vm-"+key, key, models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: true}, accounts)
	provider := vmRegisteredDesktopProvider(t, resource, stateDir, "m5-desktop/"+resource.Name)
	result := provider.Apply(ctx)
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), unsafeAccount.Username) {
		t.Fatalf("%s malicious-home Apply = %+v, want aggregate unsafe-user failure", backend, result)
	}
	if got := vmReadDesktopResource(t, safeAccount, resource); got != "true" {
		t.Fatalf("%s safe user after malicious home = %q, want true", backend, got)
	}
	if entries, err := os.ReadDir(external); err != nil || len(entries) != 0 {
		t.Fatalf("%s malicious home symlink target changed: entries=%v err=%v", backend, entries, err)
	}
	restore()
	if check := provider.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("%s recovered home Check = %+v, want remaining-user drift", backend, check)
	}
	if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("%s recovered home Apply = %+v", backend, result)
	}
	if check := provider.Check(ctx); check.Status != contract.Compliant {
		t.Fatalf("%s recovered home second Check = %+v", backend, check)
	}
}

func vmRegisteredDesktopProvider(t *testing.T, resource models.DesktopSettingResource, stateDir, address string) contract.Provider {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{DesktopSettings: []models.DesktopSettingResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindDesktopSetting {
		t.Fatalf("desktop-setting registry resources = %+v, %v", resources, err)
	}
	hostFacts, err := facts.Read()
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{Facts: hostFacts, StateDir: stateDir, ResourceAddress: address})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := handler.(*desktopsettings.Applicator); !ok {
		t.Fatalf("desktop-setting registry provider = %T", handler)
	}
	provider, err := contract.New(handler)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmDesktopAccounts(t *testing.T) []interactiveuser.Account {
	t.Helper()
	all, err := interactiveuser.List()
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]struct{}{"remotr-desktop-a": {}, "remotr-desktop-b": {}}
	accounts := make([]interactiveuser.Account, 0, len(wanted))
	for _, account := range all {
		if _, ok := wanted[account.Username]; ok {
			accounts = append(accounts, account)
		}
	}
	if len(accounts) != 2 || accounts[0].Username != "remotr-desktop-a" || accounts[1].Username != "remotr-desktop-b" {
		t.Fatalf("desktop fixture accounts = %+v", accounts)
	}
	return accounts
}

func vmStartDesktopLiveSession(t *testing.T, account interactiveuser.Account) func() {
	t.Helper()
	ready := filepath.Join(account.HomeDir, ".remotr-provider-session-ready")
	stop := filepath.Join(account.HomeDir, ".remotr-provider-session-stop")
	_ = os.Remove(ready)
	_ = os.Remove(stop)
	var output bytes.Buffer
	command := exec.Command("runuser", "-u", account.Username, "--", "env", "HOME="+account.HomeDir, "dbus-run-session", "--", "sh", "-eu", "-c", `touch "$HOME/.remotr-provider-session-ready"; while test ! -e "$HOME/.remotr-provider-session-stop"; do sleep 0.05; done`)
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			t.Fatalf("live desktop session did not start: %s", output.String())
		}
		time.Sleep(25 * time.Millisecond)
	}
	stopped := false
	stopSession := func() {
		if stopped {
			return
		}
		stopped = true
		if err := os.WriteFile(stop, nil, 0o600); err != nil {
			t.Errorf("stop live desktop session: %v", err)
		}
		if err := command.Wait(); err != nil {
			t.Errorf("live desktop session: %v: %s", err, output.String())
		}
		_ = os.Remove(ready)
		_ = os.Remove(stop)
	}
	t.Cleanup(stopSession)
	return stopSession
}

func vmReadDesktopResource(t *testing.T, account interactiveuser.Account, resource models.DesktopSettingResource) string {
	t.Helper()
	return vmReadDesktopValue(t, account, resource.Provider, resource.Path, resource.Schema, resource.Key)
}

func vmReadDesktopValue(t *testing.T, account interactiveuser.Account, backend models.DesktopSettingProvider, path, schema, key string) string {
	t.Helper()
	if backend == models.DesktopSettingProviderDconf {
		return vmRunDesktop(t, account, "dconf", "read", path)
	}
	return vmRunDesktop(t, account, "gsettings", "get", schema, key)
}

func vmWriteDesktopValue(t *testing.T, account interactiveuser.Account, backend models.DesktopSettingProvider, path, schema, key, value string) {
	t.Helper()
	if backend == models.DesktopSettingProviderDconf {
		vmRunDesktop(t, account, "dconf", "write", path, value)
		return
	}
	vmRunDesktop(t, account, "gsettings", "set", schema, key, value)
}

func vmRunDesktop(t *testing.T, account interactiveuser.Account, command string, args ...string) string {
	t.Helper()
	commandArgs := []string{"-u", account.Username, "--", "env", "HOME=" + account.HomeDir, "dbus-run-session", "--", command}
	commandArgs = append(commandArgs, args...)
	output, err := exec.Command("runuser", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v for %s: %v: %s", command, args, account.Username, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vmResetDesktopQualificationState(t *testing.T, accounts []interactiveuser.Account) {
	t.Helper()
	for _, account := range accounts {
		for _, path := range []string{"/com/remotr/qualification-dconf/", "/com/remotr/qualification-preserved/"} {
			vmRunDesktop(t, account, "dconf", "reset", "-f", path)
		}
		vmRunDesktop(t, account, "gsettings", "reset-recursively", "com.remotr.Qualification")
	}
	for _, root := range []string{"/etc/dconf/db/local.d", "/etc/dconf/db/remotr.d"} {
		for _, path := range []string{filepath.Join(root, "90-remotr-vm-mandatory"), filepath.Join(root, "locks", "90-remotr-vm-mandatory")} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Errorf("remove %s: %v", path, err)
			}
		}
	}
	if output, err := exec.Command("dconf", "update").CombinedOutput(); err != nil {
		t.Errorf("dconf update cleanup: %v: %s", err, output)
	}
}

func vmAssertDesktopUbuntu2404(t *testing.T) {
	t.Helper()
	hostFacts, err := facts.Read()
	if err != nil {
		t.Fatal(err)
	}
	if hostFacts.Distro != types.Ubuntu || hostFacts.DistroVersion != "24.04" || hostFacts.Arch != types.X86 {
		t.Fatalf("desktop VM facts = %+v", hostFacts)
	}
	if !containsDesktopBackend(hostFacts.Desktop, facts.DesktopDconf) || !containsDesktopBackend(hostFacts.Desktop, facts.DesktopGSettings) {
		t.Fatalf("desktop VM backends = %+v", hostFacts.Desktop)
	}
	if output, err := exec.Command("gsettings", "range", "com.remotr.Qualification", "string-list").CombinedOutput(); err != nil || !strings.Contains(string(output), "type as") {
		t.Fatalf("qualification schema unavailable: %v: %s", err, output)
	}
}

func containsDesktopBackend(backends []facts.DesktopBackend, wanted facts.DesktopBackend) bool {
	for _, backend := range backends {
		if backend == wanted {
			return true
		}
	}
	return false
}
