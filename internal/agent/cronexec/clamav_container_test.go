//go:build providercontainer

package cronexec_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/cronexec"
	"github.com/DavidHoenisch/remotr/internal/croncatalog"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Only systemd is modeled: the container has no service manager. The scan uses
// real clamdscan/clamd and the fixture database prepared by the container script.
type activeFreshclamService struct{ executil.OSRunner }

func (r activeFreshclamService) Run(name string, args ...string) ([]byte, []byte, error) {
	if name == "systemctl" {
		if fmt.Sprint(args) != "[is-active --quiet clamav-freshclam.service]" {
			return nil, nil, fmt.Errorf("unexpected service query: %v", args)
		}
		return nil, nil, nil
	}
	return r.OSRunner.Run(name, args...)
}

func TestScheduledClamAVScansPrivateHomes(t *testing.T) {
	if os.Getenv("REMOTR_CLAMAV_FIXTURE") != "1" {
		t.Fatal("requires disposable ClamAV container")
	}
	state, err := croncatalog.Resolve("", models.CronState{Crons: []models.CronJob{{Use: "builtin/clamav-scan-debian"}}})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := croncatalog.JobSpecYAML(state.Crons[0])
	if err != nil {
		t.Fatal(err)
	}
	const path = "/home/remotr-cron-private/sample.txt"
	if err := os.WriteFile(path, []byte("clean file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "next"} {
		result := cronexec.Run(t.Context(), spec, "clamav-scan-debian", id, activeFreshclamService{})
		if result.Status != "success" {
			t.Fatalf("clean private home scan %s = %+v, want success", id, result)
		}
	}
	if err := os.WriteFile(path, []byte("remotr harmless scanner fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := cronexec.Run(t.Context(), spec, "clamav-scan-debian", "detection", activeFreshclamService{})
	if result.Status != "failed" || len(result.Failures) != 1 || result.Failures[0].ResourceAddress != "clamav-scan-debian/scan-homes" {
		t.Fatalf("signature in private home was not reported: %+v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scanner must not remove files: %v", err)
	}
}
