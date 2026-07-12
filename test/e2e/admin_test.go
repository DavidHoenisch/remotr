//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestAdmin_bootstrapEnrollListAndLabels(t *testing.T) {
	runAdminWorkflowTracers(t)
}

func skipAdminIfUnavailable(t *testing.T) {
	t.Helper()
	base := baseURL()
	ca := envOr("REMOTR_E2E_CA", defaultCAPath())

	client, err := serverTLSClient(ca)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("admin probe: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		// test-exception: EXC-002
		t.Skipf("stack not ready (healthz status %d)", resp.StatusCode)
	}
}

type endpointJSON struct {
	ID              string            `json:"id"`
	Fleet           string            `json:"fleet"`
	CertFingerprint string            `json:"cert_fingerprint"`
	Labels          map[string]string `json:"labels"`
	LastDrift       *struct {
		ReleaseRef string    `json:"release_ref"`
		Digest     string    `json:"digest"`
		ReportedAt time.Time `json:"reported_at"`
	} `json:"last_drift"`
}
