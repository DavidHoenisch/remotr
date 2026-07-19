package sync

import (
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_Sync_readsGzipErrorBody(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusInternalServerError)
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte("artifact unavailable"))
		_ = gz.Close()
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // test server
	})

	_, err := client.Sync(Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "artifact unavailable") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_Sync_decodesGzipResponse(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") == "" {
			t.Fatal("expected Accept-Encoding header")
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_ = json.NewEncoder(gz).Encode(Response{
			Unchanged:         false,
			ReleaseRef:        "abc123",
			Digest:            "deadbeef",
			ArtifactYAML:      []byte("configurations: []\n"),
			RemediationPolicy: "auto",
		})
		_ = gz.Close()
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // test server
	})

	resp, err := client.Sync(Request{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReleaseRef != "abc123" {
		t.Fatalf("releaseRef = %q", resp.ReleaseRef)
	}
	if !strings.Contains(string(resp.ArtifactYAML), "configurations") {
		t.Fatalf("artifact = %q", resp.ArtifactYAML)
	}
	if resp.RemediationPolicy != "auto" {
		t.Fatalf("policy = %q", resp.RemediationPolicy)
	}
}

func TestClient_Sync_classifiesPermanentStatus(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "credential rejected", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) //nolint:gosec // test server
	_, err := client.Sync(Request{})
	if !IsPermanent(err) {
		t.Fatalf("error %v is not permanent", err)
	}
}

func TestClient_Sync_classifiesOverloadRetryAfter(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) //nolint:gosec // test server
	_, err := client.Sync(Request{})
	if !IsOverloaded(err) {
		t.Fatalf("error %v is not overload", err)
	}
	if retryAfter, ok := RetryAfter(err); !ok || retryAfter != 7*time.Second {
		t.Fatalf("RetryAfter() = (%s, %t), want (7s, true)", retryAfter, ok)
	}
}

func TestClientSyncCapabilityBlockedIsSuccessfulAuthenticatedOutcome(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{ReleaseRef: "release-target", CapabilityBlocked: &CapabilityBlocked{
			TargetReleaseRef:    "release-target",
			MissingRequirements: []MissingRequirement{{ID: "provider:package/apt", Revision: "1"}},
		}})
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) //nolint:gosec // test server
	response, err := client.Sync(Request{LastReleaseRef: "release-active", LastDigest: "digest-active"})
	if err != nil {
		t.Fatalf("capability-blocked Sync returned an error: %v", err)
	}
	if response.CapabilityBlocked == nil || response.CapabilityBlocked.TargetReleaseRef != "release-target" {
		t.Fatalf("capability-blocked response = %+v", response)
	}
	if IsPermanent(err) || IsOverloaded(err) {
		t.Fatalf("successful capability block was classified as a failure: %v", err)
	}
}
