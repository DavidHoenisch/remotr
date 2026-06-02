//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
)

func TestAdmin_bootstrapEnrollListAndLabels(t *testing.T) {
	skipAdminIfUnavailable(t)

	base := baseURL()
	ca := envOr("REMOTR_E2E_CA", defaultCAPath())
	stateDir := filepath.Join(t.TempDir(), "operator")

	token := waitBootstrapToken(t, 60*time.Second)
	if token == "" {
		t.Skip("bootstrap token not available (stack may already be bootstrapped); run: make compose-down && make test-e2e")
	}

	runRemotrBootstrap(t, token, base, ca, stateDir)
	if !opcreds.Present(stateDir) {
		t.Fatal("expected operator credentials after bootstrap")
	}

	t.Run("createEnrollTokenAndListEndpoints", func(t *testing.T) {
		runRemotrEnrollTokenCreate(t, base, stateDir, "test-fleet")

		eps := runRemotrEndpointListJSON(t, base, stateDir)
		if len(eps) == 0 {
			t.Fatal("expected seeded endpoints in list")
		}
		found := false
		for _, ep := range eps {
			if ep.Fleet == "test-fleet" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected at least one endpoint in test-fleet, got %+v", eps)
		}
	})

	t.Run("endpointLabelsFromSync", func(t *testing.T) {
		waitForAgentEnrolled(t, "debian")
		_, endpointID, err := enrolledAgentTLS("debian")
		if err != nil {
			t.Fatal(err)
		}
		syncWithLabels(t, "debian", map[string]string{"site": "e2e-berlin", "role": "web"})

		eps := runRemotrEndpointListJSON(t, base, stateDir)
		var got map[string]string
		for _, ep := range eps {
			if ep.ID == endpointID {
				got = ep.Labels
				break
			}
		}
		if got == nil {
			t.Fatalf("endpoint %s not found in list", endpointID)
		}
		if got["site"] != "e2e-berlin" || got["role"] != "web" {
			t.Fatalf("labels = %+v", got)
		}

		show := runRemotrEndpointShowJSON(t, base, stateDir, endpointID)
		if show.Labels["site"] != "e2e-berlin" {
			t.Fatalf("show labels = %+v", show.Labels)
		}
	})
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
		t.Skipf("stack not ready (healthz status %d)", resp.StatusCode)
	}
}

func runRemotrBootstrap(t *testing.T, token, baseURL, caPath, stateDir string) {
	t.Helper()
	cmd := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr", "bootstrap",
		"--server-url", baseURL,
		"--ca", caPath,
		"--token", token,
		"--state-dir", stateDir,
	)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("remotr bootstrap: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "operator bootstrapped") {
		t.Fatalf("unexpected bootstrap output: %s", out)
	}
}

func runRemotrEnrollTokenCreate(t *testing.T, baseURL, stateDir, fleet string) {
	t.Helper()
	cmd := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr", "enroll", "token", "create",
		"--server-url", baseURL,
		"--fleet", fleet,
		"--state-dir", stateDir,
	)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("remotr enroll token create: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "enrollment token") {
		t.Fatalf("unexpected enroll token output: %s", out)
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

func runRemotrEndpointListJSON(t *testing.T, baseURL, stateDir string) []endpointJSON {
	t.Helper()
	cmd := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr", "endpoint", "list",
		"--server-url", baseURL,
		"--state-dir", stateDir,
		"--json",
	)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("remotr endpoint list: %v\n%s", err, out)
	}
	var eps []endpointJSON
	if err := json.Unmarshal(out, &eps); err != nil {
		t.Fatalf("decode endpoint list json: %v\n%s", err, out)
	}
	return eps
}

func runRemotrEndpointShowJSON(t *testing.T, baseURL, stateDir, endpointID string) endpointJSON {
	t.Helper()
	cmd := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr", "endpoint", "show",
		"--server-url", baseURL,
		"--state-dir", stateDir,
		"--json",
		endpointID,
	)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("remotr endpoint show: %v\n%s", err, out)
	}
	var ep endpointJSON
	if err := json.Unmarshal(out, &ep); err != nil {
		t.Fatalf("decode endpoint show json: %v\n%s", err, out)
	}
	return ep
}

func syncWithLabels(t *testing.T, agentName string, labels map[string]string) {
	t.Helper()
	tlsCfg, _, err := enrolledAgentTLS(agentName)
	if err != nil {
		t.Fatal(err)
	}

	base := baseURL()
	body, _ := json.Marshal(map[string]any{"labels": labels})
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
	_, _ = io.Copy(io.Discard, resp.Body)
}
