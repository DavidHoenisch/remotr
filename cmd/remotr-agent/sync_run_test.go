package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/credentials"
	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/inventory"
	"github.com/DavidHoenisch/remotr/internal/agent/polling"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/types"
)

type syncAuthorityResolver struct {
	calls int
}

func (r *syncAuthorityResolver) Resolve(
	_ context.Context,
	_ secrets.ResolveRequest,
) (secrets.Resolved, error) {
	r.calls++
	return secrets.Resolved{
		Provider: secrets.ProviderRemotr,
		Version:  "1",
		Material: []byte("secret-canary"),
	}, nil
}

// OS-LSM-082, OS-LSM-083. Public seam: authenticated Sync responses change the
// authority before later artifact handling; a missing token fails closed.
func TestSyncRunObservesSecretAuthorityToken(t *testing.T) {
	var token = "first"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request sync.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(sync.Response{
			Unchanged:              true,
			SecretAuthorityToken:   token,
			AcceptedDocumentHashes: request.DocumentHashes,
		})
	}))
	t.Cleanup(server.Close)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, InsecureSkipVerify: true,
	} //nolint:gosec // test server
	stateDir := t.TempDir()
	if err := credentials.SaveState(stateDir, credentials.State{
		EndpointID: "11111111-1111-1111-1111-111111111111",
	}); err != nil {
		t.Fatal(err)
	}
	state := newSyncRunState(stateDir, server.URL, tlsConfig, nil)
	state.throttler = nil
	state.networkState = nil
	state.bootID = func() (string, error) { return "boot-test", nil }
	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
			OSID: "ubuntu", OSReleaseSourceCount: 2,
			OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, nil
	}
	delegate := &syncAuthorityResolver{}
	state.secretCache = secrets.NewAuthorityCachingResolver(
		delegate, secrets.AuthorityCacheOptions{},
	)
	request := secrets.ResolveRequest{
		Reference:       "remotr:token@active",
		ArtifactDigest:  "sha256:artifact",
		ResourceAddress: "base/resource",
		Purpose:         "test",
	}
	client := sync.NewClient(server.URL, tlsConfig)
	var pending sync.Pending

	if err := state.runOnce(t.Context(), client, &pending, "v0.6.34"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := state.secretCache.Resolve(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	if delegate.calls != 1 {
		t.Fatalf("stable token resolver calls = %d, want 1", delegate.calls)
	}

	token = "second"
	if err := state.runOnce(t.Context(), client, &pending, "v0.6.34"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.secretCache.Resolve(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if delegate.calls != 2 {
		t.Fatalf("changed token resolver calls = %d, want 2", delegate.calls)
	}

	token = ""
	if err := state.runOnce(t.Context(), client, &pending, "v0.6.34"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := state.secretCache.Resolve(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	if delegate.calls != 4 {
		t.Fatalf("missing token resolver calls = %d, want 4", delegate.calls)
	}
}

// OS-LSM-080, OS-PSA-019. Public seam: the composed agent receives an artifact,
// executes the real Check pipeline, and then completes unchanged Sync cycles
// without another request to the authenticated secret endpoint.
func TestSyncRunStableSecretArtifactResolvesOnce(t *testing.T) {
	const (
		digest     = "sha256:secret-artifact"
		releaseRef = "release-secret-artifact"
	)
	artifact := []byte(`schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: endpointSchedule
        name: nightly-backup
        lifecycle: present
        backend: cron
        schedule: "0 3 * * *"
        user: root
        argv: [/usr/bin/true]
        environment:
          - name: BACKUP_TOKEN
            secretRef: remotr:schedules/backup-token@active
`)
	secretRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sync":
			var request sync.Request
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			response := sync.Response{
				Digest: digest, ReleaseRef: releaseRef,
				RemediationPolicy:      "report",
				SecretAuthorityToken:   "stable-token",
				AcceptedDocumentHashes: request.DocumentHashes,
			}
			if request.LastDigest == digest && request.LastReleaseRef == releaseRef {
				response.Unchanged = true
			} else {
				response.ArtifactYAML = artifact
			}
			_ = json.NewEncoder(w).Encode(response)
		case "/v1/secrets/resolve":
			secretRequests++
			_ = json.NewEncoder(w).Encode(secrets.Resolved{
				Provider:             secrets.ProviderRemotr,
				Version:              "1",
				ActivationGeneration: 1,
				Material:             []byte("secret-canary"),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	stateDir := t.TempDir()
	if err := credentials.SaveState(stateDir, credentials.State{
		EndpointID: "11111111-1111-1111-1111-111111111111",
	}); err != nil {
		t.Fatal(err)
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, InsecureSkipVerify: true,
	} //nolint:gosec // test server
	state := newSyncRunState(stateDir, server.URL, tlsConfig, nil)
	state.throttler = nil
	state.networkState = nil
	state.bootID = func() (string, error) { return "boot-test", nil }
	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
			OSID: "ubuntu", OSReleaseSourceCount: 2,
			OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, nil
	}
	client := sync.NewClient(server.URL, tlsConfig)
	var pending sync.Pending
	for range 10 {
		if err := state.runOnce(t.Context(), client, &pending, "v0.6.34"); err != nil {
			t.Fatal(err)
		}
	}
	if secretRequests != 1 {
		t.Fatalf("secret resolution requests = %d, want 1", secretRequests)
	}
}

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

// OS-USF-004: only a server acknowledgement permits the agent to replace a
// repeatable full document with its hash, including after an agent restart.
func TestSyncRunElidesAcknowledgedDocumentsAndRestoresHashesAfterRestart(t *testing.T) {
	stateDir := t.TempDir()
	if err := credentials.SaveState(stateDir, credentials.State{EndpointID: "11111111-1111-1111-1111-111111111111"}); err != nil {
		t.Fatal(err)
	}
	var requests []sync.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request sync.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		if request.DocumentHashes == nil {
			t.Fatal("Sync request omitted document hashes")
		}
		_ = json.NewEncoder(w).Encode(sync.Response{
			Unchanged: true,
			AcceptedDocumentHashes: &documenthash.Summary{
				Version:   documenthash.CurrentVersion,
				Documents: request.DocumentHashes.Documents,
			},
		})
	}))
	t.Cleanup(server.Close)

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} //nolint:gosec // test server
	newState := func() syncRunState {
		state := newSyncRunState(stateDir, server.URL, tlsConfig, nil)
		state.throttler = nil
		state.networkState = nil
		state.bootID = func() (string, error) { return "boot-test", nil }
		state.readCapabilityFacts = func() (facts.Facts, error) {
			return facts.Facts{
				Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
				OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
			}, nil
		}
		return state
	}
	client := sync.NewClient(server.URL, tlsConfig)
	state := newState()
	var pending sync.Pending
	pending.SetSystemInfo("inventory-digest", json.RawMessage(`{"cpu":{"modelName":"Test CPU"}}`))
	if err := state.runOnce(t.Context(), client, &pending, "v0.6.10"); err != nil {
		t.Fatal(err)
	}
	if err := state.runOnce(t.Context(), client, &pending, "v0.6.10"); err != nil {
		t.Fatal(err)
	}
	restarted := newState()
	if err := restarted.runOnce(t.Context(), client, &pending, "v0.6.10"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requests))
	}
	if requests[0].CapabilityDocument == nil || requests[0].SystemInfo == nil {
		t.Fatalf("initial request omitted full documents: %+v", requests[0])
	}
	if len(requests[0].Labels) == 0 || len(requests[0].Usernames) == 0 {
		t.Fatalf("initial request omitted full targeting document: %+v", requests[0])
	}
	for _, name := range []string{documenthash.Capability, documenthash.SystemInformation, documenthash.Delivery, documenthash.Targeting} {
		if requests[0].DocumentHashes.Documents[name] == "" {
			t.Fatalf("initial request omitted %s hash: %+v", name, requests[0].DocumentHashes)
		}
	}
	for index, request := range requests[1:] {
		if request.CapabilityDocument != nil || request.SystemInfo != nil || len(request.Labels) != 0 || len(request.Usernames) != 0 {
			t.Fatalf("request %d repeated acknowledged full documents: %+v", index+2, request)
		}
		for _, name := range []string{documenthash.Capability, documenthash.SystemInformation, documenthash.Delivery, documenthash.Targeting} {
			if request.DocumentHashes.Documents[name] == "" {
				t.Fatalf("request %d omitted %s hash: %+v", index+2, name, request.DocumentHashes)
			}
		}
	}
}

// OS-USF-001. Public seam: consecutive authenticated Sync exchanges from the
// composed agent. A stable compliance snapshot is transition telemetry: after
// one successful report, the next unchanged poll must be telemetry-free so it
// can use the server's unchanged Sync fast path.
func TestSyncRunOmitsRepeatedComplianceSnapshotFromQuietSync(t *testing.T) {
	const (
		digest        = "sha256:stable"
		releaseRef    = "release-stable"
		changedDigest = "sha256:changed"
		changedRef    = "release-changed"
	)
	stateDir := t.TempDir()
	if err := credentials.SaveState(stateDir, credentials.State{EndpointID: "11111111-1111-1111-1111-111111111111"}); err != nil {
		t.Fatal(err)
	}
	var requests []sync.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request sync.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		response := sync.Response{
			Digest: digest, ReleaseRef: releaseRef,
			AcceptedDocumentHashes: request.DocumentHashes,
		}
		if len(requests) == 3 {
			response.Digest = changedDigest
			response.ReleaseRef = changedRef
			response.ArtifactYAML = []byte("schemaVersion: 1\nconfigurations: []\n")
		} else if request.LastDigest == digest && request.LastReleaseRef == releaseRef || request.LastDigest == changedDigest && request.LastReleaseRef == changedRef {
			response.Unchanged = true
		} else {
			response.ArtifactYAML = []byte("schemaVersion: 1\nconfigurations: []\n")
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} //nolint:gosec // test server
	state := newSyncRunState(stateDir, server.URL, tlsConfig, nil)
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
	for range 4 {
		if err := state.runOnce(t.Context(), client, &pending, "v0.6.30"); err != nil {
			t.Fatal(err)
		}
	}
	if len(requests) != 4 {
		t.Fatalf("Sync requests = %d, want 4", len(requests))
	}
	if requests[1].Drift == nil {
		t.Fatal("first compliance snapshot was not reported")
	}
	if requests[2].Drift != nil {
		t.Fatal("stable compliance snapshot was repeated on quiet Sync")
	}
	if requests[2].CapabilityDocument != nil || len(requests[2].Labels) != 0 || len(requests[2].Usernames) != 0 {
		t.Fatalf("quiet Sync repeated accepted documents: %+v", requests[2])
	}
	if requests[3].Drift == nil || requests[3].Drift.Digest != changedDigest {
		t.Fatalf("changed compliance snapshot was suppressed: %+v", requests[3].Drift)
	}
}

// OS-USF-001. Public seam: volatile inventory readings must not turn an
// otherwise quiet authenticated Sync into a durable server transaction.
func TestSyncRunIgnoresVolatileInventoryChangesOnQuietSync(t *testing.T) {
	stateDir := t.TempDir()
	if err := credentials.SaveState(stateDir, credentials.State{
		EndpointID: "11111111-1111-1111-1111-111111111111",
	}); err != nil {
		t.Fatal(err)
	}
	var requests []sync.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		var request sync.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		if err := json.NewEncoder(w).Encode(sync.Response{
			Digest:                 "sha256:stable",
			ReleaseRef:             "release-stable",
			Unchanged:              true,
			AcceptedDocumentHashes: request.DocumentHashes,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	snapshots := []inventory.Snapshot{
		{
			CPU: inventory.CPUInfo{ModelName: "Test CPU", CoreCount: "4"},
			RAM: inventory.RAMInfo{
				MemTotal: "16384000 kB", MemFree: "8192000 kB",
				MemAvailable: "12288000 kB",
			},
			Batteries: []inventory.BatteryInfo{{
				Name: "BAT0", Status: "Discharging", Capacity: "82",
				CapacityLevel: "Normal", PowerNow: "7420000",
				Technology: "Li-ion",
			}},
			Firewall: inventory.FirewallInfo{
				Backend: "nftables",
				Nftables: &inventory.NftablesInfo{
					RawRuleset: "tcp dport 443 counter packets 10 bytes 640 accept\n",
				},
			},
		},
		{
			CPU: inventory.CPUInfo{ModelName: "Test CPU", CoreCount: "4"},
			RAM: inventory.RAMInfo{
				MemTotal: "16384000 kB", MemFree: "4096000 kB",
				MemAvailable: "6144000 kB",
			},
			Batteries: []inventory.BatteryInfo{{
				Name: "BAT0", Status: "Charging", Capacity: "83",
				CapacityLevel: "High", PowerNow: "12500000",
				Technology: "Li-ion",
			}},
			Firewall: inventory.FirewallInfo{
				Backend: "nftables",
				Nftables: &inventory.NftablesInfo{
					RawRuleset: "tcp dport 443 counter packets 18 bytes 1152 accept\n",
				},
			},
		},
	}
	collected := 0
	now := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, InsecureSkipVerify: true,
	} //nolint:gosec // test server
	state := newSyncRunState(stateDir, server.URL, tlsConfig, nil)
	state.lastDigest = "sha256:stable"
	state.lastReleaseRef = "release-stable"
	state.networkState = nil
	state.bootID = func() (string, error) { return "boot-test", nil }
	state.now = func() time.Time { return now }
	state.collectInventory = func() inventory.Snapshot {
		snapshot := snapshots[collected]
		collected++
		return snapshot
	}
	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
			OSID: "ubuntu", OSReleaseSourceCount: 2,
			OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, nil
	}
	client := sync.NewClient(server.URL, tlsConfig)
	var pending sync.Pending
	if err := state.runOnce(t.Context(), client, &pending, "v0.6.35"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	if err := state.runOnce(t.Context(), client, &pending, "v0.6.35"); err != nil {
		t.Fatal(err)
	}

	if len(requests) != 2 {
		t.Fatalf("Sync requests = %d, want 2", len(requests))
	}
	if requests[0].SystemInfo == nil {
		t.Fatal("initial Sync omitted inventory")
	}
	if requests[1].SystemInfo != nil {
		t.Fatal("volatile inventory readings forced a full Sync")
	}
	if requests[1].DocumentHashes == nil ||
		requests[1].DocumentHashes.Documents[documenthash.SystemInformation] == "" {
		t.Fatal("quiet Sync omitted accepted inventory hash")
	}
}

// OS-USF-001. Public seam: consecutive authenticated Sync exchanges from the
// composed agent. A firewall audit is transition telemetry: after one
// successful report, the next byte-identical audit must be omitted, while a
// changed audit must still be delivered.
func TestSyncRunOmitsRepeatedFirewallAuditFromQuietSync(t *testing.T) {
	const (
		digest     = "sha256:stable"
		releaseRef = "release-stable"
	)
	stateDir := t.TempDir()
	if err := credentials.SaveState(stateDir, credentials.State{EndpointID: "11111111-1111-1111-1111-111111111111"}); err != nil {
		t.Fatal(err)
	}
	var requests []sync.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request sync.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		if err := json.NewEncoder(w).Encode(sync.Response{
			Digest: digest, ReleaseRef: releaseRef, Unchanged: true,
			AcceptedDocumentHashes: request.DocumentHashes,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} //nolint:gosec // test server
	state := newSyncRunState(stateDir, server.URL, tlsConfig, nil)
	state.lastDigest = digest
	state.lastReleaseRef = releaseRef
	state.throttler = nil
	state.networkState = nil
	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
			OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, nil
	}
	client := sync.NewClient(server.URL, tlsConfig)
	var pending sync.Pending
	for _, audit := range []struct {
		digest string
		report string
	}{
		{digest: "sha256:audit-stable", report: `{"rules":"stable"}`},
		{digest: "sha256:audit-stable", report: `{"rules":"stable"}`},
		{digest: "sha256:audit-changed", report: `{"rules":"changed"}`},
	} {
		pending.SetFirewallAudit(audit.digest, json.RawMessage(audit.report))
		if err := state.runOnce(t.Context(), client, &pending, "v0.6.32"); err != nil {
			t.Fatal(err)
		}
	}
	if requests[0].FirewallAudit == nil {
		t.Fatal("first firewall audit was not reported")
	}
	if requests[1].FirewallAudit != nil {
		t.Fatal("stable firewall audit was repeated on quiet Sync")
	}
	if requests[1].CapabilityDocument != nil || len(requests[1].Labels) != 0 || len(requests[1].Usernames) != 0 {
		t.Fatalf("quiet Sync repeated accepted documents: %+v", requests[1])
	}
	if requests[2].FirewallAudit == nil || requests[2].FirewallAudit.Digest != "sha256:audit-changed" {
		t.Fatalf("changed firewall audit was suppressed: %+v", requests[2].FirewallAudit)
	}
}

func TestRepeatableDocumentsResendAfterLostResponseChangeOrServerRequest(t *testing.T) {
	state := newSyncRunState("", "https://remotr.example", nil, nil)
	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
			OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, nil
	}
	capability, err := state.currentCapabilityDocument("v0.6.10")
	if err != nil {
		t.Fatal(err)
	}
	requestWithSystemInfo := func(report string) sync.Request {
		return sync.Request{
			Labels:     map[string]string{"distro": "ubuntu", "arch": "x86"},
			Usernames:  []string{"operator"},
			SystemInfo: &sync.SystemInfoPayload{Digest: "inventory-digest", Report: json.RawMessage(report)},
		}
	}
	first, err := state.attachRepeatableDocuments(requestWithSystemInfo(`{"cpu":"first"}`), capability)
	if err != nil {
		t.Fatal(err)
	}
	lostResponseRetry, err := state.attachRepeatableDocuments(requestWithSystemInfo(`{"cpu":"first"}`), capability)
	if err != nil {
		t.Fatal(err)
	}
	if first.CapabilityDocument == nil || first.SystemInfo == nil || lostResponseRetry.CapabilityDocument == nil || lostResponseRetry.SystemInfo == nil {
		t.Fatalf("lost response suppressed a full document: first=%+v retry=%+v", first, lostResponseRetry)
	}
	if err := state.acceptDocumentHashes(first, sync.Response{AcceptedDocumentHashes: first.DocumentHashes}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := state.attachRepeatableDocuments(requestWithSystemInfo(`{"cpu":"first"}`), capability)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.CapabilityDocument != nil || unchanged.SystemInfo != nil || len(unchanged.Labels) != 0 || len(unchanged.Usernames) != 0 {
		t.Fatalf("acknowledged documents were not elided: %+v", unchanged)
	}
	changedSystemInfo, err := state.attachRepeatableDocuments(requestWithSystemInfo(`{"cpu":"second"}`), capability)
	if err != nil {
		t.Fatal(err)
	}
	if changedSystemInfo.CapabilityDocument != nil || changedSystemInfo.SystemInfo == nil {
		t.Fatalf("changed system information request = %+v", changedSystemInfo)
	}

	state.readCapabilityFacts = func() (facts.Facts, error) {
		return facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.Arm,
			OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, nil
	}
	changedCapability, err := state.currentCapabilityDocument("v0.6.10")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := state.attachRepeatableDocuments(sync.Request{
		Labels: map[string]string{"distro": "ubuntu", "arch": "arm"}, Usernames: []string{"operator"},
	}, changedCapability)
	if err != nil {
		t.Fatal(err)
	}
	if changed.CapabilityDocument == nil || changed.DocumentHashes.Documents[documenthash.Capability] == first.DocumentHashes.Documents[documenthash.Capability] {
		t.Fatalf("changed capability request = %+v", changed)
	}
	if len(changed.Labels) == 0 || changed.DocumentHashes.Documents[documenthash.Targeting] == first.DocumentHashes.Documents[documenthash.Targeting] {
		t.Fatalf("changed targeting request = %+v", changed)
	}

	deliveryChanged, err := state.attachRepeatableDocuments(sync.Request{
		LastReleaseRef: "release-next", LastDigest: "sha256:next",
		Labels: map[string]string{"distro": "ubuntu", "arch": "x86"}, Usernames: []string{"operator"},
	}, capability)
	if err != nil {
		t.Fatal(err)
	}
	if deliveryChanged.DocumentHashes.Documents[documenthash.Delivery] == first.DocumentHashes.Documents[documenthash.Delivery] {
		t.Fatalf("changed delivery request = %+v", deliveryChanged)
	}

	if err := state.acceptDocumentHashes(unchanged, sync.Response{RequestedDocuments: []string{documenthash.Capability, documenthash.Targeting}}); err != nil {
		t.Fatal(err)
	}
	requested, err := state.attachRepeatableDocuments(sync.Request{
		Labels: map[string]string{"distro": "ubuntu", "arch": "x86"}, Usernames: []string{"operator"},
	}, capability)
	if err != nil {
		t.Fatal(err)
	}
	if requested.CapabilityDocument == nil || len(requested.Labels) == 0 || len(requested.Usernames) == 0 {
		t.Fatal("server full-document request did not restore capability and targeting uploads")
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
