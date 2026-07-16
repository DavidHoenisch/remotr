package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
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
	srv := New(Config{Admin: admin, AuditLog: auditLog, ChangeControl: changes, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

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

	adoptionBody, err := json.Marshal(changecontrol.FleetPlan{
		ReleaseRef: "release-existing", ArtifactDigest: "artifact-existing",
		Targets:   []changecontrol.TargetEvidence{{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true}},
		Resources: []changecontrol.ResourcePlan{{Address: "base/sudo", DesiredHash: "hash-sudo", Risk: models.RiskAccess, Provider: "sudo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := request(http.MethodPost, "/v1/admin/fleets/engineering/baseline-adoptions", string(adoptionBody))
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("baseline-adoption")) {
		t.Fatalf("baseline adoption: status=%d body=%s", rec.Code, rec.Body.String())
	}
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
