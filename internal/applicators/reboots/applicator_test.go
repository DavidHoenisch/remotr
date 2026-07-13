package reboots_test

import (
	"context"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/applicators/reboots"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicatorPreparesAcknowledgedIntentWithoutExecutingReboot(t *testing.T) {
	now := time.Date(2026, 7, 12, 2, 30, 0, 0, time.UTC) // Sunday
	store := rebootstate.New(t.TempDir())
	if _, err := store.Record([]rebootstate.Source{{Address: "base/packages/kernel", Provider: "apt"}}); err != nil {
		t.Fatal(err)
	}
	resource := validResource()
	app := reboots.New(resource, store, rebootProbes{bootID: "boot-1", acPower: true}, func() time.Time { return now })

	check := app.Check(context.Background())
	if check.Status != executor.Drifted {
		t.Fatalf("Check() = %+v", check)
	}
	result := app.ApplyResult(context.Background())
	if result.Status != executor.ApplyDeferred || result.DeferredWork == nil || result.DeferredWork.ReasonCode != "pre_reboot_ack" || result.RebootRequired != executor.RebootRequired {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	status, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Generation != "kernel-6.12.1" || status.Intent.Phase != rebootstate.PhaseAwaitingAcknowledgement || status.Intent.PriorBootID != "boot-1" || !status.Intent.NotBefore.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("prepared state = %+v", status)
	}
}

func TestApplicatorDefersUnsafePreconditionsWithoutPreparingIntent(t *testing.T) {
	baseNow := time.Date(2026, 7, 12, 2, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*models.RebootResource, *rebootProbes, *time.Time)
		reason executor.ReasonCode
	}{
		{"outside maintenance window", func(_ *models.RebootResource, _ *rebootProbes, now *time.Time) { *now = now.Add(8 * time.Hour) }, "maintenance_window"},
		{"AC power unavailable", func(_ *models.RebootResource, probes *rebootProbes, _ *time.Time) { probes.acPower = false }, "ac_power_required"},
		{"active user", func(_ *models.RebootResource, probes *rebootProbes, _ *time.Time) { probes.activeUsers = true }, "active_user_inhibitor"},
		{"active workload inhibitor", func(_ *models.RebootResource, probes *rebootProbes, _ *time.Time) { probes.activeWorkloads = true }, "active_workload_inhibitor"},
		{"deadline elapsed", func(resource *models.RebootResource, _ *rebootProbes, _ *time.Time) {
			resource.Deadline = baseNow.Add(-time.Minute).Format(time.RFC3339)
		}, "reboot_deadline_elapsed"},
		{"delay exceeds deadline", func(resource *models.RebootResource, _ *rebootProbes, _ *time.Time) {
			resource.Delay = "2m"
			resource.Deadline = baseNow.Add(time.Minute).Format(time.RFC3339)
		}, "reboot_deadline_elapsed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := baseNow
			resource := validResource()
			probes := rebootProbes{bootID: "boot-1", acPower: true}
			tc.mutate(&resource, &probes, &now)
			store := rebootstate.New(t.TempDir())
			if _, err := store.Record([]rebootstate.Source{{Address: "base/packages/kernel"}}); err != nil {
				t.Fatal(err)
			}
			result := reboots.New(resource, store, probes, func() time.Time { return now }).ApplyResult(context.Background())
			if result.Status != executor.ApplyDeferred || result.DeferredWork == nil || result.DeferredWork.ReasonCode != tc.reason {
				t.Fatalf("ApplyResult() = %+v, want %q", result, tc.reason)
			}
			status, err := store.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if status.Intent != nil {
				t.Fatalf("unsafe precondition prepared intent: %+v", status.Intent)
			}
		})
	}
}

func validResource() models.RebootResource {
	enforce := true
	return models.RebootResource{
		ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindReboot, Enforce: &enforce},
		Name:         "kernel-maintenance", Generation: "kernel-6.12.1", OnlyIfRequired: true,
		Delay: "2m", Timeout: "15m", Deadline: "2026-07-13T05:00:00Z",
		MaintenanceWindow: &models.RebootMaintenanceWindow{Weekdays: []string{"Sunday"}, Start: "02:00", Duration: "2h"},
		RequireACPower:    true, UserInhibition: models.InhibitionDefer, WorkloadInhibition: models.InhibitionDefer,
	}
}

type rebootProbes struct {
	bootID          string
	acPower         bool
	activeUsers     bool
	activeWorkloads bool
}

func (p rebootProbes) BootID(context.Context) (string, error)    { return p.bootID, nil }
func (p rebootProbes) OnACPower(context.Context) (bool, error)   { return p.acPower, nil }
func (p rebootProbes) ActiveUsers(context.Context) (bool, error) { return p.activeUsers, nil }
func (p rebootProbes) ActiveWorkloadInhibitors(context.Context) (bool, error) {
	return p.activeWorkloads, nil
}
