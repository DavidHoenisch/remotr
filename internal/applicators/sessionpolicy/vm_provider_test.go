//go:build vmsafety

package sessionpolicy_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/sessionpolicy"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// TestSessionPolicyProviderVM qualifies both registered sessionPolicy rows on
// pinned Ubuntu 24.04. It covers supported lock/idle/proxy/restriction fields,
// merge-only default applications, logout/application activation, logged-in
// and logged-out users, authoritative field cleanup, malicious home symlink
// isolation, preservation, idempotence, and a second Check.
func TestSessionPolicyProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("session-policy VM provider test must run as root")
	}
	vmAssertSessionUbuntu2404(t)
	ctx := context.Background()
	accounts := vmSessionAccounts(t)
	stateDir := filepath.Join(t.TempDir(), "selector-state")
	vmResetSessionQualificationState(t, accounts)
	t.Cleanup(func() { vmResetSessionQualificationState(t, accounts) })

	stopLiveSession := vmStartSessionLiveUser(t, accounts[0])
	for _, backend := range []models.DesktopSettingProvider{models.DesktopSettingProviderDconf, models.DesktopSettingProviderGSettings} {
		t.Run(string(backend)+"/supported-fields", func(t *testing.T) {
			vmResetSessionQualificationState(t, accounts)
			for _, account := range accounts {
				vmRunSessionTool(t, account, "gsettings", "set", "com.remotr.Qualification", "string", "'preserve'")
				vmRunSessionTool(t, account, "xdg-mime", "default", "remotr-viewer.desktop", "image/png")
			}
			resource := vmStructuredSessionResource(backend, accounts)
			provider := vmRegisteredSessionProvider(t, resource, stateDir, "m5-desktop/"+resource.Name)
			if check := provider.Check(ctx); check.Status != contract.Drifted || len(check.Subresults) != len(accounts) {
				t.Fatalf("initial %s Check = %+v, want per-user drift", backend, check)
			}
			wantActivation := []contract.ActivationSignal{
				{Kind: contract.ActivationLogoutRequired},
				{Kind: contract.ActivationApplicationRestart, Target: "remotr-browser.desktop"},
				{Kind: contract.ActivationApplicationRestart, Target: "remotr-viewer.desktop"},
			}
			if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil || !slices.Equal(result.Activation, wantActivation) {
				t.Fatalf("%s Apply = %+v, want changed with activation %+v", backend, result, wantActivation)
			}
			if check := provider.Check(ctx); check.Status != contract.Compliant || len(check.Subresults) != len(accounts) {
				t.Fatalf("%s second Check = %+v, want compliant", backend, check)
			}
			if result := provider.Apply(ctx); result.Status != contract.NoChange || result.Err != nil || len(result.Activation) != 0 {
				t.Fatalf("%s second Apply = %+v, want no change", backend, result)
			}
			for _, account := range accounts {
				vmAssertStructuredSessionState(t, account)
				if got := vmRunSessionTool(t, account, "gsettings", "get", "com.remotr.Qualification", "string"); got != "'preserve'" {
					t.Fatalf("%s unmanaged setting = %q, want preserved", account.Username, got)
				}
				if got := vmRunSessionTool(t, account, "xdg-mime", "query", "default", "image/png"); got != "remotr-viewer.desktop" {
					t.Fatalf("%s unmanaged MIME default = %q, want preserved", account.Username, got)
				}
			}
			vmTestSessionAuthoritativeFieldCleanup(t, ctx, backend, accounts, stateDir)
		})
	}

	invalid := vmStructuredSessionResource(models.DesktopSettingProviderGSettings, accounts)
	invalid.Ownership = models.OwnershipAuthoritative
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "merge ownership") {
		t.Fatalf("authoritative DefaultApplications validation = %v, want merge-only rejection", err)
	}

	stopLiveSession()
	for _, backend := range []models.DesktopSettingProvider{models.DesktopSettingProviderDconf, models.DesktopSettingProviderGSettings} {
		vmTestSessionMaliciousHomeIsolation(t, ctx, backend, accounts, stateDir)
	}
}

func vmStructuredSessionResource(backend models.DesktopSettingProvider, accounts []interactiveuser.Account) models.SessionPolicyResource {
	return models.SessionPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
		Name:         "vm-" + string(backend) + "-session", Provider: backend,
		Selector:           models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{accounts[0].Username, accounts[1].Username}},
		LockEnabled:        vmSessionBool(true),
		IdleTimeoutSeconds: vmSessionUint32(600),
		LockDelaySeconds:   vmSessionUint32(5),
		Proxy: &models.SessionProxyPolicy{
			Mode: models.SessionProxyManual, HTTPHost: "proxy.example.test", HTTPPort: 8080,
			IgnoreHosts: []string{"localhost", "127.0.0.1"},
		},
		DisableUserSwitching: vmSessionBool(true),
		DisableLogout:        vmSessionBool(true),
		DisableCommandLine:   vmSessionBool(true),
		DefaultApplications: map[string]string{
			"application/pdf": "remotr-viewer.desktop",
			"text/html":       "remotr-browser.desktop",
		},
	}
}

func vmAssertStructuredSessionState(t *testing.T, account interactiveuser.Account) {
	t.Helper()
	want := []struct {
		schema, key, value string
	}{
		{"org.gnome.desktop.screensaver", "lock-enabled", "true"},
		{"org.gnome.desktop.session", "idle-delay", "uint32 600"},
		{"org.gnome.desktop.screensaver", "lock-delay", "uint32 5"},
		{"org.gnome.system.proxy", "mode", "'manual'"},
		{"org.gnome.system.proxy", "ignore-hosts", "['localhost', '127.0.0.1']"},
		{"org.gnome.system.proxy.http", "host", "'proxy.example.test'"},
		{"org.gnome.system.proxy.http", "port", "8080"},
		{"org.gnome.desktop.lockdown", "disable-user-switching", "true"},
		{"org.gnome.desktop.lockdown", "disable-log-out", "true"},
		{"org.gnome.desktop.lockdown", "disable-command-line", "true"},
	}
	for _, setting := range want {
		if got := vmRunSessionTool(t, account, "gsettings", "get", setting.schema, setting.key); got != setting.value {
			t.Fatalf("%s %s/%s = %q, want %q", account.Username, setting.schema, setting.key, got, setting.value)
		}
	}
	for mime, desktopFile := range map[string]string{"application/pdf": "remotr-viewer.desktop", "text/html": "remotr-browser.desktop"} {
		if got := vmRunSessionTool(t, account, "xdg-mime", "query", "default", mime); got != desktopFile {
			t.Fatalf("%s default %s = %q, want %q", account.Username, mime, got, desktopFile)
		}
	}
}

func vmTestSessionAuthoritativeFieldCleanup(t *testing.T, ctx context.Context, backend models.DesktopSettingProvider, accounts []interactiveuser.Account, stateDir string) {
	t.Helper()
	for _, account := range accounts {
		vmRunSessionTool(t, account, "gsettings", "reset", "org.gnome.desktop.lockdown", "disable-log-out")
	}
	resource := models.SessionPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipAuthoritative},
		Name:         "vm-" + string(backend) + "-cleanup", Provider: backend,
		Selector:      models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{accounts[0].Username, accounts[1].Username}},
		DisableLogout: vmSessionBool(true),
	}
	address := "m5-desktop/" + resource.Name
	provider := vmRegisteredSessionProvider(t, resource, stateDir, address)
	if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("initial authoritative %s Apply = %+v", backend, result)
	}
	resource.Selector.Usernames = []string{accounts[0].Username}
	provider = vmRegisteredSessionProvider(t, resource, stateDir, address)
	if check := provider.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("%s authoritative cleanup Check = %+v, want drifted", backend, check)
	}
	if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("%s authoritative cleanup Apply = %+v", backend, result)
	}
	if check := provider.Check(ctx); check.Status != contract.Compliant {
		t.Fatalf("%s authoritative cleanup second Check = %+v", backend, check)
	}
	if got := vmRunSessionTool(t, accounts[0], "gsettings", "get", "org.gnome.desktop.lockdown", "disable-log-out"); got != "true" {
		t.Fatalf("selected user cleanup field = %q, want true", got)
	}
	if got := vmRunSessionTool(t, accounts[1], "gsettings", "get", "org.gnome.desktop.lockdown", "disable-log-out"); got != "false" {
		t.Fatalf("departed user cleanup field = %q, want reset false", got)
	}
}

func vmTestSessionMaliciousHomeIsolation(t *testing.T, ctx context.Context, backend models.DesktopSettingProvider, accounts []interactiveuser.Account, stateDir string) {
	t.Helper()
	vmResetSessionQualificationState(t, accounts)
	unsafeAccount, safeAccount := accounts[0], accounts[1]
	configPath := filepath.Join(unsafeAccount.HomeDir, ".config")
	backupPath := filepath.Join(unsafeAccount.HomeDir, ".config.remotr-session-backup")
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

	resource := models.SessionPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
		Name:         "vm-" + string(backend) + "-malicious", Provider: backend,
		Selector:            models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{unsafeAccount.Username, safeAccount.Username}},
		LockEnabled:         vmSessionBool(true),
		DefaultApplications: map[string]string{"text/html": "remotr-browser.desktop"},
	}
	provider := vmRegisteredSessionProvider(t, resource, stateDir, "m5-desktop/"+resource.Name)
	result := provider.Apply(ctx)
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), unsafeAccount.Username) {
		t.Fatalf("%s malicious-home Apply = %+v, want aggregate unsafe-user failure", backend, result)
	}
	if got := vmRunSessionTool(t, safeAccount, "gsettings", "get", "org.gnome.desktop.screensaver", "lock-enabled"); got != "true" {
		t.Fatalf("%s safe user lock after malicious home = %q, want true", backend, got)
	}
	if got := vmRunSessionTool(t, safeAccount, "xdg-mime", "query", "default", "text/html"); got != "remotr-browser.desktop" {
		t.Fatalf("%s safe user default after malicious home = %q", backend, got)
	}
	if entries, err := os.ReadDir(external); err != nil || len(entries) != 0 {
		t.Fatalf("%s malicious home symlink target changed: entries=%v err=%v", backend, entries, err)
	}
	restore()
	if check := provider.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("%s recovered home Check = %+v, want remaining-user drift", backend, check)
	}
	wantActivation := []contract.ActivationSignal{
		{Kind: contract.ActivationLogoutRequired},
		{Kind: contract.ActivationApplicationRestart, Target: "remotr-browser.desktop"},
	}
	if result := provider.Apply(ctx); result.Status != contract.Changed || result.Err != nil || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("%s recovered home Apply = %+v, want changed with activation %+v", backend, result, wantActivation)
	}
	if check := provider.Check(ctx); check.Status != contract.Compliant {
		t.Fatalf("%s recovered home second Check = %+v", backend, check)
	}
}

func vmRegisteredSessionProvider(t *testing.T, resource models.SessionPolicyResource, stateDir, address string) contract.Provider {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{SessionPolicies: []models.SessionPolicyResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindSessionPolicy {
		t.Fatalf("session-policy registry resources = %+v, %v", resources, err)
	}
	hostFacts, err := facts.Read()
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{Facts: hostFacts, StateDir: stateDir, ResourceAddress: address})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := handler.(*sessionpolicy.Applicator); !ok {
		t.Fatalf("session-policy registry provider = %T", handler)
	}
	provider, err := contract.New(handler)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmSessionAccounts(t *testing.T) []interactiveuser.Account {
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
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Username < accounts[j].Username })
	if len(accounts) != 2 || accounts[0].Username != "remotr-desktop-a" || accounts[1].Username != "remotr-desktop-b" {
		t.Fatalf("desktop fixture accounts = %+v", accounts)
	}
	return accounts
}

func vmStartSessionLiveUser(t *testing.T, account interactiveuser.Account) func() {
	t.Helper()
	ready := filepath.Join(account.HomeDir, ".remotr-session-policy-live-ready")
	stop := filepath.Join(account.HomeDir, ".remotr-session-policy-live-stop")
	_ = os.Remove(ready)
	_ = os.Remove(stop)
	var output bytes.Buffer
	command := exec.Command("runuser", "-u", account.Username, "--", "env", "HOME="+account.HomeDir, "dbus-run-session", "--", "sh", "-eu", "-c", `touch "$HOME/.remotr-session-policy-live-ready"; while test ! -e "$HOME/.remotr-session-policy-live-stop"; do sleep 0.05; done`)
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

func vmResetSessionQualificationState(t *testing.T, accounts []interactiveuser.Account) {
	t.Helper()
	schemas := []string{
		"org.gnome.desktop.screensaver", "org.gnome.desktop.session", "org.gnome.system.proxy",
		"org.gnome.system.proxy.http", "org.gnome.system.proxy.https", "org.gnome.desktop.lockdown",
		"com.remotr.Qualification",
	}
	for _, account := range accounts {
		for _, schema := range schemas {
			vmRunSessionTool(t, account, "gsettings", "reset-recursively", schema)
		}
		for _, path := range []string{
			filepath.Join(account.HomeDir, ".config", "mimeapps.list"),
			filepath.Join(account.HomeDir, ".local", "share", "applications", "mimeapps.list"),
		} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Fatalf("remove %s: %v", path, err)
			}
		}
	}
}

func vmRunSessionTool(t *testing.T, account interactiveuser.Account, command string, args ...string) string {
	t.Helper()
	commandArgs := []string{"-u", account.Username, "--", "env", "HOME=" + account.HomeDir, "dbus-run-session", "--", command}
	commandArgs = append(commandArgs, args...)
	output, err := exec.Command("runuser", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v for %s: %v: %s", command, args, account.Username, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vmAssertSessionUbuntu2404(t *testing.T) {
	t.Helper()
	hostFacts, err := facts.Read()
	if err != nil {
		t.Fatal(err)
	}
	if hostFacts.Distro != types.Ubuntu || hostFacts.DistroVersion != "24.04" || hostFacts.Arch != types.X86 {
		t.Fatalf("desktop VM facts = %+v", hostFacts)
	}
	for _, backend := range []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings} {
		if !slices.Contains(hostFacts.Desktop, backend) {
			t.Fatalf("desktop VM backends = %+v, missing %s", hostFacts.Desktop, backend)
		}
	}
}

func vmSessionBool(value bool) *bool       { return &value }
func vmSessionUint32(value uint32) *uint32 { return &value }
