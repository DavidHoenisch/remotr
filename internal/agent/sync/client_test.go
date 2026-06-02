package sync

import (
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
