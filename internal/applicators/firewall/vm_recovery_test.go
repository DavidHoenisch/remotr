//go:build vmsafety

package firewall

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const vmFirewallTable = "remotr_vm_safety"

// TestFirewallInterruptedRecoveryVM runs in two processes separated by the
// harness's controlled Ubuntu reboot. The first process leaves an enforced
// control-path rule and protected timeout recovery armed. The reconstructed
// process restores the pre-mutation ruleset after the deadline, then proves a
// second attempt can receive authenticated acknowledgement and converge.
func TestFirewallInterruptedRecoveryVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM firewall recovery test must run as root")
	}
	phase := os.Getenv("REMOTR_FIREWALL_VM_PHASE")
	stateDir := os.Getenv("REMOTR_FIREWALL_VM_STATE_DIR")
	if phase == "" || stateDir == "" {
		t.Fatal("REMOTR_FIREWALL_VM_PHASE and REMOTR_FIREWALL_VM_STATE_DIR are required")
	}

	ctx := context.Background()
	runner := executil.SanitizedOSRunner{}
	preparedAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	now := preparedAt
	if phase == "verify" {
		now = preparedAt.Add(2 * time.Minute)
	}
	provider := vmFirewallProvider(stateDir, runner, func() time.Time { return now })

	switch phase {
	case "prepare":
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatal(err)
		}
		vmDeleteFirewallTable(runner)
		vmCreateFirewallBaseline(t, runner)

		if check := provider.Check(ctx); check.Status != executor.Drifted {
			t.Fatalf("initial Check = %+v, want drifted", check)
		}
		if risks := provider.TransactionPlan().Risks; len(risks) == 0 || !strings.Contains(strings.Join(risks, " "), "block sync port") {
			t.Fatalf("control-path interruption risk was not identified: %v", risks)
		}
		result := provider.ApplyResult(ctx)
		if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
			t.Fatalf("ApplyResult = %+v, want changed transactional", result)
		}
		if check := provider.Check(ctx); check.Status != executor.Compliant {
			t.Fatalf("post-Apply Check = %+v, want compliant", check)
		}
		store := vmNetworkStore(t, stateDir, runner, func() time.Time { return now })
		status, err := store.Status()
		if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || !status.Intent.WatchdogArmed {
			t.Fatalf("prepared transaction = %+v, err=%v", status, err)
		}
	case "verify":
		store := vmNetworkStore(t, stateDir, runner, func() time.Time { return now })
		status, err := store.Reconcile(ctx)
		if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack || status.Intent.RollbackReason != "acknowledgement_timeout" {
			t.Fatalf("restart timeout recovery = %+v, err=%v", status, err)
		}
		if check := provider.Check(ctx); check.Status != executor.Drifted {
			t.Fatalf("second Check after timeout rollback = %+v, want drifted", check)
		}

		result := provider.ApplyResult(ctx)
		if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
			t.Fatalf("second ApplyResult = %+v, want changed transactional", result)
		}
		store = vmNetworkStore(t, stateDir, runner, func() time.Time { return now })
		awaiting, err := store.Status()
		if err != nil || awaiting.Intent == nil || awaiting.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement {
			t.Fatalf("second prepared transaction = %+v, err=%v", awaiting, err)
		}
		acknowledged, err := store.Acknowledge(ctx, awaiting.Intent.ID)
		if err != nil || acknowledged.Intent == nil || acknowledged.Intent.Phase != networkstate.PhaseAcknowledged || !acknowledged.Intent.AuthenticatedAck || acknowledged.Intent.WatchdogArmed {
			t.Fatalf("authenticated acknowledgement = %+v, err=%v", acknowledged, err)
		}
		if check := provider.Check(ctx); check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
			t.Fatalf("second Check after acknowledgement = %+v, want compliant", check)
		}
		vmDeleteFirewallTable(runner)
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown VM firewall recovery phase %q", phase)
	}
}

func vmFirewallProvider(stateDir string, runner executil.Runner, now func() time.Time) *Applicator {
	audit := false
	resource := models.FirewallResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "interrupted-control-path", Audit: &audit, Backend: "nftables",
		Family: "inet", Table: vmFirewallTable, Chain: "input",
		Action: "drop", Protocol: "tcp", Ports: []int{18443},
		RollbackTimeout: "1m",
	}
	provider := New(resource, runner)
	provider.StateDir = stateDir
	provider.SyncURL = "https://127.0.0.1:18443"
	provider.ResolveIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	provider.ReadFile = os.ReadFile
	provider.Now = now
	provider.AfterFunc = func(time.Duration, func()) {}
	return provider
}

func vmNetworkStore(t *testing.T, stateDir string, runner executil.Runner, now func() time.Time) *networkstate.Store {
	t.Helper()
	store, err := networkstate.New(networkstate.Options{Root: stateDir, Runner: runner, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func vmCreateFirewallBaseline(t *testing.T, runner executil.Runner) {
	t.Helper()
	if _, _, err := runner.Run("nft", "add", "table", "inet", vmFirewallTable); err != nil {
		t.Fatalf("create baseline nftables table: %v", err)
	}
	if _, _, err := runner.Run("nft", "add", "chain", "inet", vmFirewallTable, "input", "{", "type", "filter", "hook", "input", "priority", "filter;", "policy", "accept;", "}"); err != nil {
		t.Fatalf("create baseline nftables chain: %v", err)
	}
}

func vmDeleteFirewallTable(runner executil.Runner) {
	_, _, _ = runner.Run("nft", "delete", "table", "inet", vmFirewallTable)
}
