package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestAdminChangeControlLifecycleAndBaselineAdoption(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	ids := []string{"request-1", "rollout-1", "adoption-1"}
	index := 0
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{Now: func() time.Time { return now }, NewID: func() string { id := ids[index]; index++; return id }})
	created, err := changes.CreateChangeRequests(changecontrol.FleetPlan{
		Fleet: "engineering", ReleaseRef: "release-1", ArtifactDigest: "artifact-1",
		Targets:   []changecontrol.TargetEvidence{{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true}},
		Resources: []changecontrol.ResourcePlan{{Address: "base/firewall", DesiredHash: "hash-1", Risk: models.RiskConnectivity, Provider: "nftables", BaselineEligible: true}},
	}, "operator-seed")
	if err != nil || len(created) != 1 {
		t.Fatalf("seed request: %+v %v", created, err)
	}
	auditLog := &mockAuditLog{}
	srv := New(withDerivedSudoPlan(t, Config{Admin: admin, AuditLog: auditLog, ChangeControl: changes, CACert: caCert, CAKey: caKey, CACertPEM: caPEM}))

	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	if rec := request(http.MethodGet, "/v1/admin/change-requests", ""); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("request-1")) {
		t.Fatalf("list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodGet, "/v1/admin/change-requests/request-1", ""); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("hash-1")) {
		t.Fatalf("show: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodPost, "/v1/admin/change-requests/request-1/authorize", `{"attempt_limit":2,"max_concurrency":1,"justification":"CHG-42"}`); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("rollout-1")) {
		t.Fatalf("authorize: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if event := auditLog.events[len(auditLog.events)-1]; event.Action != audit.ActionAdminChangeAuthorize || event.ResourceType != "change_request" || event.ResourceID != "request-1" || event.StatusCode != http.StatusOK {
		t.Fatalf("authorize audit = %#v, want exact successful Change request", event)
	}
	for _, action := range []string{"pause", "resume", "revoke"} {
		rec := request(http.MethodPost, "/v1/admin/change-requests/request-1/"+action, "")
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(actionState(action))) {
			t.Fatalf("%s: status=%d body=%s", action, rec.Code, rec.Body.String())
		}
	}

	rec := request(http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", `{}`)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("baseline-adoption")) {
		t.Fatalf("baseline adoption: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// OS-AEC-086: an Admin client cannot replace the composed canonical resource
// identity with its own hash or other authoritative plan facts.
func TestAdminBaselineAdoptionRejectsCallerSuppliedConflictingHash(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	adminRegistry := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "55555555-5555-5555-5555-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	if err := adminRegistry.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "must-not-store" }})
	srv := New(Config{
		Admin: adminRegistry, ChangeControl: changes,
		CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
	})
	body, err := json.Marshal(changecontrol.FleetPlan{
		ReleaseRef: "caller-release", ArtifactDigest: "caller-artifact",
		Resources: []changecontrol.ResourcePlan{{
			Address: "base/sudo", DesiredHash: "sha256:caller-conflict",
			Risk: models.RiskAccess, Provider: "sudo",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("caller-authored plan: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if requests := changes.List(); len(requests) != 0 {
		t.Fatalf("stored caller-authored requests = %+v", requests)
	}
}

func TestAdminBaselineAdoptionDerivesCanonicalPlanFromServerArtifact(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	adminRegistry := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "66666666-6666-6666-6666-666666666666")
	if err != nil {
		t.Fatal(err)
	}
	if err := adminRegistry.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	artifact := []byte(`
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: sudo
        name: operators
        lifecycle: present
        ownership: fragment
        subjects: ["%operators"]
        commands: [ALL]
        recoveryPrincipals: [recovery]
`)
	state, err := models.ParseState(bytes.NewReader(artifact))
	if err != nil {
		t.Fatal(err)
	}
	identities, err := configcompose.EffectiveResources(t.Context(), state, map[string]configcompose.ProviderSelection{
		"base/operators": {ID: "sudo"},
	}, "sha256:artifact", nil)
	if err != nil || len(identities) != 1 {
		t.Fatalf("independent canonical identity: %+v %v", identities, err)
	}
	for _, endpointID := range []string{"endpoint-blocked", "endpoint-digest", "endpoint-missing", "endpoint-ready", "endpoint-stale"} {
		if err := adminRegistry.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"}); err != nil {
			t.Fatal(err)
		}
	}
	reportItem := registry.StateReportItem{
		Address: "base/operators", Provider: "sudo", ProviderRevision: "sudo-v1", EffectiveHash: identities[0].EffectiveHash,
		Status: registry.StateDrifted, PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
	}
	adminRegistry.SetEndpointStateReport("endpoint-ready", registry.DriftSummary{ReleaseRef: "release-1", Digest: "sha256:artifact", ReportedAt: time.Unix(1, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{reportItem}})
	blockedItem := reportItem
	blockedItem.PreflightStatus = registry.PlanPreflightBlocked
	blockedItem.PreflightReason = "preflight_failed"
	adminRegistry.SetEndpointStateReport("endpoint-blocked", registry.DriftSummary{ReleaseRef: "release-1", Digest: "sha256:artifact", ReportedAt: time.Unix(2, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{blockedItem}})
	adminRegistry.SetEndpointStateReport("endpoint-stale", registry.DriftSummary{ReleaseRef: "release-old", Digest: "sha256:artifact", ReportedAt: time.Unix(3, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{reportItem}})
	adminRegistry.SetEndpointStateReport("endpoint-digest", registry.DriftSummary{ReleaseRef: "release-1", Digest: "sha256:old-artifact", ReportedAt: time.Unix(4, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{reportItem}})
	ids := []string{"derived-request", "must-not-create"}
	idIndex := 0
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { id := ids[idIndex]; idIndex++; return id }})
	srv := New(Config{
		Admin: adminRegistry, ChangeControl: changes,
		StateReports:  adminRegistry,
		ReleaseRef:    "release-1",
		ArtifactStore: derivedPlanArtifactStore{artifact: artifact, digest: "sha256:artifact"},
		CACert:        caCert, CAKey: caKey, CACertPEM: caPEM,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", bytes.NewBufferString(`{}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("derived adoption: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var request changecontrol.ChangeRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.ID != "derived-request" || request.ReleaseRef != "release-1" || request.ArtifactDigest != "sha256:artifact" || request.HashContractVersion != effectivehash.SchemaVersion {
		t.Fatalf("request identity = %+v", request)
	}
	if len(request.Resources) != 1 || effectivehash.Validate(request.Resources[0].DesiredHash) != nil || request.Resources[0].ProviderRevision != "sudo-v1" || request.Resources[0].PredictedEffects[0].Code != changecontrol.EffectSudoPolicyReplace {
		t.Fatalf("derived resource = %+v", request.Resources)
	}
	if len(request.FrozenTargets) != 5 {
		t.Fatalf("frozen endpoint evidence = %+v", request.FrozenTargets)
	}
	if target := request.FrozenTargets[0]; target.EndpointID != "endpoint-blocked" || !target.Compatible || target.PreflightReady || target.PreflightReason != "preflight_failed" {
		t.Fatalf("blocked target = %+v", target)
	}
	if target := request.FrozenTargets[1]; target.EndpointID != "endpoint-digest" || target.Compatible || target.PreflightReady || target.PreflightReason != "artifact_digest_mismatch" {
		t.Fatalf("digest target = %+v", target)
	}
	if target := request.FrozenTargets[2]; target.EndpointID != "endpoint-missing" || target.Compatible || target.PreflightReady || target.PreflightReason != "state_report_missing" {
		t.Fatalf("missing target = %+v", target)
	}
	if target := request.FrozenTargets[3]; target.EndpointID != "endpoint-ready" || !target.Compatible || !target.PreflightReady || target.PreflightReason != "" {
		t.Fatalf("ready target = %+v", target)
	}
	if target := request.FrozenTargets[4]; target.EndpointID != "endpoint-stale" || target.Compatible || target.PreflightReady || target.PreflightReason != "release_mismatch" {
		t.Fatalf("stale target = %+v", target)
	}

	mismatchedItem := reportItem
	mismatchedItem.EffectiveHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	for index, endpointID := range []string{"endpoint-blocked", "endpoint-ready"} {
		adminRegistry.SetEndpointStateReport(endpointID, registry.DriftSummary{ReleaseRef: "release-1", Digest: "sha256:artifact", ReportedAt: time.Unix(int64(5+index), 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{mismatchedItem}})
	}
	second := httptest.NewRequest(http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", bytes.NewBufferString(`{}`))
	second.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	secondResponse := httptest.NewRecorder()
	srv.Handler().ServeHTTP(secondResponse, second)
	if secondResponse.Code != http.StatusBadRequest || len(changes.List()) != 1 {
		t.Fatalf("mismatched current endpoint hash: status=%d body=%s requests=%+v", secondResponse.Code, secondResponse.Body.String(), changes.List())
	}
}

// Task 4.7: persisted caller-authored authority stays visible and
// non-enforcing; replacement is an explicit, separately approved canonical
// request derived from current server and endpoint evidence.
func TestAdminRegenerateLegacyAuthorizationCreatesSeparateCanonicalPendingRequest(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	adminRegistry := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "77777777-7777-7777-7777-777777777777")
	if err != nil {
		t.Fatal(err)
	}
	if err := adminRegistry.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	if err := adminRegistry.RegisterEndpoint(registry.Endpoint{ID: "endpoint-current", Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	artifact := []byte(`
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: sudo
        name: operators
        lifecycle: present
        ownership: fragment
        subjects: ["%operators"]
        commands: [ALL]
        recoveryPrincipals: [recovery]
`)
	state, err := models.ParseState(bytes.NewReader(artifact))
	if err != nil {
		t.Fatal(err)
	}
	identities, err := configcompose.EffectiveResources(t.Context(), state, map[string]configcompose.ProviderSelection{
		"base/operators": {ID: "sudo"},
	}, "sha256:artifact", nil)
	if err != nil || len(identities) != 1 {
		t.Fatalf("canonical identities = %+v, %v", identities, err)
	}
	adminRegistry.SetEndpointStateReport("endpoint-current", registry.DriftSummary{ReleaseRef: "release-current", Digest: "sha256:artifact", ReportedAt: time.Unix(1, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{{
		Address: "base/operators", Provider: "sudo", ProviderRevision: "sudo-v1", EffectiveHash: identities[0].EffectiveHash,
		Status: registry.StateDrifted, PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
	}}})

	store := &legacyChangeStateStore{revision: 1, payload: []byte(`{
  "version": 1,
  "requests": {"legacy-request": {
    "id": "legacy-request", "fleet": "engineering", "release_ref": "release-legacy", "artifact_digest": "legacy-artifact",
    "authorization_group": "access", "risk": "access",
    "resources": [{"address": "base/operators", "desired_hash": "caller-authored-legacy-hash", "risk": "access", "provider": "sudo", "authorization_group": "access", "predicted_effects": ["legacy effect"], "rollback_class": "best_effort", "baseline_eligible": true}],
    "resource_hashes": {"base/operators": "caller-authored-legacy-hash"},
    "frozen_targets": [{"endpoint_id": "endpoint-current", "compatible": true, "preflight_ready": true}],
    "authorization_state": "authorized", "required_approvals": 1,
    "approvals": [{"operator_id": "legacy-operator", "approved_at": "2026-07-16T12:00:00Z"}],
    "audit_history": [{"at": "2026-07-16T12:00:00Z", "actor_id": "legacy-operator", "action": "created"}],
    "created_at": "2026-07-16T12:00:00Z"
  }},
  "rollouts": {"legacy-request": {"id": "legacy-rollout", "change_request_id": "legacy-request", "fleet": "engineering", "resource_hashes": {"base/operators": "caller-authored-legacy-hash"}, "frozen_targets": [{"endpoint_id": "endpoint-current", "compatible": true, "preflight_ready": true}], "valid_from": "2026-07-16T12:00:00Z", "valid_until": "2026-08-16T12:00:00Z", "attempt_limit": 1, "max_concurrency": 1, "authorized_by": "legacy-operator", "authorized_at": "2026-07-16T12:00:00Z"}},
  "baselines": {"engineering\u0000base/operators": {"id": "legacy-baseline", "change_request_id": "legacy-request", "fleet": "engineering", "resource_address": "base/operators", "desired_hash": "caller-authored-legacy-hash", "risk": "access", "provider": "sudo", "authorized_by": "legacy-operator", "authorized_at": "2026-07-16T12:00:00Z", "audit_history": []}},
  "policy": {}, "automatic_promotion": {}, "leases": {}, "attempts": {}, "break_glass": {}
}`)}
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	ids := []string{"replacement-request", "must-not-create"}
	idIndex := 0
	changes, err := changecontrol.NewPersistentRegistry(t.Context(), store, changecontrol.RegistryOptions{
		Now:   func() time.Time { return now },
		NewID: func() string { id := ids[idIndex]; idIndex++; return id },
	})
	if err != nil {
		t.Fatal(err)
	}
	if changes.RolloutActive("legacy-request", now) || changes.BaselineAuthorizes("engineering", "base/operators", "caller-authored-legacy-hash", "sudo", true) {
		t.Fatal("restored legacy authority is enforcing before regeneration")
	}
	srv := New(Config{
		Admin: adminRegistry, ChangeControl: changes, StateReports: adminRegistry,
		ReleaseRef: "release-current", ArtifactStore: derivedPlanArtifactStore{artifact: artifact, digest: "sha256:artifact"},
		CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
	})
	request := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/change-requests/legacy-request/regenerate", bytes.NewBufferString(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := request(`{"resource_hashes":{"base/operators":"caller"}}`); rec.Code != http.StatusBadRequest || len(changes.List()) != 1 {
		t.Fatalf("caller-authored regeneration: status=%d body=%s requests=%+v", rec.Code, rec.Body.String(), changes.List())
	}
	rec := request(`{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("regenerate legacy authorization: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		LegacyRequest      changecontrol.ChangeRequest `json:"legacy_request"`
		ReplacementRequest changecontrol.ChangeRequest `json:"replacement_request"`
		Comparison         struct {
			CanonicalReleaseRef     string `json:"canonical_release_ref"`
			CanonicalArtifactDigest string `json:"canonical_artifact_digest"`
			Resources               []struct {
				Address       string `json:"address"`
				Status        string `json:"status"`
				CanonicalHash string `json:"canonical_hash"`
			} `json:"resources"`
		} `json:"comparison"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.LegacyRequest.ID != "legacy-request" || result.LegacyRequest.ResourceHashes["base/operators"] != "caller-authored-legacy-hash" || result.LegacyRequest.LegacyMigration == nil || result.LegacyRequest.LegacyMigration.Enforcement != "non_enforcing" || result.LegacyRequest.LegacyMigration.Replacement != "regenerated" {
		t.Fatalf("legacy request = %+v", result.LegacyRequest)
	}
	if result.ReplacementRequest.ID != "replacement-request" || result.ReplacementRequest.AuthorizationState != changecontrol.AuthorizationPending || result.ReplacementRequest.HashContractVersion != effectivehash.SchemaVersion || len(result.ReplacementRequest.Approvals) != 0 || result.ReplacementRequest.ResourceHashes["base/operators"] == "caller-authored-legacy-hash" {
		t.Fatalf("replacement request = %+v", result.ReplacementRequest)
	}
	if result.Comparison.CanonicalReleaseRef != "release-current" || result.Comparison.CanonicalArtifactDigest != "sha256:artifact" || len(result.Comparison.Resources) != 1 || result.Comparison.Resources[0].Address != "base/operators" || result.Comparison.Resources[0].Status != "changed" || result.Comparison.Resources[0].CanonicalHash != identities[0].EffectiveHash {
		t.Fatalf("comparison = %+v", result.Comparison)
	}
	restored, err := changecontrol.NewPersistentRegistry(t.Context(), store, changecontrol.RegistryOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	legacy, ok := restored.Get("legacy-request")
	if !ok || legacy.LegacyMigration == nil || legacy.LegacyMigration.Replacement != "regenerated" || legacy.ResourceHashes["base/operators"] != "caller-authored-legacy-hash" {
		t.Fatalf("restored legacy comparison = %+v, exists=%t", legacy, ok)
	}
	replacement, ok := restored.Get("replacement-request")
	if !ok || replacement.AuthorizationState != changecontrol.AuthorizationPending || replacement.HashContractVersion != effectivehash.SchemaVersion || len(replacement.Approvals) != 0 {
		t.Fatalf("restored replacement = %+v, exists=%t", replacement, ok)
	}
}

func TestAdminBaselineAdoptionPreservesNormalDependencyReservationBlock(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	adminRegistry := registry.NewMemory()
	if err := adminRegistry.RegisterEndpoint(registry.Endpoint{ID: "endpoint", Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if err := adminRegistry.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	artifact := []byte(`
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: file
        name: config
        path: /etc/remotr/config
        content: managed
      - kind: sudo
        name: operators
        lifecycle: present
        ownership: fragment
        subjects: ["%operators"]
        commands: [ALL]
        recoveryPrincipals: [recovery]
        dependsOn: [base/config]
`)
	state, err := models.ParseState(bytes.NewReader(artifact))
	if err != nil {
		t.Fatal(err)
	}
	identities, err := configcompose.EffectiveResources(t.Context(), state, map[string]configcompose.ProviderSelection{
		"base/config": {ID: "files"}, "base/operators": {ID: "sudo"},
	}, "sha256:artifact", nil)
	if err != nil || len(identities) != 2 {
		t.Fatalf("canonical identities = %+v, %v", identities, err)
	}
	byAddress := make(map[string]configcompose.EffectiveResource, len(identities))
	for _, resource := range identities {
		byAddress[resource.Address] = resource
	}
	adminRegistry.SetEndpointStateReport("endpoint", registry.DriftSummary{ReleaseRef: "release-1", Digest: "sha256:artifact", ReportedAt: time.Unix(1, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{
		{
			Address: "base/config", Provider: byAddress["base/config"].ProviderID, ProviderRevision: byAddress["base/config"].ProviderRevision, EffectiveHash: byAddress["base/config"].EffectiveHash,
			Status: registry.StateDrifted, PreflightStatus: registry.PlanPreflightBlocked, PreflightReason: "rollback_reservation_failed",
		},
		{
			Address: "base/operators", Provider: byAddress["base/operators"].ProviderID, ProviderRevision: byAddress["base/operators"].ProviderRevision, EffectiveHash: byAddress["base/operators"].EffectiveHash,
			Status: registry.StateDrifted, PreflightStatus: registry.PlanPreflightBlocked, PreflightReason: "dependency_blocked",
		},
	}})
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "dependency-request" }})
	srv := New(Config{
		Admin: adminRegistry, ChangeControl: changes, StateReports: adminRegistry,
		ReleaseRef: "release-1", ArtifactStore: derivedPlanArtifactStore{artifact: artifact, digest: "sha256:artifact"},
		CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", bytes.NewBufferString(`{}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dependency adoption: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var request changecontrol.ChangeRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Resources) != 2 || request.Resources[0].Address != "base/config" || request.Resources[1].Address != "base/operators" {
		t.Fatalf("dependency closure = %+v", request.Resources)
	}
	if len(request.FrozenTargets) != 1 || request.FrozenTargets[0].PreflightReady || request.FrozenTargets[0].PreflightReason != "dependency_blocked" || len(request.FrozenTargets[0].ResourcePreflights) != 1 || request.FrozenTargets[0].ResourcePreflights[0].Address != "base/operators" || request.FrozenTargets[0].ResourcePreflights[0].Reason != "dependency_blocked" {
		t.Fatalf("dependency target evidence = %+v", request.FrozenTargets)
	}
}

type derivedPlanArtifactStore struct {
	artifact []byte
	digest   string
}

type legacyChangeStateStore struct {
	payload  []byte
	revision int64
}

func (s *legacyChangeStateStore) LoadChangeControlState(context.Context) ([]byte, int64, error) {
	return append([]byte(nil), s.payload...), s.revision, nil
}

func (s *legacyChangeStateStore) SaveChangeControlState(_ context.Context, expectedRevision int64, payload []byte) (int64, error) {
	if expectedRevision != s.revision {
		return 0, errors.New("revision conflict")
	}
	s.payload = append([]byte(nil), payload...)
	s.revision++
	return s.revision, nil
}

func (s derivedPlanArtifactStore) GetCompiledArtifactForFleet(context.Context, string, string, string) ([]byte, string, error) {
	return append([]byte(nil), s.artifact...), s.digest, nil
}
func (derivedPlanArtifactStore) GetCompiledArtifactForEndpoint(context.Context, string, string, string) ([]byte, string, error) {
	return nil, "", errors.New("endpoint artifact not expected")
}
func (derivedPlanArtifactStore) StoreCompiledArtifactForFleet(context.Context, string, string, string, []byte, string) error {
	return nil
}
func (derivedPlanArtifactStore) StoreCompiledArtifactForEndpoint(context.Context, string, string, string, []byte, string) error {
	return nil
}
func (derivedPlanArtifactStore) PruneOldCompiledArtifacts(context.Context, time.Time) error {
	return nil
}

func withDerivedSudoPlan(t *testing.T, config Config) Config {
	t.Helper()
	config.ReleaseRef = "release-1"
	store := derivedPlanArtifactStore{
		digest: "sha256:artifact",
		artifact: []byte(`
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: sudo
        name: operators
        lifecycle: present
        ownership: fragment
        subjects: ["%operators"]
        commands: [ALL]
        recoveryPrincipals: [recovery]
`),
	}
	config.ArtifactStore = store
	state, err := models.ParseState(bytes.NewReader(store.artifact))
	if err != nil {
		t.Fatal(err)
	}
	identities, err := configcompose.EffectiveResources(t.Context(), state, map[string]configcompose.ProviderSelection{
		"base/operators": {ID: "sudo"},
	}, store.digest, nil)
	if err != nil || len(identities) != 1 {
		t.Fatalf("derive test endpoint identity: %+v %v", identities, err)
	}
	reports := registry.NewMemory()
	if err := reports.RegisterEndpoint(registry.Endpoint{ID: "endpoint-current", Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	reports.SetEndpointStateReport("endpoint-current", registry.DriftSummary{ReleaseRef: config.ReleaseRef, Digest: store.digest, ReportedAt: time.Unix(1, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{{
		Address: "base/operators", Provider: "sudo", ProviderRevision: "sudo-v1", EffectiveHash: identities[0].EffectiveHash,
		Status: registry.StateDrifted, PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
	}}})
	config.StateReports = reports
	return config
}

func TestFailedChangeLifecycleDoesNotAcquireSuccessAuditAnnotation(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	adminRegistry := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatal(err)
	}
	if err := adminRegistry.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}

	auditLog := &mockAuditLog{}
	srv := New(Config{
		Admin:         adminRegistry,
		AuditLog:      auditLog,
		ChangeControl: changecontrol.NewRegistry(changecontrol.RegistryOptions{}),
		CACert:        caCert,
		CAKey:         caKey,
		CACertPEM:     caPEM,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/change-requests/missing/pause", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("pause missing Change request: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(auditLog.events) != 1 {
		t.Fatalf("audit event count = %d, want 1", len(auditLog.events))
	}
	event := auditLog.events[0]
	if event.Action != audit.ActionAPIRequest || event.ResourceType != "" || event.ResourceID != "" {
		t.Fatalf("failed lifecycle audit = %#v, want generic failed API request", event)
	}
	if event.StatusCode != http.StatusBadRequest {
		t.Fatalf("failed lifecycle audit status = %d, want 400", event.StatusCode)
	}
}

func TestPendingChangeApprovalAuditRetainsExactRequestIdentity(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	adminRegistry := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "44444444-4444-4444-4444-444444444444")
	if err != nil {
		t.Fatal(err)
	}
	if err := adminRegistry.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "change-destructive" }})
	created, err := changes.CreateChangeRequests(changecontrol.FleetPlan{
		Fleet: "production", ReleaseRef: "release-2", ArtifactDigest: "artifact-2",
		Targets:   []changecontrol.TargetEvidence{{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true}},
		Resources: []changecontrol.ResourcePlan{{Address: "base/sudo", DesiredHash: "hash-sudo", Risk: models.RiskDestructive, Provider: "sudo"}},
	}, "operator-seed")
	if err != nil || len(created) != 1 || created[0].RequiredApprovals != 2 {
		t.Fatalf("seed destructive Change request: %#v %v", created, err)
	}

	auditLog := &mockAuditLog{}
	srv := New(Config{Admin: adminRegistry, AuditLog: auditLog, ChangeControl: changes, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/change-requests/change-destructive/authorize", bytes.NewBufferString(`{"attempt_limit":1,"max_concurrency":1,"justification":"CHG-99"}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("first approval: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(auditLog.events) != 1 {
		t.Fatalf("audit event count = %d, want 1", len(auditLog.events))
	}
	event := auditLog.events[0]
	if event.Action != audit.ActionAdminChangeAuthorize || event.ResourceType != "change_request" || event.ResourceID != "change-destructive" {
		t.Fatalf("pending approval audit = %#v, want exact Change request identity", event)
	}
	request, ok := changes.Get("change-destructive")
	if !ok || request.AuthorizationState != changecontrol.AuthorizationApprovalPending || len(request.Approvals) != 1 {
		t.Fatalf("pending approval state = %#v", request)
	}
}

func actionState(action string) string {
	switch action {
	case "pause":
		return "paused"
	case "resume":
		return "authorized"
	default:
		return "revoked"
	}
}
