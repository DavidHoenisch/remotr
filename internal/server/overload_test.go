package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSyncOverloadSignalsAuthenticatedRetryAfter(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"})
	srv := New(Config{Registry: reg, SyncAdmission: denySyncAdmission{retryAfter: 7 * time.Second}})

	uri, err := url.Parse("urn:remotr:endpoint:" + endpointID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(`{}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Retry-After"); got != "7" {
		t.Fatalf("Retry-After = %q, want 7", got)
	}
}

func TestSyncOverloadDoesNotMaskUnauthenticatedRequest(t *testing.T) {
	srv := New(Config{SyncAdmission: denySyncAdmission{retryAfter: time.Second}})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestSyncLimiterCapsRetryAfterAndReleasesSlot(t *testing.T) {
	limiter := newSyncLimiter(1, time.Hour)
	release, _, admitted := limiter.Acquire()
	if !admitted || release == nil {
		t.Fatal("first admission was rejected")
	}
	_, retryAfter, admitted := limiter.Acquire()
	if admitted || retryAfter != maxSyncRetryAfter {
		t.Fatalf("second admission = (%t, %s), want (false, %s)", admitted, retryAfter, maxSyncRetryAfter)
	}
	release()
	if release, _, admitted := limiter.Acquire(); !admitted || release == nil {
		t.Fatal("slot was not released")
	}
}

type denySyncAdmission struct{ retryAfter time.Duration }

func (d denySyncAdmission) Acquire() (release func(), retryAfter time.Duration, admitted bool) {
	return nil, d.retryAfter, false
}
