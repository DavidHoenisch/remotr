package networkfiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type convergenceRunner struct {
	provider  string
	activated bool
	probed    bool
	calls     []executil.MockCall
}

func (r *convergenceRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	if name == "ip" && slices.Equal(args, []string{"-json", "link", "show"}) {
		return []byte(`[{"ifname":"eth0","address":"02:00:00:00:00:01","link_type":"ether","operstate":"UP"}]`), nil, nil
	}
	if name == "ip" && slices.Equal(args, []string{"-json", "address", "show", "dev", "eth0"}) {
		address := "192.0.2.20"
		if r.activated {
			address = "192.0.2.10"
		}
		return []byte(fmt.Sprintf(`[{"ifname":"eth0","operstate":"UP","addr_info":[{"family":"inet","local":%q,"prefixlen":24}]}]`, address)), nil, nil
	}
	if r.provider == models.NetworkProviderNetplan && name == "netplan" && slices.Equal(args, []string{"generate"}) {
		return nil, nil, nil
	}
	if r.provider == models.NetworkProviderNetplan && name == "netplan" && slices.Equal(args, []string{"--help"}) {
		r.probed = true
		return nil, nil, nil
	}
	if r.provider == models.NetworkProviderNetplan && name == "netplan" && slices.Equal(args, []string{"apply"}) {
		r.activated = true
		return nil, nil, nil
	}
	if r.provider == models.NetworkProviderSystemdNetworkd && name == "networkctl" && slices.Equal(args, []string{"reload"}) {
		return nil, nil, nil
	}
	if r.provider == models.NetworkProviderSystemdNetworkd && name == "networkctl" && slices.Equal(args, []string{"--version"}) {
		r.probed = true
		return nil, nil, nil
	}
	if r.provider == models.NetworkProviderSystemdNetworkd && name == "networkctl" && slices.Equal(args, []string{"reconfigure", "eth0"}) {
		r.activated = true
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected command: %s %v", name, args)
}

func TestNetplanProfileReportsConfiguredAndEffectiveStateSeparately(t *testing.T) {
	configDir := t.TempDir()
	path := filepath.Join(configDir, "90-remotr-uplink.yaml")
	configured := []byte("# managed by remotr: uplink\nnetwork:\n  version: 2\n  ethernets:\n    eth0:\n      match:\n        name: eth0\n      addresses:\n        - 192.0.2.10/24\n")
	if err := os.WriteFile(path, configured, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"ip [-json link show]":             {Stdout: []byte(`[{"ifname":"eth0","address":"02:00:00:00:00:01","link_type":"ether","operstate":"UP"}]`)},
		"ip [-json address show dev eth0]": {Stdout: []byte(`[{"ifname":"eth0","operstate":"UP","addr_info":[{"family":"inet","local":"192.0.2.20","prefixlen":24}]}]`)},
	}}
	provider := New(models.NetworkProfileResource{
		Name: "uplink", Provider: models.NetworkProviderNetplan,
		Selector:    models.NetworkInterfaceSelector{Name: "eth0", Type: "ethernet"},
		ProfileName: "office", ProfileType: "ethernet", Addresses: []string{"192.0.2.10/24"},
	}, runner)
	provider.ConfigDir = configDir

	check := provider.Check(context.Background())
	if check.Status != executor.Drifted || check.ReasonCode != "audit_plan" {
		t.Fatalf("Check() = %+v", check)
	}
	report, ok := check.Actual.(ProfileReport)
	if !ok || report.Backend != models.NetworkProviderNetplan || !report.Configured.Compliant || report.Effective.Compliant || report.Interface != "eth0" {
		t.Fatalf("separate profile report = %#v", check.Actual)
	}
	if len(runner.Calls) != 2 {
		t.Fatalf("audit check mutated profile: %+v", runner.Calls)
	}
}

func TestFileBackedProfilesConvergeThroughArmedRollbackTransaction(t *testing.T) {
	for _, providerName := range []string{models.NetworkProviderNetplan, models.NetworkProviderSystemdNetworkd} {
		t.Run(providerName, func(t *testing.T) {
			audit, enforce := false, true
			runner := &convergenceRunner{provider: providerName}
			provider := New(models.NetworkProfileResource{
				ResourceMeta: models.ResourceMeta{Enforce: &enforce},
				Name:         "uplink", Provider: providerName, Audit: &audit,
				Selector:    models.NetworkInterfaceSelector{Name: "eth0", Type: "ethernet"},
				ProfileName: "office", ProfileType: "ethernet", Addresses: []string{"192.0.2.10/24"}, RollbackTimeout: "2m",
			}, runner)
			provider.ConfigDir = t.TempDir()
			provider.StateDir = t.TempDir()
			provider.Now = func() time.Time { return time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC) }
			provider.AfterFunc = func(time.Duration, func()) {}
			previous := []byte("previous configuration\n")
			if err := os.WriteFile(provider.path(), previous, 0o640); err != nil {
				t.Fatal(err)
			}

			before := provider.Check(context.Background())
			if before.Status != executor.Drifted || before.ReasonCode != executor.ReasonStateDrift {
				t.Fatalf("initial Check() = %+v", before)
			}
			result := provider.ApplyResult(context.Background())
			if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
				t.Fatalf("ApplyResult() = %+v", result)
			}
			if !runner.probed {
				t.Fatal("provider availability was not checked before mutation")
			}
			after := provider.Check(context.Background())
			if after.Status != executor.Compliant {
				t.Fatalf("second Check() = %+v", after)
			}
			store, err := networkstate.New(networkstate.Options{Root: provider.StateDir, Runner: runner, Now: provider.now})
			if err != nil {
				t.Fatal(err)
			}
			status, err := store.Status()
			if err != nil || status.Intent == nil || status.Intent.Backend != providerName || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || !status.Intent.WatchdogArmed {
				t.Fatalf("armed transaction = %+v, err=%v", status, err)
			}
			if _, err := store.Rollback(context.Background(), "provider_contract"); err != nil {
				t.Fatal(err)
			}
			restored, err := os.ReadFile(provider.path())
			if err != nil || !slices.Equal(restored, previous) {
				t.Fatalf("restored provider snapshot = %q, err=%v", restored, err)
			}
			afterRollback := provider.Check(context.Background())
			if afterRollback.Status != executor.Drifted || afterRollback.ReasonCode != executor.ReasonStateDrift {
				t.Fatalf("second Check after rollback = %+v", afterRollback)
			}
		})
	}
}

func TestFileBackedProfilesRenderExplicitActivationPolicy(t *testing.T) {
	autoConnect := false
	resource := models.NetworkProfileResource{Name: "uplink", ProfileType: "ethernet", AutoConnect: &autoConnect}
	if got := string(renderNetplan(resource, "eth0")); !strings.Contains(got, "activation-mode: manual") || strings.Contains(got, "optional: true") {
		t.Fatalf("netplan activation policy = %q", got)
	}
	if got := string(renderNetworkd(resource, "eth0")); !strings.Contains(got, "[Link]\nActivationPolicy=manual") {
		t.Fatalf("networkd activation policy = %q", got)
	}
}

func TestNetplanAuditDiscardsExistingCredentialMaterial(t *testing.T) {
	const canary = "remotr-secret-canary-do-not-report"
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "90-remotr-wifi.yaml"), []byte("network:\n  wifis:\n    wlan0:\n      access-points:\n        corp:\n          password: "+canary+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"ip [-json link show]":              {Stdout: []byte(`[{"ifname":"wlan0","address":"02:00:00:00:00:0a","link_type":"wlan","operstate":"UP"}]`)},
		"ip [-json address show dev wlan0]": {Stdout: []byte(`[{"ifname":"wlan0","operstate":"UP","addr_info":[]}]`)},
	}}
	provider := New(models.NetworkProfileResource{
		Name: "wifi", Provider: models.NetworkProviderNetplan,
		Selector:    models.NetworkInterfaceSelector{Name: "wlan0", Type: "wifi"},
		ProfileName: "office", ProfileType: "wifi", SSID: "corp", CredentialRef: "remotr:wifi/office@active",
	}, runner)
	provider.ConfigDir = configDir
	check := provider.Check(context.Background())
	if check.Status != executor.Drifted || strings.Contains(fmt.Sprintf("%+v", check), canary) {
		t.Fatalf("redacted netplan report = %+v", check)
	}
}
