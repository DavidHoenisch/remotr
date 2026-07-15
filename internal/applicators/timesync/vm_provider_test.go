//go:build vmsafety

package timesync_test

import (
	"context"
	"os"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/timesync"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-KHB-009: exercise the systemd-timesyncd provider against the disposable
// guest, including provider enablement and its named NTP fragment.
func TestTimeSyncProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-017
		t.Skip("time-sync VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	currentEnabled := true
	current := timesync.New(models.TimeSyncResource{Name: "current", Provider: models.TimeSyncProviderSystemdTimesyncd, Enabled: &currentEnabled}, nil)
	check := current.Check(ctx)
	if check.Status == executor.CheckFailed {
		t.Fatalf("read current time synchronization = %+v", check)
	}
	wantEnabled := true
	if check.Status == executor.Compliant {
		wantEnabled = false
	}
	provider := timesync.New(models.TimeSyncResource{
		Name:     "vm",
		Provider: models.TimeSyncProviderSystemdTimesyncd,
		Enabled:  &wantEnabled,
		Servers:  []string{"0.debian.pool.ntp.org"},
	}, nil)
	result := provider.ApplyResult(ctx)
	if result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := provider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("Check() after real Apply = %+v", check)
	}
}
