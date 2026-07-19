package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/polling"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

func TestCapabilityBlockedSuccessKeepsStablePollingCadence(t *testing.T) {
	policy := polling.NewPolicy(30 * time.Second)
	backoff := polling.NewBackoff(policy, zeroPollingRandom{})
	_ = backoff.NextDelay()

	got := nextSyncDelay(policy, backoff, "endpoint-blocked", nil)
	want := policy.SuccessDelay("endpoint-blocked")
	if got != want || got < policy.Interval || got > policy.Interval+policy.JitterMax {
		t.Fatalf("capability-blocked success delay = %s, want stable %s", got, want)
	}
	if retry := backoff.NextDelay(); retry != policy.RetryBase {
		t.Fatalf("successful capability block did not reset retry backoff: %s", retry)
	}
}

type zeroPollingRandom struct{}

func (zeroPollingRandom) Int64N(int64) int64 { return 0 }

// OS-SRM-007: the composed agent carries durable reboot-required state into a
// later compliant Sync report without coupling it to reboot execution.
func TestSyncRunStateCarriesPersistedRebootRequirementIntoLaterReport(t *testing.T) {
	dir := t.TempDir()
	state := newSyncRunState(dir, "https://remotr.example", nil, nil)
	var pending sync.Pending
	if err := state.recordRebootRequirement(&pending, engine.ApplyResult{Items: []engine.ApplyItem{{
		Address: "base/packages/kernel", Name: "kernel", Provider: "apt",
		Status: executor.Changed, RebootRequired: executor.RebootRequired,
	}}}); err != nil {
		t.Fatal(err)
	}

	restarted := newSyncRunState(dir, "https://remotr.example", nil, nil)
	var afterRestart sync.Pending
	if err := restarted.recordRebootRequirement(&afterRestart, engine.ApplyResult{}); err != nil {
		t.Fatal(err)
	}
	afterRestart.SetFromPipeline(nil, engine.DriftReport{InCompliance: true}, engine.ApplyResult{}, nil, "digest")
	if !afterRestart.RebootRequired.Required || len(afterRestart.RebootRequired.Sources) != 1 || afterRestart.RebootRequired.Sources[0].Address != "base/packages/kernel" {
		t.Fatalf("pending reboot requirement = %+v", afterRestart.RebootRequired)
	}
}

func TestSyncRunStateExecutesRebootOnlyAfterAcknowledgement(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	state := newSyncRunState(t.TempDir(), "https://remotr.example", nil, nil)
	state.now = func() time.Time { return now }
	state.bootID = func() (string, error) { return "boot-1", nil }
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"systemctl [reboot]": {}}}
	state.rebootRunner = runner
	if _, err := state.rebootState.Prepare(rebootstate.Intent{
		Generation: "kernel-6.12.1", Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: "boot-1", PreparedAt: now, NotBefore: now, Timeout: 15 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}

	if err := state.executeAcknowledgedReboot(&sync.RebootIntentPayload{Generation: "kernel-6.12.1"}); err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "systemctl" || len(runner.Calls[0].Args) != 1 || runner.Calls[0].Args[0] != "reboot" {
		t.Fatalf("reboot commands = %+v", runner.Calls)
	}
	status, err := state.rebootState.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if status.Intent == nil || status.Intent.Phase != rebootstate.PhaseAttempting || status.Intent.AttemptGeneration != 1 {
		t.Fatalf("durable attempt = %+v", status)
	}
	if err := state.executeAcknowledgedReboot(&sync.RebootIntentPayload{Generation: "kernel-6.12.1"}); err == nil || len(runner.Calls) != 1 {
		t.Fatalf("same generation repeated: err=%v calls=%+v", err, runner.Calls)
	}
}

func TestSyncRunStateRedactsRebootCommandFailure(t *testing.T) {
	const canary = "reboot-command-secret-canary"
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	state := newSyncRunState(t.TempDir(), "https://remotr.example", nil, nil)
	state.now = func() time.Time { return now }
	state.bootID = func() (string, error) { return "boot-1", nil }
	state.rebootRunner = &executil.MockRunner{Next: map[string]executil.MockResult{"systemctl [reboot]": {Stderr: []byte(canary), Err: errors.New("exit status 1")}}}
	if _, err := state.rebootState.Prepare(rebootstate.Intent{Generation: "g1", Phase: rebootstate.PhaseAwaitingAcknowledgement, PriorBootID: "boot-1", PreparedAt: now, NotBefore: now, Timeout: time.Minute}); err != nil {
		t.Fatal(err)
	}
	err := state.executeAcknowledgedReboot(&sync.RebootIntentPayload{Generation: "g1"})
	if err == nil || strings.Contains(err.Error(), canary) {
		t.Fatalf("unsafe reboot error = %v", err)
	}
	status, loadErr := state.rebootState.Snapshot()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if status.Intent == nil || status.Intent.Phase != rebootstate.PhaseFailed || status.Intent.Reason != "reboot_command_failed" {
		t.Fatalf("failed reboot state = %+v", status)
	}
}

func TestSyncRunStateQueuesDelayedIntentAndReconcilesChangedBoot(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	bootID := "boot-1"
	state := newSyncRunState(t.TempDir(), "https://remotr.example", nil, nil)
	state.now = func() time.Time { return now }
	state.bootID = func() (string, error) { return bootID, nil }
	state.rebootRunner = &executil.MockRunner{Next: map[string]executil.MockResult{"systemctl [reboot]": {}}}
	if _, err := state.rebootState.Record([]rebootstate.Source{{Address: "base/packages/kernel"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.rebootState.Prepare(rebootstate.Intent{
		Generation: "g1", Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: bootID, PreparedAt: now, NotBefore: now.Add(2 * time.Minute), Timeout: 5 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	var pending sync.Pending
	if err := state.refreshRebootCoordination(&pending); err != nil {
		t.Fatal(err)
	}
	if pending.RebootIntent != nil {
		t.Fatalf("delayed reboot queued early: %+v", pending.RebootIntent)
	}

	now = now.Add(2 * time.Minute)
	if err := state.refreshRebootCoordination(&pending); err != nil {
		t.Fatal(err)
	}
	if pending.RebootIntent == nil || pending.RebootIntent.Generation != "g1" {
		t.Fatalf("due reboot intent = %+v", pending.RebootIntent)
	}
	if err := state.executeAcknowledgedReboot(pending.RebootIntent); err != nil {
		t.Fatal(err)
	}
	bootID = "boot-2"
	now = now.Add(time.Minute)
	if err := state.refreshRebootCoordination(&pending); err != nil {
		t.Fatal(err)
	}
	if pending.RebootRequired.Required || pending.RebootIntent != nil || !state.rebootState.Completed("g1", "boot-2") {
		t.Fatalf("reconciled pending = %+v intent=%+v", pending.RebootRequired, pending.RebootIntent)
	}
}

func TestAcknowledgedRebootIntentRequiresMatchingServerGeneration(t *testing.T) {
	intent := &sync.RebootIntentPayload{Generation: "g1"}
	request := sync.Request{RebootIntent: intent}
	for _, response := range []sync.Response{{}, {RebootAcknowledged: "other"}} {
		if got := acknowledgedRebootIntent(request, response); got != nil {
			t.Fatalf("mismatched response acknowledged reboot: %+v", response)
		}
	}
	if got := acknowledgedRebootIntent(request, sync.Response{RebootAcknowledged: "g1"}); got != intent {
		t.Fatalf("matching acknowledgement = %+v", got)
	}
}

func TestAcknowledgedNetworkIntentRequiresMatchingServerTransaction(t *testing.T) {
	intent := &sync.NetworkIntentPayload{ID: "network-1"}
	request := sync.Request{NetworkIntent: intent}
	for _, response := range []sync.Response{{}, {NetworkAcknowledged: "other"}} {
		if got := acknowledgedNetworkIntent(request, response); got != nil {
			t.Fatalf("mismatched response acknowledged network transaction: %+v", response)
		}
	}
	if got := acknowledgedNetworkIntent(request, sync.Response{NetworkAcknowledged: "network-1"}); got != intent {
		t.Fatalf("matching acknowledgement = %+v", got)
	}
}
