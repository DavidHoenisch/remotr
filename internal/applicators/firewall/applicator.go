package firewall

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const defaultAuditLogPath = "/var/log/remotr/firewall-audit.log"

// backend defines the interface for firewall backends (firewalld, nftables).
type backend interface {
	name() string
	available() bool
	state(ctx context.Context, rule models.FirewallResource) (bool, error)
	apply(ctx context.Context, rule models.FirewallResource) error
	revert(ctx context.Context, rule models.FirewallResource) error
}

// Applicator implements executor.Handler for firewall rules.
type Applicator struct {
	Resource  models.FirewallResource
	Exec      executil.Runner
	AuditPath string
	SyncURL   string
}

// New creates a firewall applicator.
func New(r models.FirewallResource, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{
		Resource:  r,
		Exec:      exec,
		AuditPath: defaultAuditLogPath,
	}
}

func (a *Applicator) Name() string { return "firewall:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return fmt.Sprintf("firewall rule %s (%s)", a.Resource.Name, a.Resource.Action)
}

// State checks if the target state is met.
// For audit mode, it checks whether the audit log already has an up-to-date entry.
// For enforcement mode, it checks the actual firewall state.
func (a *Applicator) State(ctx context.Context) (any, bool) {
	if a.Resource.IsAudit() {
		met, err := a.auditLogUpToDate()
		if err != nil {
			return nil, false
		}
		return nil, met
	}

	b, err := a.resolveBackend()
	if err != nil {
		return nil, false
	}
	met, err := b.state(ctx, a.Resource)
	if err != nil {
		return nil, false
	}
	return nil, met
}

// Apply writes the rule. In audit mode, it appends to the audit log.
// In enforcement mode, it delegates to the backend after optional sync-path validation.
func (a *Applicator) Apply(ctx context.Context) error {
	_, met := a.State(ctx)
	if met {
		return appErr.ErrStateAlreadyMet
	}

	if a.Resource.IsAudit() {
		return a.writeAuditLog()
	}

	if a.Resource.IsProtectRemotr() {
		if err := a.validateSyncPath(); err != nil {
			return fmt.Errorf("firewall %q: sync-path protection blocked apply: %w", a.Resource.Name, err)
		}
	}

	b, err := a.resolveBackend()
	if err != nil {
		return err
	}
	return b.apply(ctx, a.Resource)
}

// Revert removes the rule. In audit mode, it is a no-op.
func (a *Applicator) Revert(ctx context.Context) error {
	if a.Resource.IsAudit() {
		return appErr.ErrNoOp
	}
	b, err := a.resolveBackend()
	if err != nil {
		return err
	}
	return b.revert(ctx, a.Resource)
}

// resolveBackend selects the appropriate backend based on availability and resource preference.
func (a *Applicator) resolveBackend() (backend, error) {
	preferred := strings.ToLower(strings.TrimSpace(a.Resource.Backend))

	firewalldB := &firewalldBackend{exec: a.Exec}
	nftB := &nftablesBackend{exec: a.Exec}

	switch preferred {
	case "firewalld":
		if !firewalldB.available() {
			return nil, fmt.Errorf("firewall %q: backend firewalld not available", a.Resource.Name)
		}
		return firewalldB, nil
	case "nftables":
		if !nftB.available() {
			return nil, fmt.Errorf("firewall %q: backend nftables not available", a.Resource.Name)
		}
		return nftB, nil
	case "":
		if firewalldB.available() {
			return firewalldB, nil
		}
		if nftB.available() {
			return nftB, nil
		}
		return nil, fmt.Errorf("firewall %q: no firewall backend available (firewalld or nftables)", a.Resource.Name)
	default:
		return nil, fmt.Errorf("firewall %q: unknown backend %q", a.Resource.Name, preferred)
	}
}

// auditLogHash returns a deterministic hash of the resource definition for deduplication.
func (a *Applicator) auditLogHash() string {
	// Hash the fields that determine what the rule would do.
	raw, _ := json.Marshal(struct {
		Action       string   `json:"action"`
		Protocol     string   `json:"protocol"`
		Ports        []int    `json:"ports"`
		Sources      []string `json:"sources"`
		Destinations []string `json:"destinations"`
		Services     []string `json:"services"`
		Zones        []string `json:"zones"`
		Backend      string   `json:"backend"`
		Table        string   `json:"table"`
		Chain        string   `json:"chain"`
		Family       string   `json:"family"`
		Rule         string   `json:"rule"`
	}{
		Action:       a.Resource.Action,
		Protocol:     a.Resource.Protocol,
		Ports:        a.Resource.Ports,
		Sources:      a.Resource.Sources,
		Destinations: a.Resource.Destinations,
		Services:     a.Resource.Services,
		Zones:        a.Resource.Zones,
		Backend:      a.Resource.Backend,
		Table:        a.Resource.Table,
		Chain:        a.Resource.Chain,
		Family:       a.Resource.Family,
		Rule:         a.Resource.Rule,
	})
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:16])
}

// auditLogUpToDate checks whether the audit log already contains a current entry for this rule.
func (a *Applicator) auditLogUpToDate() (bool, error) {
	path := a.auditLogPath()
	data, err := os.ReadFile(path) // #nosec G304 -- path is a fixed constant or explicitly configured
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	wantHash := a.auditLogHash()
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry auditLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.RuleName == a.Resource.Name && entry.Hash == wantHash {
			return true, nil
		}
	}
	return false, nil
}

// writeAuditLog appends a JSON Lines entry describing what the rule would have done.
func (a *Applicator) writeAuditLog() error {
	path := a.auditLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("audit log dir: %w", err)
	}

	b, _ := a.resolveBackend()
	backendName := "unknown"
	if b != nil {
		backendName = b.name()
	}

	entry := auditLogEntry{
		Timestamp: time.Now().UTC(),
		RuleName:  a.Resource.Name,
		Hash:      a.auditLogHash(),
		Action:    a.Resource.Action,
		Protocol:  a.Resource.Protocol,
		Ports:     a.Resource.Ports,
		Sources:   a.Resource.Sources,
		Backend:   backendName,
		WouldHave: a.describeWouldHave(),
		Enforced:  false,
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- path is a fixed constant or explicitly configured
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, string(raw))
	return err
}

func (a *Applicator) describeWouldHave() string {
	b, _ := a.resolveBackend()
	if b == nil {
		return fmt.Sprintf("unknown backend: apply %s rule %s", a.Resource.Action, a.Resource.Name)
	}
	return fmt.Sprintf("[%s] would %s rule %s", b.name(), a.Resource.Action, a.Resource.Name)
}

func (a *Applicator) auditLogPath() string {
	if a.AuditPath != "" {
		return a.AuditPath
	}
	return defaultAuditLogPath
}

// validateSyncPath prevents rules that would block the agent's sync path.
func (a *Applicator) validateSyncPath() error {
	if a.SyncURL == "" {
		return nil
	}

	u, err := url.Parse(a.SyncURL)
	if err != nil {
		return nil // can't validate malformed URL
	}

	syncPort := u.Port()
	if syncPort == "" {
		switch u.Scheme {
		case "https":
			syncPort = "443"
		case "http":
			syncPort = "80"
		default:
			return nil
		}
	}
	portNum, _ := strconv.Atoi(syncPort)

	action := strings.ToLower(a.Resource.Action)
	if action != "deny" && action != "drop" && action != "reject" {
		return nil // allow rules don't block
	}

	// Check if this rule explicitly targets the sync port.
	for _, p := range a.Resource.Ports {
		if p == portNum {
			return fmt.Errorf("rule would block sync port %s to %s (set protectRemotr: false to override)", syncPort, a.SyncURL)
		}
	}

	// If no ports are specified and the protocol is tcp (or unspecified), the rule
	// is broad enough that it could block sync. Be conservative.
	if len(a.Resource.Ports) == 0 && len(a.Resource.Services) == 0 {
		proto := strings.ToLower(a.Resource.Protocol)
		if proto == "" || proto == "tcp" {
			return fmt.Errorf("broad deny/drop/reject rule without port restriction could block sync to %s (set protectRemotr: false to override)", a.SyncURL)
		}
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// cmdRunnerAdapter wraps executil.Runner to satisfy probe.CommandRunner.
type cmdRunnerAdapter struct {
	runner executil.Runner
}

func (a cmdRunnerAdapter) Run(name string, args ...string) ([]byte, error) {
	stdout, _, err := a.runner.Run(name, args...)
	return stdout, err
}

// auditLogEntry is one line in the firewall audit log.
type auditLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	RuleName  string    `json:"ruleName"`
	Hash      string    `json:"hash"`
	Action    string    `json:"action"`
	Protocol  string    `json:"protocol,omitempty"`
	Ports     []int     `json:"ports,omitempty"`
	Sources   []string  `json:"sources,omitempty"`
	Backend   string    `json:"backend"`
	WouldHave string    `json:"wouldHave"`
	Enforced  bool      `json:"enforced"`
}
