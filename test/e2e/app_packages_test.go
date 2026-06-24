//go:build e2e

package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdmin_appPackagesSeeded(t *testing.T) {
	skipAdminIfUnavailable(t)

	base := baseURL()
	ca := envOr("REMOTR_E2E_CA", defaultCAPath())
	stateDir := filepath.Join(t.TempDir(), "operator")

	token := waitBootstrapToken(t, 60*time.Second)
	if token == "" {
		t.Skip("bootstrap token not available")
	}
	runRemotrBootstrap(t, token, base, ca, stateDir)

	cmd := exec.Command("go", "run", "-mod=vendor", "./cmd/remotr", "app", "list", "--json",
		"--server-url", base,
		"--ca", ca,
		"--state-dir", stateDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("app list: %v: %s", err, out)
	}
	body := string(out)
	if !strings.Contains(body, "e2e/test-cli") || !strings.Contains(body, "1.0.0") {
		t.Fatalf("expected seeded app package in list, got %s", body)
	}
}
