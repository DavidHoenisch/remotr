package firewall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
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

	// Second State call: audit log now has the entry, so met.
	_, met = a.State(ctx)
	if !met {
		t.Fatal("expected State to return true after audit log entry exists")
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

	// Apply should invoke firewall-cmd.
	if err := a.Apply(ctx); err != nil {
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

	ctx := context.Background()
	_, met := a.State(ctx)
	if met {
		t.Fatal("expected State false before apply")
	}

	// protectRemotr is false, so apply should succeed (no validation).
	if err := a.Apply(ctx); err != nil {
		t.Fatalf("Apply error with protectRemotr=false: %v", err)
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
	err := a2.Apply(ctx)
	if err == nil {
		t.Fatal("expected Apply to fail when rule blocks sync port with protectRemotr=true")
	}
	if !strings.Contains(err.Error(), "sync-path protection blocked apply") {
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
	}}
	a := New(r, exec)
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
