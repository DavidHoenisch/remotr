package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestGetEndpointStateReport(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	caCert, caKey, caPEM := testCAForEnroll(t)
	reg := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	_ = reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"})
	reportedAt := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if err := reg.SetEndpointDriftReport(endpointID, registry.DriftSummary{
		ReleaseRef: "abc123",
		Digest:     "sha256:deadbeef",
		ReportedAt: reportedAt,
	}, []byte(`{"schemaVersion":5,"inCompliance":true,"items":[],"rebootRequired":{"required":true,"sources":[{"address":"base/packages/kernel","name":"kernel","provider":"apt"}],"attemptGeneration":3,"intent":{"generation":"kernel-6.12.1","phase":"timed-out","priorBootId":"boot-1","currentBootId":"boot-1","attemptGeneration":3,"reason":"reboot_timeout_same_boot_id"}}}`)); err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		Admin:        reg,
		StateReports: reg,
		CACert:       caCert,
		CAKey:        caKey,
		CACertPEM:    caPEM,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/endpoints/"+endpointID+"/state-report", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var report registry.StateReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.EndpointID != endpointID || report.Fleet != "engineering" {
		t.Fatalf("report = %+v", report)
	}
	if !report.InCompliance || !report.HasReport() {
		t.Fatalf("expected compliant report, got %+v", report)
	}
	if report.Digest != "sha256:deadbeef" {
		t.Fatalf("digest = %q", report.Digest)
	}
	if report.RebootRequired == nil || !report.RebootRequired.Required || len(report.RebootRequired.Sources) != 1 || report.RebootRequired.Sources[0].Provider != "apt" || report.RebootRequired.Intent == nil || report.RebootRequired.Intent.Phase != "timed-out" || report.RebootRequired.Intent.Reason != "reboot_timeout_same_boot_id" || report.RebootRequired.AttemptGeneration != 3 {
		t.Fatalf("reboot requirement = %+v", report.RebootRequired)
	}
}

func TestGetFleetStateReport(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	reg := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	_ = reg.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	_ = reg.RegisterEndpoint(registry.Endpoint{ID: "11111111-1111-1111-1111-111111111111", Fleet: "engineering"})
	_ = reg.RegisterEndpoint(registry.Endpoint{ID: "22222222-2222-2222-2222-222222222222", Fleet: "engineering"})
	reportedAt := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if err := reg.SetEndpointDriftReport("11111111-1111-1111-1111-111111111111", registry.DriftSummary{
		ReleaseRef: "abc123",
		Digest:     "sha256:one",
		ReportedAt: reportedAt,
	}, []byte(`{"inCompliance":true,"items":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetEndpointDriftReport("22222222-2222-2222-2222-222222222222", registry.DriftSummary{
		ReleaseRef: "abc123",
		Digest:     "sha256:two",
		ReportedAt: reportedAt,
	}, []byte(`{"inCompliance":false,"items":[{"address":"cfg/pkg","name":"pkg","description":"missing"}]}`)); err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		Admin:        reg,
		StateReports: reg,
		CACert:       caCert,
		CAKey:        caKey,
		CACertPEM:    caPEM,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/fleets/engineering/state-report", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var report registry.FleetStateReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 2 || report.Summary.Compliant != 1 || report.Summary.Drift != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
}

func TestGetFleetStateReport_separatesStructuredOutcomeBuckets(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	reg := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "99999999-9999-9999-9999-999999999999")
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}

	reports := []struct {
		id      string
		payload string
	}{
		{"11111111-1111-1111-1111-111111111111", `{"schemaVersion":2,"inCompliance":true,"items":[{"address":"cfg/ok","status":"compliant","reasonCode":"compliant"}]}`},
		{"22222222-2222-2222-2222-222222222222", `{"schemaVersion":2,"inCompliance":false,"items":[{"address":"cfg/drift","status":"drifted","reasonCode":"state_drift"}]}`},
		{"33333333-3333-3333-3333-333333333333", `{"schemaVersion":2,"inCompliance":false,"items":[{"address":"cfg/unsupported","status":"unsupported","reasonCode":"provider_unavailable"}]}`},
		{"44444444-4444-4444-4444-444444444444", `{"schemaVersion":2,"inCompliance":false,"items":[{"address":"cfg/failed","status":"check_failed","reasonCode":"probe_failed"}]}`},
		{"55555555-5555-5555-5555-555555555555", `{"schemaVersion":2,"inCompliance":false,"items":[{"address":"cfg/deferred","status":"deferred","reasonCode":"maintenance_window"}]}`},
		{"66666666-6666-6666-6666-666666666666", `{"schemaVersion":2,"inCompliance":true,"items":[{"address":"cfg/applied","status":"compliant","reasonCode":"compliant"}],"apply":[{"address":"cfg/applied","status":"failed","reasonCode":"apply_failed"}]}`},
	}
	reportedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	for _, item := range reports {
		if err := reg.RegisterEndpoint(registry.Endpoint{ID: item.id, Fleet: "engineering"}); err != nil {
			t.Fatal(err)
		}
		if err := reg.SetEndpointDriftReport(item.id, registry.DriftSummary{ReleaseRef: "abc123", Digest: "sha256:" + item.id[:8], ReportedAt: reportedAt}, []byte(item.payload)); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: "77777777-7777-7777-7777-777777777777", Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}

	srv := New(Config{Admin: reg, StateReports: reg, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/fleets/engineering/state-report", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var report registry.FleetStateReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 7 || report.Summary.Compliant != 1 || report.Summary.Drift != 1 ||
		report.Summary.Unsupported != 1 || report.Summary.CheckFailed != 1 || report.Summary.Deferred != 1 ||
		report.Summary.ApplyFailed != 1 || report.Summary.NoReport != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	for _, endpoint := range report.Endpoints {
		if endpoint.EndpointID == "66666666-6666-6666-6666-666666666666" && endpoint.Status != registry.StateApplyFailed {
			t.Fatalf("apply failure endpoint status = %q", endpoint.Status)
		}
	}
}
