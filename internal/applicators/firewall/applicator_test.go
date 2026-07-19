package firewall

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_AuditMode_StateAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	exec := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"firewall-cmd [--version]": {Stdout: []byte("0.9.0\n")},
		},
	}

	trueVal := true
	resource := models.FirewallResource{
		Name:   "test-rule",
		Audit:  &trueVal,
		Action: "allow",
		Ports:  []int{22},
	}

	a := New(resource, exec)
	a.AuditPath = auditPath

	ctx := context.Background()

	// First State call: audit log doesn't exist yet, so not met.
	_, met := a.State(ctx)
	if met {
		t.Fatal("expected State to return false before audit log entry exists")
	}

	// Apply should write the audit log and succeed.
	if err := a.Apply(ctx); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Audit evidence never masquerades as enforced compliance.
	check := a.Check(ctx)
	if check.Status != executor.Drifted || check.ReasonCode != "audit_plan" {
		t.Fatalf("check = %+v", check)
	}
	plan, ok := check.Actual.(Plan)
	if !ok || plan.Enforced || plan.Name != "test-rule" {
		t.Fatalf("plan = %#v", check.Actual)
	}

	// Verify audit log file exists and contains the rule.
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(data), "test-rule") {
		t.Fatalf("audit log missing rule name: %s", string(data))
	}
	if !strings.Contains(string(data), `"enforced":false`) {
		t.Fatalf("audit log should show enforced=false: %s", string(data))
	}
}

func TestApplicator_AuditMode_RevertIsNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	exec := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"firewall-cmd [--version]": {Stdout: []byte("0.9.0\n")},
		},
	}

	trueVal := true
	resource := models.FirewallResource{
		Name:   "test-rule",
		Audit:  &trueVal,
		Action: "allow",
		Ports:  []int{22},
	}

	a := New(resource, exec)
	a.AuditPath = auditPath

	err := a.Revert(context.Background())
	if !errors.Is(err, appErr.ErrNoOp) {
		t.Fatalf("expected ErrNoOp in audit mode, got: %v", err)
	}
}

func TestApplicator_EnforcementMode_FirewalldApply(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	exec := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"firewall-cmd [--version]":                                   {Stdout: []byte("0.9.0\n")},
			"firewall-cmd [--get-default-zone]":                          {Stdout: []byte("public\n")},
			"firewall-cmd [--zone public --list-all]":                    {Stdout: []byte("public\n  target: default\n  services: ssh\n  ports:\n  protocols:\n  forward: no\n  masquerade: no\n  forward-ports:\n  source-ports:\n  icmp-blocks:\n  rich rules:\n")},
			"firewall-cmd [--zone public --add-port 22/tcp --permanent]": {Stdout: []byte("success\n")},
			"firewall-cmd [--zone public --add-rich-rule rule port protocol=\"tcp\" port=\"22\" accept --permanent]": {Stdout: []byte("success\n")},
			"firewall-cmd [--reload]": {Stdout: []byte("success\n")},
		},
	}

	falseVal := false
	resource := models.FirewallResource{
		Name:   "allow-ssh",
		Audit:  &falseVal,
		Action: "allow",
		Ports:  []int{22},
	}

	a := New(resource, exec)
	a.AuditPath = auditPath

	ctx := context.Background()

	// State should return false because port 22/tcp is not in the zone.
	_, met := a.State(ctx)
	if met {
		t.Fatal("expected State false before apply")
	}

	// The backend contract still converges a rule, while the top-level
	// applicator keeps firewalld enforcement audit-only until transactional
	// restore is available.
	b := &firewalldBackend{exec: exec}
	if err := b.apply(ctx, resource); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Verify that firewall-cmd was called for add-port.
	var found bool
	for _, call := range exec.Calls {
		if call.Name == "firewall-cmd" {
			for _, arg := range call.Args {
				if arg == "--add-port" {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected firewall-cmd --add-port call, got: %+v", exec.Calls)
	}

	// Audit log should not be written in enforcement mode.
	_, err := os.ReadFile(auditPath)
	if !os.IsNotExist(err) {
		t.Fatal("expected no audit log in enforcement mode")
	}
}

func TestApplicator_BackendAutoDetect_PrefersFirewalld(t *testing.T) {
	exec := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"firewall-cmd [--version]": {Stdout: []byte("0.9.0\n")},
			"nft [--version]":          {Stdout: []byte("1.0.0\n")},
		},
	}

	resource := models.FirewallResource{
		Name:   "test",
		Action: "allow",
		Ports:  []int{80},
	}

	a := New(resource, exec)
	b, err := a.resolveBackend()
	if err != nil {
		t.Fatalf("resolveBackend error: %v", err)
	}
	if b.name() != "firewalld" {
		t.Fatalf("expected firewalld backend, got: %s", b.name())
	}
}

func TestApplicator_BackendOverride_Nftables(t *testing.T) {
	exec := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"nft [--version]": {Stdout: []byte("1.0.0\n")},
		},
	}

	resource := models.FirewallResource{
		Name:    "test",
		Backend: "nftables",
		Action:  "allow",
		Ports:   []int{80},
	}

	a := New(resource, exec)
	b, err := a.resolveBackend()
	if err != nil {
		t.Fatalf("resolveBackend error: %v", err)
	}
	if b.name() != "nftables" {
		t.Fatalf("expected nftables backend, got: %s", b.name())
	}
}

func TestApplicator_ProtectRemotr_BlocksSyncPort(t *testing.T) {
	exec := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"firewall-cmd [--version]":                                    {Stdout: []byte("0.9.0\n")},
			"firewall-cmd [--get-default-zone]":                           {Stdout: []byte("public\n")},
			"firewall-cmd [--zone public --add-port 443/tcp --permanent]": {Stdout: []byte("success\n")},
			"firewall-cmd [--zone public --add-rich-rule rule port protocol=\"tcp\" port=\"443\" drop --permanent]": {Stdout: []byte("success\n")},
			"firewall-cmd [--reload]": {Stdout: []byte("success\n")},
		},
	}

	falseVal := false
	resource := models.FirewallResource{
		Name:          "drop-https",
		Audit:         &falseVal,
		Action:        "drop",
		Protocol:      "tcp",
		Ports:         []int{443},
		ProtectRemotr: &falseVal,
	}

	a := New(resource, exec)
	a.SyncURL = "https://remotr.example.com:8443"

	// protectRemotr is false, so the legacy local port guard is bypassed. The
	// transaction preflight still captures the complete control path.
	if err := a.validateSyncPath(); err != nil {
		t.Fatalf("validation with protectRemotr=false: %v", err)
	}

	// Now test with protectRemotr=true (default).
	trueVal := true
	resource2 := models.FirewallResource{
		Name:          "drop-https-2",
		Audit:         &falseVal,
		Action:        "drop",
		Protocol:      "tcp",
		Ports:         []int{443},
		ProtectRemotr: &trueVal,
	}
	a2 := New(resource2, exec)
	a2.SyncURL = "https://remotr.example.com"

	// Apply should fail because it would block the sync path.
	err := a2.validateSyncPath()
	if err == nil {
		t.Fatal("expected Apply to fail when rule blocks sync port with protectRemotr=true")
	}
	if !strings.Contains(err.Error(), "would block sync port 443") {
		t.Fatalf("expected sync-path protection error, got: %v", err)
	}
}

func TestApplicator_AuditDefaultTrue(t *testing.T) {
	// When Audit is nil, it defaults to true.
	resource := models.FirewallResource{
		Name:   "default-audit",
		Action: "allow",
		Ports:  []int{443},
	}
	if !resource.IsAudit() {
		t.Fatal("expected IsAudit=true when Audit is nil")
	}
}

func TestApplicator_ProtectRemotrDefaultTrue(t *testing.T) {
	// When ProtectRemotr is nil, it defaults to true.
	resource := models.FirewallResource{
		Name:   "default-protect",
		Action: "allow",
		Ports:  []int{443},
	}
	if !resource.IsProtectRemotr() {
		t.Fatal("expected IsProtectRemotr=true when ProtectRemotr is nil")
	}
}

func TestApplicator_NftablesAbsentDeletesManagedHandle(t *testing.T) {
	audit := false
	r := models.FirewallResource{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "allow-web", Audit: &audit, Backend: "nftables", Action: "allow", Ports: []int{80}}
	exec := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nft [--version]":                               {Stdout: []byte("nftables v1")},
		"nft [-j list ruleset]":                         {Stdout: []byte(`tcp dport 80 accept comment "remotr:allow-web"`)},
		"nft [-a list chain inet filter input]":         {Stdout: []byte(`tcp dport 80 accept comment "remotr:allow-web" # handle 42`)},
		"nft [delete rule inet filter input handle 42]": {},
		"nft [list ruleset]":                            {Stdout: []byte("table inet filter {}\n")},
	}}
	a := New(r, exec)
	enableTestTransaction(t, a)
	if _, met := a.State(context.Background()); met {
		t.Fatal("existing managed rule must drift when absent")
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, call := range exec.Calls {
		if call.Name == "nft" && strings.Join(call.Args, " ") == "delete rule inet filter input handle 42" {
			found = true
		}
	}
	if !found {
		t.Fatalf("managed handle not deleted: %+v", exec.Calls)
	}
}

func TestFirewalldAbsentRemovesExactManagedRule(t *testing.T) {
	exec := &executil.MockRunner{Next: map[string]executil.MockResult{"firewall-cmd [--reload]": {}}}
	b := &firewalldBackend{exec: exec}
	r := models.FirewallResource{Name: "allow-web", Action: "allow", Protocol: "tcp", Ports: []int{80}, Zones: []string{"public"}}
	if err := b.revert(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	want := `--zone public --remove-rich-rule rule port protocol="tcp" port="80" accept --permanent`
	found := false
	for _, call := range exec.Calls {
		if call.Name == "firewall-cmd" && strings.Join(call.Args, " ") == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("exact managed rich rule not removed: %+v", exec.Calls)
	}
}

func TestApplicator_NftablesAuthoritativeSetBoundsCleanup(t *testing.T) {
	audit := false
	r := models.FirewallResource{
		ResourceMeta: models.ResourceMeta{
			Lifecycle: models.LifecyclePresent,
			Ownership: models.OwnershipAuthoritative,
		},
		Name:         "web-ingress",
		Audit:        &audit,
		Backend:      "nftables",
		Family:       "inet",
		Table:        "filter",
		Chain:        "input",
		CleanupLimit: 1,
		Rules: []models.FirewallRule{
			{Name: "https", Action: "allow", Protocol: "tcp", Ports: []int{443}},
		},
	}
	exec := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nft [--version]":       {Stdout: []byte("nftables v1")},
		"nft [-j list ruleset]": {Stdout: []byte(`tcp dport 443 accept comment "remotr:web-ingress/https"`)},
		"nft [-a list chain inet filter input]": {Stdout: []byte(strings.Join([]string{
			`tcp dport 443 accept comment "remotr:web-ingress/https" # handle 10`,
			`tcp dport 80 accept comment "remotr:web-ingress/old-http" # handle 11`,
			`ip saddr 192.0.2.10 drop comment "foreign" # handle 12`,
		}, "\n"))},
		"nft [delete rule inet filter input handle 11]": {},
		"nft [list ruleset]":                            {Stdout: []byte("table inet filter {}\n")},
	}}

	a := New(r, exec)
	enableTestTransaction(t, a)
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}

	var deleted []string
	for _, call := range exec.Calls {
		if call.Name == "nft" && len(call.Args) > 0 && call.Args[0] == "delete" {
			deleted = append(deleted, strings.Join(call.Args, " "))
		}
	}
	if len(deleted) != 1 || deleted[0] != "delete rule inet filter input handle 11" {
		t.Fatalf("cleanup crossed ownership boundary or limit: %v", deleted)
	}
}

func TestApplicator_AuthoritativeCleanupRefusesToExceedBound(t *testing.T) {
	audit := false
	r := models.FirewallResource{
		ResourceMeta: models.ResourceMeta{Ownership: models.OwnershipAuthoritative},
		Name:         "web-ingress", Audit: &audit, Backend: "nftables", CleanupLimit: 1,
		Rules: []models.FirewallRule{{Name: "https", Action: "allow", Protocol: "tcp", Ports: []int{443}}},
	}
	exec := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nft [--version]": {Stdout: []byte("nftables v1")},
		"nft [-a list chain inet filter input]": {Stdout: []byte(strings.Join([]string{
			`tcp dport 443 accept comment "remotr:web-ingress/https" # handle 10`,
			`tcp dport 80 accept comment "remotr:web-ingress/old-http" # handle 11`,
			`tcp dport 8080 accept comment "remotr:web-ingress/old-admin" # handle 12`,
		}, "\n"))},
		"nft [list ruleset]": {Stdout: []byte("table inet filter {}\n")},
		"nft [-f -]":         {},
	}}

	a := New(r, exec)
	enableTestTransaction(t, a)
	err := a.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeding cleanupLimit 1") {
		t.Fatalf("expected bounded cleanup failure, got %v", err)
	}
	for _, call := range exec.Calls {
		if call.Name == "nft" && len(call.Args) > 0 && call.Args[0] == "delete" {
			t.Fatalf("cleanup mutated before validating its bound: %+v", exec.Calls)
		}
	}
}

func TestApplicator_FirewalldAuthoritativeZoneCleansOnlyOwnedZone(t *testing.T) {
	audit := false
	desired := `rule port protocol="tcp" port="443" accept`
	stale := `rule port protocol="tcp" port="80" accept`
	r := models.FirewallResource{
		ResourceMeta: models.ResourceMeta{Ownership: models.OwnershipAuthoritative},
		Name:         "web-zone", Audit: &audit, Backend: "firewalld", Zones: []string{"remotr-web"}, CleanupLimit: 1,
		Rules: []models.FirewallRule{{Name: "https", Action: "allow", Protocol: "tcp", Ports: []int{443}}},
	}
	exec := &executil.MockRunner{Next: map[string]executil.MockResult{
		"firewall-cmd [--version]":                                                      {Stdout: []byte("1.0")},
		"firewall-cmd [--zone remotr-web --list-rich-rules --permanent]":                {Stdout: []byte(desired + "\n" + stale + "\n")},
		"firewall-cmd [--zone remotr-web --remove-rich-rule " + stale + " --permanent]": {},
		"firewall-cmd [--reload]":                                                       {},
	}}

	b := &firewalldBackend{exec: exec}
	if err := b.applyOwned(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	for _, call := range exec.Calls {
		joined := strings.Join(call.Args, " ")
		if strings.Contains(joined, "--zone public") || strings.Contains(joined, "--zone trusted") {
			t.Fatalf("cleanup escaped owned zone: %+v", exec.Calls)
		}
	}
}

func enableTestTransaction(t *testing.T, a *Applicator) {
	t.Helper()
	a.Resource.RollbackTimeout = "2m"
	a.StateDir = t.TempDir()
	a.controlPlan = ControlPathPlan{Host: "mdm.example", Protocol: "tcp", Port: 443, RollbackTimeout: 2 * time.Minute}
	a.Now = func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) }
	a.AfterFunc = func(time.Duration, func()) {}
}

func TestApplicator_PreflightPlansCompleteRemotrControlPath(t *testing.T) {
	audit := false
	r := models.FirewallResource{
		Name: "guard-sync", Audit: &audit, Backend: "nftables", Action: "allow", Ports: []int{8443},
		RollbackTimeout: "2m",
	}
	exec := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nft [--version]":                                   {Stdout: []byte("nftables v1")},
		"ip [-json route get 203.0.113.10]":                 {Stdout: []byte(`[{"dst":"203.0.113.10","gateway":"192.0.2.1","dev":"eth0","prefsrc":"192.0.2.20"}]`)},
		"ss [-Htn state established dst 203.0.113.10:8443]": {Stdout: []byte("ESTAB 0 0 192.0.2.20:40000 203.0.113.10:8443\n")},
	}}
	a := New(r, exec)
	a.SyncURL = "https://mdm.example:8443"
	a.StateDir = t.TempDir()
	a.ResolveIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	}
	a.ReadFile = func(string) ([]byte, error) {
		return []byte("nameserver 192.0.2.53\nsearch corp.example\n"), nil
	}

	if err := a.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan := a.TransactionPlan()
	if plan.Host != "mdm.example" || plan.Protocol != "tcp" || plan.Port != 8443 || plan.RollbackTimeout != 2*time.Minute {
		t.Fatalf("control endpoint omitted from plan: %+v", plan)
	}
	if len(plan.Destinations) != 1 || plan.Destinations[0] != "203.0.113.10" || len(plan.Routes) != 1 || plan.Routes[0].Device != "eth0" {
		t.Fatalf("resolved destination or route omitted: %+v", plan)
	}
	if len(plan.DNSServers) != 1 || plan.DNSServers[0] != "192.0.2.53" || len(plan.SearchDomains) != 1 || plan.SearchDomains[0] != "corp.example" || !plan.EstablishedControlTraffic {
		t.Fatalf("DNS or established control traffic omitted: %+v", plan)
	}
}

func TestApplicator_EnforcedNftablesArmsTimedRollback(t *testing.T) {
	audit := false
	r := models.FirewallResource{
		Name: "allow-sync", Audit: &audit, Backend: "nftables", Action: "allow", Protocol: "tcp", Ports: []int{8443},
		RollbackTimeout: "2m",
	}
	exec := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nft [--version]":                       {Stdout: []byte("nftables v1")},
		"nft [-a list chain inet filter input]": {Stdout: []byte("table inet filter {\n\tchain input {\n\t}\n}\n")},
		"nft [list ruleset]":                    {Stdout: []byte("table inet filter {}\n")},
		"nft [-f -]":                            {},
		"nft [add table inet filter]":           {},
		"nft [add chain inet filter input { type filter hook input priority filter; }]":        {},
		"nft [add rule inet filter input tcp dport 8443 accept comment \"remotr:allow-sync\"]": {},
		"ip [-json route get 203.0.113.10]":                                                    {Stdout: []byte(`[{"dst":"203.0.113.10","gateway":"192.0.2.1","dev":"eth0","prefsrc":"192.0.2.20"}]`)},
		"ss [-Htn state established dst 203.0.113.10:443]":                                     {Stdout: []byte("ESTAB\n")},
	}}
	a := New(r, exec)
	enableTestTransaction(t, a)
	a.Resource.RollbackTimeout = "2m"
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	a.SyncURL = "https://mdm.example"
	a.ResolveIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	}
	a.ReadFile = func(string) ([]byte, error) { return []byte("nameserver 192.0.2.53\n"), nil }
	var watchdogDelay time.Duration
	a.AfterFunc = func(delay time.Duration, _ func()) { watchdogDelay = delay }

	result := a.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("transactional apply result = %+v", result)
	}
	if watchdogDelay != 2*time.Minute {
		t.Fatalf("watchdog delay = %s", watchdogDelay)
	}
	store, err := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: exec, Now: a.now})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || !status.Intent.WatchdogArmed || status.Intent.PlanHash == "" {
		t.Fatalf("armed transaction = %+v, err=%v", status, err)
	}
	now = now.Add(2 * time.Minute)
	status, err = store.Reconcile(context.Background())
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack {
		t.Fatalf("restart timeout rollback = %+v, err=%v", status, err)
	}
	if len(exec.Inputs) != 1 || exec.Inputs[0].Name != "nft" || !strings.Contains(string(exec.Inputs[0].Input), "table inet filter") {
		t.Fatalf("protected nftables rollback input = %+v", exec.Inputs)
	}
	secondCheck := a.Check(context.Background())
	if secondCheck.Status != executor.Drifted || secondCheck.ReasonCode != executor.ReasonStateDrift {
		t.Fatalf("second Check after rollback = %+v", secondCheck)
	}
}

// OS-AEC-073/080: the provider must recognize the exact managed nftables rule
// from nft's stable handle-bearing text output so the required second Check
// can observe convergence after a transactional Apply.
func TestApplicator_EnforcedNftablesSecondCheckRecognizesManagedRule(t *testing.T) {
	audit := false
	resource := models.FirewallResource{
		Name: "vm-control-path", Audit: &audit, Backend: "nftables",
		Family: "inet", Table: "remotr_vm_safety", Chain: "input",
		Action: "allow", Protocol: "tcp", Ports: []int{18443},
		RollbackTimeout: "2m",
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nft [--version]": {},
		"nft [-a list chain inet remotr_vm_safety input]": {
			Stdout: []byte("table inet remotr_vm_safety {\n\tchain input {\n\t\ttcp dport 18443 accept comment \"remotr:vm-control-path\" # handle 2\n\t}\n}\n"),
		},
	}}

	check := New(resource, runner).Check(context.Background())
	if check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("second Check = %+v, want compliant managed rule", check)
	}
}
