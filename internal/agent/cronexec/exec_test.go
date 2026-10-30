//go:build providercontainer

package cronexec_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/cronexec"
	"github.com/DavidHoenisch/remotr/internal/croncatalog"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestSkippedCronResourceDoesNotReportSuccess(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"/usr/bin/false []": {Err: errors.New("not satisfied")}}}
	result := cronexec.Run(t.Context(), []byte(`crons:
  - name: blocked
    schedule: "0 0 * * 0"
    commands:
      - name: unsafe-command
        check: [/usr/bin/false]
        apply: [/usr/bin/touch, /must-not-run]
`), "blocked", "run-1", runner)
	if result.Status != "failed" || !strings.Contains(result.Message, "skipped") {
		t.Fatalf("skipped cron result = %+v, want failed with skipped reason", result)
	}
	if len(result.Failures) != 1 || result.Failures[0].ResourceAddress != "blocked/unsafe-command" {
		t.Fatalf("failures = %+v", result.Failures)
	}
	for _, call := range runner.Calls {
		if call.Name == "/usr/bin/touch" {
			t.Fatal("safety-blocked command executed")
		}
	}
}

// This test runs only in the disposable package-provider container, with the
// local fixture repository configured and fixture version 1 already installed.
func TestScheduledUpgradeExecutesRealAPT(t *testing.T) {
	if os.Getenv("REMOTR_PROVIDER_DISTRO") != "debian" {
		t.Fatal("requires disposable Debian provider container")
	}
	version := func() string {
		t.Helper()
		data, err := exec.Command("dpkg-query", "-W", "-f=${Version}", "remotr-fixture").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := version(); got != "1.0.0-1" {
		t.Fatalf("initial fixture version = %q, want 1.0.0-1", got)
	}
	state, err := croncatalog.Resolve("", models.CronState{Crons: []models.CronJob{{Use: "builtin/system-upgrade-debian"}}})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := croncatalog.JobSpecYAML(state.Crons[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "next"} {
		result := cronexec.Run(t.Context(), spec, "system-upgrade-debian", id, nil)
		if result.Status != "success" {
			t.Fatalf("upgrade result = %+v", result)
		}
		if got := version(); got != "2.0.0-1" {
			t.Fatalf("installed version after %s occurrence = %q, want 2.0.0-1", id, got)
		}
	}
}
