//go:build e2e

package e2e

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/sync"
)

func TestStack_healthz(t *testing.T) {
	base := baseURL()
	ca := envOr("REMOTR_E2E_CA", defaultCAPath())

	pool, err := loadCAPool(ca)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		},
	}

	resp, err := client.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("healthz status=%d body=%s", resp.StatusCode, b)
	}
}

func TestStack_debianAgentSyncReceivesArtifact(t *testing.T) {
	syncWithEnrolledAgent(t, "debian")
}

func TestStack_archAgentSyncReceivesArtifact(t *testing.T) {
	syncWithEnrolledAgent(t, "arch")
}

func TestStack_syncResponseUsesGzip(t *testing.T) {
	waitForAgentEnrolled(t, "debian")
	tlsCfg, _, err := enrolledAgentTLS("debian")
	if err != nil {
		t.Fatal(err)
	}

	base := baseURL()
	body, _ := json.Marshal(map[string]string{})
	req, err := http.NewRequest(http.MethodPost, base+"/v1/sync", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("sync status=%d body=%s", resp.StatusCode, b)
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", resp.Header.Get("Content-Encoding"))
	}
}

func syncWithEnrolledAgent(t *testing.T, name string) {
	t.Helper()
	waitForAgentEnrolled(t, name)

	tlsCfg, _, err := enrolledAgentTLS(name)
	if err != nil {
		t.Fatal(err)
	}

	client := sync.NewClient(baseURL(), tlsCfg)
	resp, err := client.Sync(sync.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Unchanged {
		t.Fatal("expected enrolled agent sync to return artifact")
	}
	if len(resp.ArtifactYAML) == 0 {
		t.Fatal("expected artifact yaml")
	}
	if resp.ReleaseRef == "" {
		t.Fatal("expected release ref")
	}

	if !strings.Contains(string(resp.ArtifactYAML), "configurations:") {
		t.Fatalf("artifact missing configurations key: %q", truncate(string(resp.ArtifactYAML), 120))
	}
}
