//go:build vmsafety

package timesync_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/timesync"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098, OS-KHB-009: exercise the exact systemd-timesyncd provider on
// pinned Ubuntu. Enablement and server ownership remain independent, native
// configured/effective state is observable, unrelated configuration survives,
// activation is truthful, and each successful Apply has a compliant second
// Check and no-change replay.
func TestTimeSyncProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-017
		t.Skip("time-sync VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	configDir := "/etc/systemd/timesyncd.conf.d"
	managedPath := filepath.Join(configDir, "99-remotr-ubuntu-vm.conf")
	unrelatedPath := filepath.Join(configDir, "50-remotr-qualification-unrelated.conf")
	unrelated := []byte("[Time]\nPollIntervalMinSec=32\n")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelatedPath, unrelated, 0o640); err != nil {
		t.Fatal(err)
	}
	originalNTP := vmTimeSyncValue(t, "timedatectl", "show", "--property=NTP", "--value")
	t.Cleanup(func() {
		_ = os.Remove(managedPath)
		_ = os.Remove(unrelatedPath)
		value := "false"
		if originalNTP == "yes" {
			value = "true"
		}
		_ = exec.Command("timedatectl", "set-ntp", value).Run()
	})

	wantEnabled := originalNTP != "yes"
	enablement := vmTimeSyncProvider(t, models.TimeSyncResource{
		Name: "ubuntu-enablement", Provider: models.TimeSyncProviderSystemdTimesyncd, Enabled: &wantEnabled,
	})
	if check := enablement.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("opposite enablement Check = %+v, want drifted", check)
	}
	result := enablement.Apply(ctx)
	if result.Status != contract.Changed || result.Err != nil || len(result.Activation) != 0 {
		t.Fatalf("enablement Apply = %+v, want immediate changed outcome", result)
	}
	wantUnitFileState, wantActiveState, wantNTP := "disabled", "inactive", "no"
	if wantEnabled {
		wantUnitFileState, wantActiveState, wantNTP = "enabled", "active", "yes"
	}
	if got := vmTimeSyncValue(t, "systemctl", "show", "systemd-timesyncd.service", "--property=UnitFileState", "--value"); got != wantUnitFileState {
		t.Fatalf("configured timesyncd state = %q, want %q", got, wantUnitFileState)
	}
	if got := vmTimeSyncValue(t, "systemctl", "show", "systemd-timesyncd.service", "--property=ActiveState", "--value"); got != wantActiveState {
		t.Fatalf("effective timesyncd state = %q, want %q", got, wantActiveState)
	}
	if got := vmTimeSyncValue(t, "timedatectl", "show", "--property=NTP", "--value"); got != wantNTP {
		t.Fatalf("generic NTP state = %q, want %q", got, wantNTP)
	}
	vmAssertTimeSyncSecondPass(t, enablement)

	servers := vmTimeSyncProvider(t, models.TimeSyncResource{
		Name: "ubuntu-vm", Provider: models.TimeSyncProviderSystemdTimesyncd,
		Servers: []string{"time.example.invalid"}, Pools: []string{"pool.example.invalid"},
	})
	if check := servers.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("server fragment Check = %+v, want drifted", check)
	}
	result = servers.Apply(ctx)
	wantActivation := []contract.ActivationSignal{{Kind: contract.ActivationRestart, Target: "systemd-timesyncd.service"}}
	if result.Status != contract.Changed || result.Err != nil || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("server fragment Apply = %+v, want changed/restart", result)
	}
	wantFragment := "[Time]\nNTP=time.example.invalid\nFallbackNTP=pool.example.invalid\n"
	if got, err := os.ReadFile(managedPath); err != nil || string(got) != wantFragment {
		t.Fatalf("managed time-server fragment = %q, %v; want %q", got, err, wantFragment)
	}
	if got, err := os.ReadFile(unrelatedPath); err != nil || !slices.Equal(got, unrelated) {
		t.Fatalf("unrelated time-sync fragment = %q, %v; want preserved", got, err)
	}
	if got := vmTimeSyncValue(t, "timedatectl", "show", "--property=NTP", "--value"); got != wantNTP {
		t.Fatalf("server-only Apply changed NTP state to %q, want %q", got, wantNTP)
	}
	vmAssertTimeSyncSecondPass(t, servers)
}

func vmTimeSyncProvider(t *testing.T, resource models.TimeSyncResource) contract.Provider {
	t.Helper()
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(timesync.New(resource, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmTimeSyncValue(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vmAssertTimeSyncSecondPass(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("time-sync second Check = %+v, want compliant", check)
	}
	result := provider.Apply(context.Background())
	if result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("time-sync second Apply = %+v, want no change", result)
	}
}
