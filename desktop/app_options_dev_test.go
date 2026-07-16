//go:build dev

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDevelopmentApplicationAssetPolicyAllowsViteStylesOnly(t *testing.T) {
	app := newApplicationOptions()
	if app.AssetServer == nil || app.AssetServer.Middleware == nil {
		t.Fatal("development application asset middleware is not configured")
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := app.AssetServer.Middleware(next)
	request := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/"}}
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("development asset status = %d, want %d", response.Code, http.StatusNoContent)
	}
	policy := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "style-src 'self' 'unsafe-inline'") {
		t.Errorf("development Content-Security-Policy = %q, want Vite inline styles allowed", policy)
	}
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("development Content-Security-Policy = %q, must not allow inline scripts", policy)
	}
}
