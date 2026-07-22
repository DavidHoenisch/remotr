package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/polling"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-AEC-116. Public seam: consecutive authenticated Sync exchanges around a
// composed-agent pipeline failure. A failed offer must not become the agent's
// acknowledged digest, so the server sends it again on the next poll.
func TestSyncRunRetriesArtifactAfterPipelineFailure(t *testing.T) {
	requests, offers := runSyncOfferScenario(t, []byte("schemaVersion: [\n"))
	if offers != 2 {
		t.Fatalf("artifact offers = %d, want retry offer after failed processing", offers)
	}
	if requests[1].LastDigest != "" || requests[1].LastReleaseRef != "" {
		t.Fatalf("failed artifact acknowledged on retry: digest=%q releaseRef=%q", requests[1].LastDigest, requests[1].LastReleaseRef)
	}
}

func TestSyncRunAcknowledgesSuccessfullyProcessedArtifact(t *testing.T) {
	requests, offers := runSyncOfferScenario(t, []byte("schemaVersion: 1\nconfigurations: []\n"))
	if offers != 1 {
		t.Fatalf("artifact offers = %d, want one successful offer", offers)
	}
	if requests[1].LastDigest != "sha256:offered" || requests[1].LastReleaseRef != "release-offered" {
		t.Fatalf("successful artifact acknowledgement: digest=%q releaseRef=%q", requests[1].LastDigest, requests[1].LastReleaseRef)
	}
}

func TestSyncRunRetriesCachedArtifactWhenUnchangedResponseCarriesExecutionLease(t *testing.T) {
	const (
		digest     = "sha256:cached"
		releaseRef = "release-cached"
	)
	now := time.Now().UTC()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request sync.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.LastDigest != digest || request.LastReleaseRef != releaseRef {
			t.Fatalf("cached acknowledgement = digest %q release %q", request.LastDigest, request.LastReleaseRef)
		}
		_ = json.NewEncoder(w).Encode(sync.Response{
			Unchanged: true, Digest: digest, ReleaseRef: releaseRef,
			ExecutionLeases: []changecontrol.ExecutionLease{{
				ID: "lease-1", ChangeRequestID: "change-1", EndpointID: "endpoint-1",
				ResourceHashes:      map[string]string{"subscriptions/primary": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				HashContractVersion: effectivehash.SchemaVersion, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
			}},
		})
	}))
	t.Cleanup(server.Close)

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} //nolint:gosec // test server
	state := newSyncRunState(t.TempDir(), server.URL, tlsConfig, nil)
	state.throttler = nil
	state.networkState = nil
	state.bootID = func() (string, error) { return "boot-test", nil }
	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86, OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu"}, nil
	}
	state.lastDigest = digest
	state.lastReleaseRef = releaseRef
	state.lastArtifactYAML = []byte("schemaVersion: [\n")
	var pending sync.Pending
	if err := state.runOnce(t.Context(), sync.NewClient(server.URL, tlsConfig), &pending, "v0.6.10"); err != nil {
		t.Fatal(err)
	}
	if pending.Drift == nil {
		t.Fatal("unchanged response lease did not retry the cached artifact")
	}
}

func runSyncOfferScenario(t *testing.T, artifact []byte) ([]sync.Request, int) {
	t.Helper()
	const (
		digest     = "sha256:offered"
		releaseRef = "release-offered"
	)
	var requests []sync.Request
	offers := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request sync.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		response := sync.Response{Digest: digest, ReleaseRef: releaseRef}
		if request.LastDigest == digest && request.LastReleaseRef == releaseRef {
			response.Unchanged = true
		} else {
			offers++
			response.ArtifactYAML = artifact
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} //nolint:gosec // test server
	state := newSyncRunState(t.TempDir(), server.URL, tlsConfig, nil)
	state.throttler = nil
	state.networkState = nil
	state.bootID = func() (string, error) { return "boot-test", nil }
	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
			OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, nil
	}
	client := sync.NewClient(server.URL, tlsConfig)
	var pending sync.Pending

	for range 2 {
		if err := state.runOnce(t.Context(), client, &pending, "v0.6.10"); err != nil {
			t.Fatal(err)
		}
	}
	if len(requests) != 2 {
		t.Fatalf("Sync requests = %d, want 2", len(requests))
	}
	return requests, offers
}

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
