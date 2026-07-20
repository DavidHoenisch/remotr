package timesync_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/timesync"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-KHB-009: a provider without custom-server support must not accept only
// the enablement portion of a time-sync resource.
func TestApplicator_ReportsCustomServersUnsupported(t *testing.T) {
	enabled := true
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"systemctl [show systemd-timesyncd.service --property=LoadState --value]": {Stdout: []byte("loaded\n")},
	}}
	applicator := timesync.New(models.TimeSyncResource{
		Name:     "ntp",
		Provider: "systemd-timesyncd",
		Enabled:  &enabled,
		Servers:  []string{"time.example.test"},
	}, runner)
	applicator.SupportsCustomServers = func() bool { return false }

	check := applicator.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != "time_sync_servers_unsupported" {
		t.Fatalf("Check() = %+v, want custom servers unsupported", check)
	}
}

// OS-KHB-009 / OS-AEC-098: timedatectl is provider-neutral and may exist when
// systemd-timesyncd does not. The exact backend row must report that mismatch
// as unsupported before probing or changing generic NTP enablement.
func TestProviderReportsUnavailableTimesyncdAsUnsupported(t *testing.T) {
	enabled := true
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"systemctl [show systemd-timesyncd.service --property=LoadState --value]": {Stdout: []byte("not-found\n")},
	}}
	applicator := timesync.New(models.TimeSyncResource{
		Name: "unavailable", Provider: models.TimeSyncProviderSystemdTimesyncd, Enabled: &enabled,
	}, runner)

	check := applicator.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != "time_sync_provider_unsupported" {
		t.Fatalf("unavailable systemd-timesyncd Check = %+v, want unsupported", check)
	}
	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("unavailable systemd-timesyncd Apply = %+v, want pre-mutation failure", result)
	}
	if len(runner.Calls) != 2 {
		t.Fatalf("unavailable systemd-timesyncd calls = %#v, want two availability probes", runner.Calls)
	}
	for index, call := range runner.Calls {
		if call.Name != "systemctl" || !slices.Equal(call.Args, []string{"show", "systemd-timesyncd.service", "--property=LoadState", "--value"}) {
			t.Fatalf("availability call %d = %#v, want exact systemctl show", index, call)
		}
	}
}

func TestApplicator_ConvergesEnablementAndOwnedServerFragment(t *testing.T) {
	enabled := true
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"systemctl [show systemd-timesyncd.service --property=LoadState --value]": {Stdout: []byte("loaded\n")},
		"timedatectl [show --property=NTP --value]":                               {Stdout: []byte("no\n")},
		"timedatectl [set-ntp true]":                                              {},
	}}
	applicator := timesync.New(models.TimeSyncResource{
		Name:     "ntp",
		Provider: models.TimeSyncProviderSystemdTimesyncd,
		Enabled:  &enabled,
		Servers:  []string{"time.example.test"},
		Pools:    []string{"pool.example.test"},
	}, runner)
	applicator.ConfigDir = t.TempDir()

	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Changed || !slices.Equal(result.Activation, []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "systemd-timesyncd.service"}}) {
		t.Fatalf("ApplyResult() = %+v, want changed/restart", result)
	}
	if got := runner.Calls; len(got) != 3 || got[0].Name != "systemctl" || !slices.Equal(got[0].Args, []string{"show", "systemd-timesyncd.service", "--property=LoadState", "--value"}) || got[1].Name != "timedatectl" || !slices.Equal(got[1].Args, []string{"show", "--property=NTP", "--value"}) || got[2].Name != "timedatectl" || !slices.Equal(got[2].Args, []string{"set-ntp", "true"}) {
		t.Fatalf("runner calls = %#v", got)
	}
	contents, err := os.ReadFile(filepath.Join(applicator.ConfigDir, "99-remotr-ntp.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "[Time]\nNTP=time.example.test\nFallbackNTP=pool.example.test\n" {
		t.Fatalf("fragment = %q", contents)
	}
}
