package rebootstate_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
)

func TestStoreCompletesAcknowledgedRebootAfterBootIDChanges(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	store := rebootstate.New(t.TempDir())
	if _, err := store.Record([]rebootstate.Source{{Address: "base/packages/kernel", Provider: "apt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(rebootstate.Intent{
		Generation: "kernel-6.12.1", Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: "boot-1", PreparedAt: now, NotBefore: now, Timeout: 15 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Acknowledge("kernel-6.12.1", now, "boot-1")
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Phase != rebootstate.PhaseAttempting || attempt.AttemptGeneration != 1 || attempt.PriorBootID != "boot-1" {
		t.Fatalf("attempt = %+v", attempt)
	}

	restarted := rebootstate.New(filepath.Dir(store.Path()))
	status, err := restarted.Reconcile("boot-2", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if status.Required || status.Intent != nil || status.Completion == nil || status.Completion.Generation != "kernel-6.12.1" || status.Completion.BootID != "boot-2" {
		t.Fatalf("completed reboot state = %+v", status)
	}
}

func TestStoreDoesNotCompleteOrRepeatWhenBootIDDoesNotChange(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	store := rebootstate.New(t.TempDir())
	if _, err := store.Prepare(rebootstate.Intent{
		Generation: "generation-1", Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: "boot-1", PreparedAt: now, NotBefore: now, Timeout: 5 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acknowledge("generation-1", now, "boot-1"); err != nil {
		t.Fatal(err)
	}

	verifying, err := store.Reconcile("boot-1", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if verifying.Intent == nil || verifying.Intent.Phase != rebootstate.PhaseAttempting || verifying.Intent.Reason != "boot_id_unchanged" || verifying.Completion != nil {
		t.Fatalf("same-boot verification = %+v", verifying)
	}
	timedOut, err := store.Reconcile("boot-1", now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if timedOut.Intent == nil || timedOut.Intent.Phase != rebootstate.PhaseTimedOut || timedOut.Intent.Reason != "reboot_timeout_same_boot_id" || timedOut.Completion != nil {
		t.Fatalf("timed-out reboot = %+v", timedOut)
	}
	if _, err := store.Acknowledge("generation-1", now.Add(6*time.Minute), "boot-1"); err == nil {
		t.Fatal("timed-out generation was acknowledged for a second attempt")
	}
}

func TestStoreDoesNotCompleteChangedBootAfterAttemptTimeout(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	store := rebootstate.New(t.TempDir())
	if _, err := store.Prepare(rebootstate.Intent{
		Generation: "generation-1", Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: "boot-1", PreparedAt: now, NotBefore: now, Timeout: time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acknowledge("generation-1", now, "boot-1"); err != nil {
		t.Fatal(err)
	}
	status, err := store.Reconcile("boot-2", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if status.Completion != nil || status.Intent == nil || status.Intent.Phase != rebootstate.PhaseTimedOut || status.Intent.Reason != "reboot_timeout" {
		t.Fatalf("late changed boot = %+v", status)
	}
}

func TestStoreTimesOutUnacknowledgedIntentAtDeadline(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	store := rebootstate.New(t.TempDir())
	if _, err := store.Prepare(rebootstate.Intent{
		Generation: "generation-1", Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: "boot-1", PreparedAt: now, NotBefore: now,
		Timeout: 5 * time.Minute, Deadline: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	status, err := store.Reconcile("boot-1", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if status.Completion != nil || status.Intent == nil || status.Intent.Phase != rebootstate.PhaseTimedOut || status.Intent.Reason != "reboot_deadline_elapsed" || status.AttemptGeneration != 0 {
		t.Fatalf("unacknowledged deadline = %+v", status)
	}
	if _, err := store.Acknowledge("generation-1", now.Add(2*time.Minute), "boot-1"); err == nil {
		t.Fatal("expired intent was acknowledged")
	}
}

func TestStoreTreatsCompletedGenerationAsNoLoop(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	store := rebootstate.New(t.TempDir())
	if _, err := store.Prepare(rebootstate.Intent{Generation: "once", Phase: rebootstate.PhaseAwaitingAcknowledgement, PriorBootID: "boot-1", PreparedAt: now, NotBefore: now, Timeout: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acknowledge("once", now, "boot-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reconcile("boot-2", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !store.Completed("once", "boot-2") {
		t.Fatal("completed generation was not remembered")
	}
	if _, err := store.Prepare(rebootstate.Intent{Generation: "once", Phase: rebootstate.PhaseAwaitingAcknowledgement, PriorBootID: "boot-2", PreparedAt: now.Add(time.Minute), NotBefore: now.Add(time.Minute), Timeout: time.Minute}); err == nil {
		t.Fatal("completed generation prepared another reboot")
	}
}
