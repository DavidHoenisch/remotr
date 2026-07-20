package reboots_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/reboots"
	"github.com/DavidHoenisch/remotr/internal/executil"
)

func TestSystemProbesUseExactInhibitorArgvAndRedactFailures(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"loginctl [list-users --no-legend --no-pager]":                 {Stdout: nil},
		"systemd-inhibit [--list --no-legend --no-pager --mode=block]": {Stdout: []byte("Backup Agent 0 root 123 backupd shutdown Applying kernel maintenance block\n")},
	}}
	probes := reboots.SystemProbes{Runner: runner}
	users, err := probes.ActiveUsers(context.Background())
	if err != nil || users {
		t.Fatalf("ActiveUsers() = %t, %v", users, err)
	}
	workloads, err := probes.ActiveWorkloadInhibitors(context.Background())
	if err != nil || !workloads {
		t.Fatalf("ActiveWorkloadInhibitors() = %t, %v", workloads, err)
	}
	if len(runner.Calls) != 2 || runner.Calls[0].Name != "loginctl" || runner.Calls[1].Name != "systemd-inhibit" {
		t.Fatalf("probe calls = %+v", runner.Calls)
	}

	const canary = "inhibitor-secret-canary"
	failing := reboots.SystemProbes{Runner: &executil.MockRunner{Next: map[string]executil.MockResult{
		"systemd-inhibit [--list --no-legend --no-pager --mode=block]": {Stderr: []byte(canary), Err: errors.New("exit status 1")},
	}}}
	if _, err := failing.ActiveWorkloadInhibitors(context.Background()); err == nil || strings.Contains(err.Error(), canary) {
		t.Fatalf("unsafe inhibitor error = %v", err)
	}
}

// OS-SRM-010 / OS-AEC-098: a block lock for sleep or a hardware key is not a
// workload inhibitor for the coordinated shutdown/reboot boundary.
func TestSystemProbesIgnoreBlockInhibitorsThatDoNotCoverShutdown(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"systemd-inhibit [--list --no-legend --no-pager --mode=block]": {
			Stdout: []byte("ModemManager 0 root 123 ModemManager sleep Managing radio sleep state block\n"),
		},
	}}
	active, err := (reboots.SystemProbes{Runner: runner}).ActiveWorkloadInhibitors(context.Background())
	if err != nil || active {
		t.Fatalf("ActiveWorkloadInhibitors() = %t, %v; want false for sleep-only lock", active, err)
	}
}
