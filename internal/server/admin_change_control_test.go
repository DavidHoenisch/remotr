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
	srv := New(withDerivedSudoPlan(Config{Admin: admin, AuditLog: auditLog, ChangeControl: changes, CACert: caCert, CAKey: caKey, CACertPEM: caPEM}))

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
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "derived-request" }})
	srv := New(Config{
		Admin: adminRegistry, ChangeControl: changes,
		ReleaseRef:    "release-1",
		ArtifactStore: derivedPlanArtifactStore{artifact: artifact, digest: "sha256:artifact"},
		ChangePlanProviders: fixedChangePlanProviders{
			"base/operators": {ID: "sudo"},
		},
		CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
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
}

type derivedPlanArtifactStore struct {
	artifact []byte
	digest   string
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

type fixedChangePlanProviders map[string]configcompose.ProviderSelection

func (p fixedChangePlanProviders) SelectChangePlanProviders(context.Context, string, string, models.State) (map[string]configcompose.ProviderSelection, error) {
	out := make(map[string]configcompose.ProviderSelection, len(p))
	for address, selection := range p {
		out[address] = selection
	}
	return out, nil
}

func withDerivedSudoPlan(config Config) Config {
	config.ReleaseRef = "release-1"
	config.ArtifactStore = derivedPlanArtifactStore{
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
	config.ChangePlanProviders = fixedChangePlanProviders{
		"base/operators": {ID: "sudo"},
	}
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
