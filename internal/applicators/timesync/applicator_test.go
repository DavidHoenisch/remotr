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
	applicator := timesync.New(models.TimeSyncResource{
		Name:     "ntp",
		Provider: "systemd-timesyncd",
		Enabled:  &enabled,
		Servers:  []string{"time.example.test"},
	}, nil)
	applicator.SupportsCustomServers = func() bool { return false }

	check := applicator.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != "time_sync_servers_unsupported" {
		t.Fatalf("Check() = %+v, want custom servers unsupported", check)
	}
}

func TestApplicator_ConvergesEnablementAndOwnedServerFragment(t *testing.T) {
	enabled := true
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"timedatectl [show --property=NTP --value]": {Stdout: []byte("no\n")},
		"timedatectl [set-ntp true]":                {},
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
	if got := runner.Calls; len(got) != 2 || got[0].Name != "timedatectl" || !slices.Equal(got[0].Args, []string{"show", "--property=NTP", "--value"}) || got[1].Name != "timedatectl" || !slices.Equal(got[1].Args, []string{"set-ntp", "true"}) {
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
