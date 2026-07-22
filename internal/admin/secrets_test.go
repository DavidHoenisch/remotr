package admin

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/secrets"
)

func TestUploadSecretVersionScopedSendsCanonicalScopeWithoutMaterialInURL(t *testing.T) {
	canary := []byte("admin-client-global-canary")
	client := &Client{BaseURL: "https://remotr.test", HTTPClient: &http.Client{Transport: secretRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("name") != "ubuntu-pro/shared" || r.URL.Query().Get("scope") != "global" || r.URL.Query().Get("fleet") != "" || r.URL.Query().Get("endpoint_id") != "" {
			t.Errorf("query = %v", r.URL.Query())
		}
		if r.URL.RawQuery == string(canary) || r.URL.Query().Has(string(canary)) {
			t.Fatal("secret material entered request URL")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != string(canary) {
			t.Errorf("body=%q err=%v", body, err)
		}
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"name":"ubuntu-pro/shared","version":"1","scope":"global","fingerprint":"sha256:safe","active":false,"createdAt":"2026-07-22T00:00:00Z","createdBy":"operator-1","revoked":false,"resolutionBlocked":false}`)), Header: make(http.Header)}, nil
	})}}
	metadata, err := client.UploadSecretVersionScoped("ubuntu-pro/shared", secrets.ScopeGlobal, "", "", canary)
	if err != nil || metadata.Scope != secrets.ScopeGlobal {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
}

type secretRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn secretRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
