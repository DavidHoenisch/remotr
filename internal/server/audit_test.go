package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
)

type mockAuditLog struct {
	events  []audit.Event
	pathKey string
}

func (m *mockAuditLog) RecordAuditEvent(_ context.Context, event audit.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockAuditLog) ListAuditEvents(_ context.Context, filter audit.ListFilter) (audit.Page, error) {
	out := make([]audit.Event, 0, len(m.events))
	for _, event := range m.events {
		if !filter.Since.IsZero() && event.OccurredAt.Before(filter.Since) {
			continue
		}
		out = append(out, event)
	}
	return audit.Page{Events: out}, nil
}

func (m *mockAuditLog) EnsureAuditExportPathKey(context.Context) (string, error) {
	if m.pathKey == "" {
		m.pathKey = "test-export-key"
	}
	return m.pathKey, nil
}

func TestAuditMiddlewareSkipsAnonymousGenericEvent(t *testing.T) {
	auditLog := &mockAuditLog{}
	srv := New(Config{AuditLog: auditLog})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/ca.pem", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(auditLog.events) != 0 {
		t.Fatalf("events = %d", len(auditLog.events))
	}
}

func TestAuditMiddlewareRecordsAnonymousSemanticEvent(t *testing.T) {
	auditLog := &mockAuditLog{}
	srv := New(Config{AuditLog: auditLog})
	handler := srv.auditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		annotateAudit(r, audit.ActionAdminBootstrap, "operator", "op-test", nil)
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/admin/bootstrap", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(auditLog.events) != 1 {
		t.Fatalf("events = %d", len(auditLog.events))
	}
	if auditLog.events[0].Action != audit.ActionAdminBootstrap {
		t.Fatalf("action = %q", auditLog.events[0].Action)
	}
}

func TestExportAuditEventsRequiresOperatorMTLS(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	auditLog := &mockAuditLog{pathKey: "secret-key"}
	srv := New(Config{
		Admin:     admin,
		AuditLog:  auditLog,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/exports/audit/secret-key", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/exports/audit/secret-key", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListAuditEventsAdminRoute(t *testing.T) {
	const canary = "audit-admin-output-secret-canary"
	present := true
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	auditLog := &mockAuditLog{
		events: []audit.Event{{
			ID:         "22222222-2222-2222-2222-222222222222",
			OccurredAt: time.Now().UTC(),
			ActorType:  audit.ActorOperator,
			Action:     audit.ActionAdminGitSync,
			Method:     http.MethodPost,
			Path:       "/v1/admin/git-sync",
			StatusCode: http.StatusOK,
			Details: &executor.SafeSummary{Fields: []executor.SafeField{{
				Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present,
			}}},
		}},
	}
	srv := New(Config{Admin: admin, AuditLog: auditLog, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/audit-events", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"sensitivity":"secret"`)) || bytes.Contains(rec.Body.Bytes(), []byte(canary)) {
		t.Fatalf("classified Admin audit output = %s", rec.Body.String())
	}
}

func TestRejectedSyncRequestsRemainImmediateAudits(t *testing.T) {
	auditLog := &mockAuditLog{}
	srv := New(Config{AuditLog: auditLog, FastPath: FastPathConfig{Enabled: true, ServingProcesses: 1}})

	unauthorized := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(`{}`))
	unauthorizedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthorizedRec, unauthorized)
	if unauthorizedRec.Code != http.StatusUnauthorized || len(auditLog.events) != 1 {
		t.Fatalf("unauthorized status=%d audits=%+v", unauthorizedRec.Code, auditLog.events)
	}

	endpointURI, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	malformed := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(`{`))
	malformed.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{endpointURI}}}}
	malformedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(malformedRec, malformed)
	if malformedRec.Code != http.StatusBadRequest || len(auditLog.events) != 2 {
		t.Fatalf("malformed status=%d audits=%+v", malformedRec.Code, auditLog.events)
	}
	for _, event := range auditLog.events {
		if event.Action != audit.ActionAPIRequest || event.StatusCode < 400 {
			t.Fatalf("rejected Sync audit = %+v", event)
		}
	}
}
