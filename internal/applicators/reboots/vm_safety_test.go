//go:build vmsafety

package reboots_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/applicators/reboots"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// TestCoordinatedRebootSafetyVM runs in two processes separated by the
// harness's controlled VM reboot. It exercises the real boot-ID probe,
// applicator, durable acknowledgement state, completion check, and no-loop
// behavior without allowing the test binary itself to reboot the host.
func TestCoordinatedRebootSafetyVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM reboot safety test must run as root")
	}
	phase := os.Getenv("REMOTR_REBOOT_VM_PHASE")
	stateDir := os.Getenv("REMOTR_REBOOT_VM_STATE_DIR")
	if phase == "" || stateDir == "" {
		t.Fatal("REMOTR_REBOOT_VM_PHASE and REMOTR_REBOOT_VM_STATE_DIR are required")
	}

	ctx := context.Background()
	store := rebootstate.New(stateDir)
	probes := reboots.SystemProbes{}
	now := time.Now().UTC()
	resource := models.RebootResource{
		Name: "vm-coordinated-reboot", Generation: "vm-generation-1", Timeout: "30m",
		UserInhibition: models.InhibitionIgnore, WorkloadInhibition: models.InhibitionIgnore,
	}
	provider := reboots.New(resource, store, probes, func() time.Time { return now })
	bootID, err := probes.BootID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	switch phase {
	case "prepare":
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatal(err)
		}
		vmAssertRebootPreflight(t, stateDir, resource, probes, now)
		vmAssertSameBootTimeoutIsTerminal(t, stateDir, resource, probes, bootID, now)
		if _, err := store.Record([]rebootstate.Source{{Address: "vm/kernel", Provider: "vm-safety"}}); err != nil {
			t.Fatal(err)
		}
		if check := provider.Check(ctx); check.Status != executor.Drifted {
			t.Fatalf("initial Check = %+v", check)
		}
		applied := provider.ApplyResult(ctx)
		if applied.Status != executor.ApplyDeferred || applied.DeferredWork == nil || applied.DeferredWork.ReasonCode != "pre_reboot_ack" {
			t.Fatalf("ApplyResult = %+v", applied)
		}
		status, err := store.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if status.Intent == nil || status.Intent.Phase != rebootstate.PhaseAwaitingAcknowledgement || status.Intent.PriorBootID != bootID {
			t.Fatalf("prepared state = %+v", status)
		}
		attempt, err := store.Acknowledge(resource.Generation, now, bootID)
		if err != nil {
			t.Fatal(err)
		}
		if attempt.Phase != rebootstate.PhaseAttempting || attempt.AttemptGeneration != 1 {
			t.Fatalf("acknowledged attempt = %+v", attempt)
		}
	case "verify":
		before, err := store.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if before.Intent == nil || before.Intent.Phase != rebootstate.PhaseAttempting || before.Intent.PriorBootID == bootID {
			t.Fatalf("state did not cross a changed boot identity: boot=%q state=%+v", bootID, before)
		}
		completed, err := store.Reconcile(bootID, now)
		if err != nil {
			t.Fatal(err)
		}
		if completed.Required || completed.Intent != nil || completed.Completion == nil || completed.Completion.Generation != resource.Generation || completed.Completion.BootID != bootID || completed.Completion.AttemptGeneration != 1 {
			t.Fatalf("completed state = %+v", completed)
		}
		if check := provider.Check(ctx); check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
			t.Fatalf("second Check = %+v", check)
		}
		if _, err := store.Prepare(rebootstate.Intent{
			Generation: resource.Generation, PriorBootID: bootID, PreparedAt: now,
			NotBefore: now, Timeout: 30 * time.Minute,
		}); err == nil {
			t.Fatal("completed desired generation prepared another reboot")
		}
	default:
		t.Fatalf("unknown VM reboot safety phase %q", phase)
	}
}

func vmAssertRebootPreflight(t *testing.T, stateDir string, resource models.RebootResource, probes reboots.SystemProbes, now time.Time) {
	t.Helper()
	ctx := context.Background()
	outside := resource
	outside.Generation = "vm-maintenance-window"
	outside.MaintenanceWindow = &models.RebootMaintenanceWindow{
		Weekdays: []string{now.AddDate(0, 0, 1).Weekday().String()}, Start: "00:00", Duration: "1m",
	}
	maintenanceStore := rebootstate.New(filepath.Join(stateDir, "maintenance"))
	result := reboots.New(outside, maintenanceStore, probes, func() time.Time { return now }).ApplyResult(ctx)
	if result.Status != executor.ApplyDeferred || result.DeferredWork == nil || result.DeferredWork.ReasonCode != "maintenance_window" {
		t.Fatalf("outside-window ApplyResult = %+v", result)
	}
	if status, err := maintenanceStore.Snapshot(); err != nil || status.Intent != nil {
		t.Fatalf("outside-window state = %+v, %v; want no intent", status, err)
	}

	active, err := probes.ActiveWorkloadInhibitors(ctx)
	if err != nil || active {
		t.Fatalf("baseline shutdown inhibitors = %t, %v; want none", active, err)
	}
	blocker := exec.Command("systemd-inhibit", "--what=shutdown", "--who=remotr-qualification", "--why=verify-reboot-preflight", "--mode=block", "/bin/sh", "-c", "printf 'ready\\n'; read _")
	stdin, err := blocker.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := blocker.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := blocker.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = blocker.Wait()
	}()
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "ready\n" {
		t.Fatalf("shutdown inhibitor readiness = %q, %v", ready, err)
	}
	blocked := resource
	blocked.Generation = "vm-workload-inhibitor"
	blocked.WorkloadInhibition = models.InhibitionDefer
	blockedStore := rebootstate.New(filepath.Join(stateDir, "inhibitor"))
	result = reboots.New(blocked, blockedStore, probes, func() time.Time { return now }).ApplyResult(ctx)
	if result.Status != executor.ApplyDeferred || result.DeferredWork == nil || result.DeferredWork.ReasonCode != "active_workload_inhibitor" {
		t.Fatalf("shutdown-inhibited ApplyResult = %+v", result)
	}
	if status, err := blockedStore.Snapshot(); err != nil || status.Intent != nil {
		t.Fatalf("shutdown-inhibited state = %+v, %v; want no intent", status, err)
	}
}

func vmAssertSameBootTimeoutIsTerminal(t *testing.T, stateDir string, resource models.RebootResource, probes reboots.SystemProbes, bootID string, now time.Time) {
	t.Helper()
	store := rebootstate.New(filepath.Join(stateDir, "timeout"))
	resource.Generation = "vm-timeout-generation"
	if _, err := store.Prepare(rebootstate.Intent{
		Generation: resource.Generation, Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: bootID, PreparedAt: now, NotBefore: now, Timeout: time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acknowledge(resource.Generation, now, bootID); err != nil {
		t.Fatal(err)
	}
	timedOutAt := now.Add(2 * time.Minute)
	status, err := store.Reconcile(bootID, timedOutAt)
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Phase != rebootstate.PhaseTimedOut || status.Intent.Reason != "reboot_timeout_same_boot_id" || status.Intent.AttemptGeneration != 1 {
		t.Fatalf("same-boot timeout state = %+v", status)
	}
	provider := reboots.New(resource, store, probes, func() time.Time { return timedOutAt })
	if check := provider.Check(context.Background()); check.Status != executor.CheckFailed || check.ReasonCode != "reboot_timeout_same_boot_id" {
		t.Fatalf("timed-out Check = %+v, want observable terminal failure", check)
	}
	if _, err := store.Prepare(rebootstate.Intent{
		Generation: resource.Generation, PriorBootID: bootID, PreparedAt: timedOutAt,
		NotBefore: timedOutAt, Timeout: time.Minute,
	}); err == nil {
		t.Fatal("timed-out generation prepared another reboot")
	}
}
