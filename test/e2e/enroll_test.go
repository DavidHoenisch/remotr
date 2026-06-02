//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/credentials"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

// Enroll e2e exercises remotr-agent enroll (CSR) then mTLS sync — same path as compose agents.

type enrollRequest struct {
	Token string `json:"token"`
}

func TestEnroll_agentSubcommandThenSync(t *testing.T) {
	skipEnrollIfUnavailable(t)

	token := freshEnrollToken(t)

	base := baseURL()
	ca := envOr("REMOTR_E2E_CA", defaultCAPath())
	stateDir := filepath.Join(t.TempDir(), "state")

	runAgentEnroll(t, token, base, ca, stateDir)

	layout, err := credentials.Layout(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := credentials.LoadState(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if st.EndpointID == "" {
		t.Fatal("missing endpoint id in stored state")
	}

	tlsCfg, err := tlsconfig.ClientTLSConfig(layout.Cert, layout.Key, layout.CA)
	if err != nil {
		t.Fatal(err)
	}

	client := sync.NewClient(base, tlsCfg)
	resp, err := client.Sync(sync.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Unchanged {
		t.Fatal("expected initial sync after enroll to return artifact")
	}
	if len(resp.ArtifactYAML) == 0 {
		t.Fatal("expected artifact yaml")
	}
	if !strings.Contains(string(resp.ArtifactYAML), "configurations:") {
		t.Fatalf("artifact missing configurations key: %q", truncate(string(resp.ArtifactYAML), 120))
	}
}

func runAgentEnroll(t *testing.T, token, baseURL, caPath, stateDir string) {
	t.Helper()

	cmd := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr-agent", "enroll",
		"--token", token,
		"--server-url", baseURL,
		"--ca", caPath,
		"--state-dir", stateDir,
		"--no-sync",
	)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("remotr-agent enroll: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "enrolled endpoint") {
		t.Fatalf("unexpected enroll output: %s", out)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func skipEnrollIfUnavailable(t *testing.T) {
	t.Helper()
	base := baseURL()
	ca := envOr("REMOTR_E2E_CA", defaultCAPath())

	client, err := serverTLSClient(ca)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(enrollRequest{Token: "probe"})
	resp, err := client.Post(base+"/v1/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("enroll probe request: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		t.Skip("enroll API not ready")
	default:
		t.Skipf("enroll API not ready (probe status %d)", resp.StatusCode)
	}
}
