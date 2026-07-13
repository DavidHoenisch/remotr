//go:build providerintegration

package systemdtimer_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/systemdtimer"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-ESM-007: run the staged pair through the host's real systemd-analyze
// verifier while keeping only systemctl state at a controlled boundary.
func TestApplicatorUsesRealSystemdAnalyzeVerification(t *testing.T) {
	if _, err := exec.LookPath("systemd-analyze"); err != nil {
		t.Fatal("systemd-analyze is required for provider integration evidence")
	}
	persistent := true
	root := t.TempDir()
	runner := &realAnalyzeRunner{systemdTimerRunner: systemdTimerRunner{}}
	provider := systemdtimer.New(models.EndpointScheduleResource{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "verified", Backend: models.ScheduleBackendSystemdTimer, Schedule: "daily", User: "root", Argv: []string{"/usr/bin/true"}, Persistent: &persistent}, runner)
	provider.UnitDir, provider.EnvironmentDir = filepath.Join(root, "units"), filepath.Join(root, "environment")
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ResolveSecret = func(context.Context, string) (string, error) { return "", nil }
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() with real systemd-analyze = %+v", result)
	}
	if !runner.analyzedPair {
		t.Fatal("provider did not verify the staged service/timer pair")
	}
}

type realAnalyzeRunner struct {
	systemdTimerRunner
	analyzedPair bool
}

func (r *realAnalyzeRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != "systemd-analyze" {
		return r.systemdTimerRunner.Run(name, args...)
	}
	if len(args) != 3 || args[0] != "verify" || filepath.Ext(args[1]) != ".service" || filepath.Ext(args[2]) != ".timer" {
		return nil, nil, fmt.Errorf("unexpected systemd-analyze argv %v", args)
	}
	r.analyzedPair = true
	return (executil.SanitizedOSRunner{}).Run(name, args...)
}
