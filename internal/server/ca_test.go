package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCAPEM_servesPublicCA(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	srv := New(Config{
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ca.pem", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/x-pem-file" {
		t.Fatalf("content-type = %q", got)
	}
	if string(rec.Body.Bytes()) != string(caPEM) {
		t.Fatal("body does not match CA PEM")
	}
}

func TestCAPEM_unavailableWithoutCA(t *testing.T) {
	srv := New(Config{})
	req := httptest.NewRequest(http.MethodGet, "/v1/ca.pem", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
