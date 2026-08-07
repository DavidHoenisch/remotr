//go:build vmsafety

package firewall_test

import (
	"context"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/applicators/firewall"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

const vmFirewallTable = "remotr_vm_safety"

func TestFirewallAuditProvidersVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM firewall audit test must run as root")
	}
	assertFirewallUbuntu2404(t)
	for _, backend := range []string{"nftables", "firewalld"} {
		t.Run(backend, func(t *testing.T) {
			runner := &vmFirewallRecordingRunner{delegate: executil.SanitizedOSRunner{}}
			provider := vmRegisteredFirewallProvider(t, t.TempDir(), runner, "https://127.0.0.1:18443", models.FirewallResource{
				Name: "audit-" + backend, Backend: backend, Action: "allow", Protocol: "tcp", Ports: []int{443},
			})
			provider.AuditPath = t.TempDir() + "/audit.jsonl"
			if check := provider.Check(context.Background()); check.Status != executor.Drifted || check.ReasonCode != "audit_plan" {
				t.Fatalf("initial %s audit Check = %+v", backend, check)
			} else if plan, ok := check.Actual.(firewall.Plan); !ok || plan.Backend != backend || plan.Enforced {
				t.Fatalf("%s audit plan = %#v", backend, check.Actual)
			}
			if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.RollbackClass != executor.RollbackNone {
				t.Fatalf("%s audit ApplyResult = %+v", backend, result)
			}
			if second := provider.Check(context.Background()); second.Status != executor.Drifted || second.ReasonCode != "audit_plan" {
				t.Fatalf("%s second Check = %+v, want persistent structured audit plan", backend, second)
			}
			assertNoFirewallMutation(t, runner.calls)
		})
	}
}

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
	provider := vmFirewallProvider(t, stateDir, runner, func() time.Time { return now })

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

func vmFirewallProvider(t *testing.T, stateDir string, runner executil.Runner, now func() time.Time) *firewall.Applicator {
	t.Helper()
	audit := false
	resource := models.FirewallResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "interrupted-control-path", Audit: &audit, Backend: "nftables",
		Family: "inet", Table: vmFirewallTable, Chain: "input",
		Action: "drop", Protocol: "tcp", Ports: []int{18443},
		RollbackTimeout: "1m",
	}
	provider := vmRegisteredFirewallProvider(t, stateDir, runner, "https://127.0.0.1:18443", resource)
	provider.ResolveIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	provider.ReadFile = os.ReadFile
	provider.Now = now
	provider.AfterFunc = func(time.Duration, func()) {}
	return provider
}

func vmRegisteredFirewallProvider(t *testing.T, stateDir string, runner executil.Runner, syncURL string, resource models.FirewallResource) *firewall.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{Firewall: []models.FirewallResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindFirewall {
		t.Fatalf("firewall registry resources = %+v, %v", resources, err)
	}
	firewallFact := facts.FirewallNftables
	if resource.Backend == "firewalld" {
		firewallFact = facts.FirewallFirewalld
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Firewall: firewallFact}, Runner: runner, StateDir: stateDir, SyncURL: syncURL,
	})
	provider, ok := handler.(*firewall.Applicator)
	if err != nil || !ok || provider.StateDir != stateDir || provider.SyncURL != syncURL {
		t.Fatalf("firewall registry provider = %#v, %v", handler, err)
	}
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

func assertFirewallUbuntu2404(t *testing.T) {
	t.Helper()
	_ = testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04")
}

type vmFirewallRecordingRunner struct {
	delegate executil.Runner
	calls    []executil.MockCall
}

func (r *vmFirewallRecordingRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	return r.delegate.Run(name, args...)
}

func assertNoFirewallMutation(t *testing.T, calls []executil.MockCall) {
	t.Helper()
	for _, call := range calls {
		if call.Name == "nft" && len(call.Args) > 0 && slices.Contains([]string{"add", "delete", "insert", "replace", "flush", "-f"}, call.Args[0]) {
			t.Fatalf("firewall audit mutated nftables: %+v", call)
		}
		if call.Name == "firewall-cmd" {
			for _, arg := range call.Args {
				if strings.HasPrefix(arg, "--add-") || strings.HasPrefix(arg, "--remove-") || arg == "--reload" || arg == "--complete-reload" {
					t.Fatalf("firewall audit mutated firewalld: %+v", call)
				}
			}
		}
	}
}
