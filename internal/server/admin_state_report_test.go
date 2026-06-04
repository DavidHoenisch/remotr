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
	reg.SetEndpointDriftReport(endpointID, registry.DriftSummary{
		ReleaseRef: "abc123",
		Digest:     "sha256:deadbeef",
		ReportedAt: reportedAt,
	}, []byte(`{"inCompliance":true,"items":[]}`))

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
	reg.SetEndpointDriftReport("11111111-1111-1111-1111-111111111111", registry.DriftSummary{
		ReleaseRef: "abc123",
		Digest:     "sha256:one",
		ReportedAt: reportedAt,
	}, []byte(`{"inCompliance":true,"items":[]}`))
	reg.SetEndpointDriftReport("22222222-2222-2222-2222-222222222222", registry.DriftSummary{
		ReleaseRef: "abc123",
		Digest:     "sha256:two",
		ReportedAt: reportedAt,
	}, []byte(`{"inCompliance":false,"items":[{"address":"cfg/pkg","name":"pkg","description":"missing"}]}`))

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
