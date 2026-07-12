package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/applicators/agentinstall"
	"github.com/DavidHoenisch/remotr/internal/applicators/bootstrap"
	"github.com/DavidHoenisch/remotr/internal/applicators/command"
	"github.com/DavidHoenisch/remotr/internal/applicators/downloads"
	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/applicators/firewall"
	pkgfactory "github.com/DavidHoenisch/remotr/internal/applicators/packages"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemd"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
	"github.com/DavidHoenisch/remotr/internal/applicators/userfiles"
	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Policy controls whether drift triggers apply.
type Policy string

const (
	PolicyAuto   Policy = "auto"
	PolicyReport Policy = "report"
)

type Kind int

const (
	KindPackage Kind = iota
	KindFile
	KindDownload
	KindFileCritical
	KindUser
	KindUserFile
	KindSystemd
	KindSystemdUser
	KindBootstrap
	KindAgentInstall
	KindFirewall
	KindCommand
)

type node struct {
	Address            string
	ConfigName         string
	Name               string
	Kind               Kind
	Handler            executor.Handler
	DependsOn          []string
	PreApplyValidation []string
	Risk               models.RiskClass
	Enforce            *bool
	LockDomains        []string
}

// ExecutionResource supplies one resolved resource to the execution engine.
// It is useful to callers that have already resolved resource handlers, such
// as a resource registry, and gives tests a public Agent execution seam.
type ExecutionResource struct {
	Address            string
	Name               string
	Kind               Kind
	Handler            executor.Handler
	DependsOn          []string
	PreApplyValidation []string
	Risk               models.RiskClass
	Enforce            *bool
	LockDomains        []string
}

// DriftItem describes one resource out of compliance.
type DriftItem struct {
	Address     string
	Name        string
	Description string
}

// DriftReport summarizes check results.
type DriftReport struct {
	Items        []DriftItem
	InCompliance bool
}

// ApplyResult summarizes an apply run.
type ApplyResult struct {
	Applied     []string
	Skipped     []string
	Activations []executor.ActivationSignal
	Failed      *ApplyFailure
}

type ApplyFailure struct {
	Address  string
	Err      error
	Rollback *executor.RollbackResult
}

// Option configures Engine creation.
type Option func(*Engine)

// WithSyncURL sets the agent sync URL so the firewall applicator can prevent lockouts.
func WithSyncURL(url string) Option {
	return func(e *Engine) {
		e.syncURL = url
	}
}

// WithActivator replaces post-Apply activation execution.
func WithActivator(activator executor.Activator) Option {
	return func(e *Engine) {
		if activator != nil {
			e.activator = activator
		}
	}
}

// Engine runs check/apply over resolved desired state.
type Engine struct {
	nodes     []node
	exec      executil.Runner
	executor  *executor.Applicator
	locks     *executor.LockManager
	activator executor.Activator
	syncURL   string
}

// New builds an engine from resolved state.
func New(resolved resolve.ResolvedState, f facts.Facts, exec executil.Runner, pkgURLs apppackages.URLResolver, opts ...Option) (*Engine, error) {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	e := &Engine{exec: exec, executor: executor.New(), locks: executor.NewLockManager(), activator: systemActivator{runner: exec}}
	for _, opt := range opts {
		opt(e)
	}
	nodes, err := buildNodes(resolved, f, exec, pkgURLs, e.syncURL)
	if err != nil {
		return nil, err
	}
	if err := validateNodeRisks(nodes); err != nil {
		return nil, err
	}
	order, err := sortNodes(nodes)
	if err != nil {
		return nil, err
	}
	e.nodes = order
	return e, nil
}

// NewForExecution builds an Engine from caller-provided resolved resources.
// It preserves the same ordering and dependency validation as New.
func NewForExecution(resources []ExecutionResource, exec executil.Runner, opts ...Option) (*Engine, error) {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	e := &Engine{exec: exec, executor: executor.New(), locks: executor.NewLockManager(), activator: systemActivator{runner: exec}}
	for _, opt := range opts {
		opt(e)
	}
	nodes := make([]node, 0, len(resources))
	addresses := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if resource.Address == "" {
			return nil, fmt.Errorf("execution resource address is required")
		}
		if resource.Handler == nil {
			return nil, fmt.Errorf("execution resource %q handler is required", resource.Address)
		}
		if _, exists := addresses[resource.Address]; exists {
			return nil, fmt.Errorf("duplicate execution resource %q", resource.Address)
		}
		risk := resource.Risk
		if risk == "" {
			risk = models.RiskNormal
		}
		if !risk.Valid() {
			return nil, fmt.Errorf("execution resource %q has unknown risk %q", resource.Address, risk)
		}
		addresses[resource.Address] = struct{}{}
		nodes = append(nodes, node{
			Address:            resource.Address,
			Name:               resource.Name,
			Kind:               resource.Kind,
			Handler:            resource.Handler,
			DependsOn:          append([]string(nil), resource.DependsOn...),
			PreApplyValidation: append([]string(nil), resource.PreApplyValidation...),
			Risk:               risk,
			Enforce:            resource.Enforce,
			LockDomains:        append([]string(nil), resource.LockDomains...),
		})
	}
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if _, exists := addresses[dep]; !exists {
				return nil, fmt.Errorf("unknown dependency %q for resource %q", dep, n.Address)
			}
		}
	}
	order, err := sortNodes(nodes)
	if err != nil {
		return nil, err
	}
	e.nodes = order
	return e, nil
}

func validateNodeRisks(nodes []node) error {
	for _, n := range nodes {
		if !n.Risk.Valid() {
			return fmt.Errorf("resource %q has unknown risk %q", n.Address, n.Risk)
		}
	}
	return nil
}

func buildNodes(resolved resolve.ResolvedState, f facts.Facts, exec executil.Runner, pkgURLs apppackages.URLResolver, syncURL string) ([]node, error) {
	var nodes []node
	addresses := map[string]struct{}{}

	add := func(n node) {
		nodes = append(nodes, n)
		addresses[n.Address] = struct{}{}
	}

	for _, cfg := range resolved.Configurations {
		for _, pkg := range cfg.Packages {
			h, err := pkgfactory.SelectPackageApplicator(f.Distro, pkg, f, exec, pkgURLs)
			if err != nil {
				return nil, err
			}
			add(node{
				Address:            models.ResourceAddress(cfg.Name, pkg.Name),
				ConfigName:         cfg.Name,
				Name:               pkg.Name,
				Kind:               KindPackage,
				Handler:            h,
				DependsOn:          append([]string(nil), pkg.DependsOn...),
				PreApplyValidation: append([]string(nil), pkg.PreApplyValidation...),
				Risk:               pkg.EffectiveRisk(models.RiskNormal),
				Enforce:            pkg.Enforce,
				LockDomains:        pkg.EffectiveLockDomains("package-database"),
			})
		}
		for _, file := range cfg.Files {
			kind := KindFile
			if isCriticalFile(file) {
				kind = KindFileCritical
			}
			add(node{
				Address:            models.ResourceAddress(cfg.Name, file.Name),
				ConfigName:         cfg.Name,
				Name:               file.Name,
				Kind:               kind,
				Handler:            files.New(file),
				DependsOn:          append([]string(nil), file.DependsOn...),
				PreApplyValidation: append([]string(nil), file.PreApplyValidation...),
				Risk:               fileRisk(file, kind),
				Enforce:            file.Enforce,
				LockDomains:        file.EffectiveLockDomains(),
			})
		}
		for _, dl := range cfg.Downloads {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, dl.Name),
				ConfigName:         cfg.Name,
				Name:               dl.Name,
				Kind:               KindDownload,
				Handler:            downloads.New(dl, exec),
				DependsOn:          append([]string(nil), dl.DependsOn...),
				PreApplyValidation: append([]string(nil), dl.PreApplyValidation...),
				Risk:               dl.EffectiveRisk(models.RiskNormal),
				Enforce:            dl.Enforce,
				LockDomains:        dl.EffectiveLockDomains(),
			})
		}
		for _, u := range cfg.Users {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, u.Name),
				ConfigName:         cfg.Name,
				Name:               u.Name,
				Kind:               KindUser,
				Handler:            users.New(u),
				DependsOn:          append([]string(nil), u.DependsOn...),
				PreApplyValidation: append([]string(nil), u.PreApplyValidation...),
				Risk:               u.EffectiveRisk(models.RiskAccess),
				Enforce:            u.Enforce,
				LockDomains:        u.EffectiveLockDomains("account-database"),
			})
		}
		for _, uf := range cfg.UserFiles {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, uf.Name),
				ConfigName:         cfg.Name,
				Name:               uf.Name,
				Kind:               KindUserFile,
				Handler:            userfiles.New(uf),
				DependsOn:          append([]string(nil), uf.DependsOn...),
				PreApplyValidation: append([]string(nil), uf.PreApplyValidation...),
				Risk:               uf.EffectiveRisk(models.RiskNormal),
				Enforce:            uf.Enforce,
				LockDomains:        uf.EffectiveLockDomains(),
			})
		}
		for _, s := range cfg.Systemd {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, s.Name),
				ConfigName:         cfg.Name,
				Name:               s.Name,
				Kind:               KindSystemd,
				Handler:            systemd.New(s, exec),
				DependsOn:          append([]string(nil), s.DependsOn...),
				PreApplyValidation: append([]string(nil), s.PreApplyValidation...),
				Risk:               s.EffectiveRisk(models.RiskNormal),
				Enforce:            s.Enforce,
				LockDomains:        s.EffectiveLockDomains(),
			})
		}
		for _, su := range cfg.SystemdUser {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, su.Name),
				ConfigName:         cfg.Name,
				Name:               su.Name,
				Kind:               KindSystemdUser,
				Handler:            systemduser.New(su, exec),
				DependsOn:          append([]string(nil), su.DependsOn...),
				PreApplyValidation: append([]string(nil), su.PreApplyValidation...),
				Risk:               su.EffectiveRisk(models.RiskNormal),
				Enforce:            su.Enforce,
				LockDomains:        su.EffectiveLockDomains(),
			})
		}
		for _, b := range cfg.Bootstrap {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, b.Name),
				ConfigName:         cfg.Name,
				Name:               b.Name,
				Kind:               KindBootstrap,
				Handler:            bootstrap.New(b, exec),
				DependsOn:          append([]string(nil), b.DependsOn...),
				PreApplyValidation: append([]string(nil), b.PreApplyValidation...),
				Risk:               b.EffectiveRisk(models.RiskBoot),
				Enforce:            b.Enforce,
				LockDomains:        b.EffectiveLockDomains(),
			})
		}
		for _, ag := range cfg.AgentInstall {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, ag.Name),
				ConfigName:         cfg.Name,
				Name:               ag.Name,
				Kind:               KindAgentInstall,
				Handler:            agentinstall.New(ag, exec),
				DependsOn:          append([]string(nil), ag.DependsOn...),
				PreApplyValidation: append([]string(nil), ag.PreApplyValidation...),
				Risk:               ag.EffectiveRisk(models.RiskSensitive),
				Enforce:            ag.Enforce,
				LockDomains:        ag.EffectiveLockDomains(),
			})
		}
		for _, fw := range cfg.Firewall {
			fwApp := firewall.New(fw, exec)
			fwApp.SyncURL = syncURL
			add(node{
				Address:            models.ResourceAddress(cfg.Name, fw.Name),
				ConfigName:         cfg.Name,
				Name:               fw.Name,
				Kind:               KindFirewall,
				Handler:            fwApp,
				DependsOn:          append([]string(nil), fw.DependsOn...),
				PreApplyValidation: append([]string(nil), fw.PreApplyValidation...),
				Risk:               fw.EffectiveRisk(models.RiskConnectivity),
				Enforce:            fw.Enforce,
				LockDomains:        fw.EffectiveLockDomains("firewall"),
			})
		}
		for _, c := range cfg.Commands {
			add(node{
				Address:            models.ResourceAddress(cfg.Name, c.Name),
				ConfigName:         cfg.Name,
				Name:               c.Name,
				Kind:               KindCommand,
				Handler:            command.New(c, exec),
				DependsOn:          append([]string(nil), c.DependsOn...),
				PreApplyValidation: append([]string(nil), c.PreApplyValidation...),
				Risk:               c.EffectiveRisk(models.RiskDestructive),
				Enforce:            c.Enforce,
				LockDomains:        c.EffectiveLockDomains(),
			})
		}
	}

	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if _, ok := addresses[dep]; !ok {
				return nil, fmt.Errorf("unknown dependency %q for resource %q", dep, n.Address)
			}
		}
	}
	return nodes, nil
}

func isCriticalFile(f models.File) bool {
	if len(f.PreApplyValidation) > 0 {
		return true
	}
	return strings.HasPrefix(f.Path, "/etc/ssh")
}

func fileRisk(f models.File, kind Kind) models.RiskClass {
	defaultRisk := models.RiskNormal
	if kind == KindFileCritical {
		defaultRisk = models.RiskAccess
	}
	return f.EffectiveRisk(defaultRisk)
}

func defaultTier(k Kind) int {
	switch k {
	case KindPackage:
		return 0
	case KindFile:
		return 1
	case KindDownload:
		return 2
	case KindFileCritical:
		return 3
	case KindUser:
		return 4
	case KindUserFile:
		return 5
	case KindFirewall:
		return 6
	case KindSystemd:
		return 7
	case KindSystemdUser:
		return 8
	case KindBootstrap:
		return 9
	case KindAgentInstall:
		return 10
	case KindCommand:
		return 11
	default:
		return 99
	}
}

// sortNodes orders nodes by depends_on (topological) with default tier tiebreaker.
func sortNodes(nodes []node) ([]node, error) {
	byAddr := make(map[string]node, len(nodes))
	inDegree := make(map[string]int, len(nodes))
	adj := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		byAddr[n.Address] = n
		inDegree[n.Address] = 0
	}
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			adj[dep] = append(adj[dep], n.Address)
			inDegree[n.Address]++
		}
	}

	var queue []string
	for addr, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, addr)
		}
	}
	sortQueue(queue, byAddr)

	var order []node
	for len(queue) > 0 {
		addr := queue[0]
		queue = queue[1:]
		order = append(order, byAddr[addr])
		for _, next := range adj[addr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
		sortQueue(queue, byAddr)
	}
	if len(order) != len(nodes) {
		return nil, fmt.Errorf("dependency cycle detected")
	}
	return order, nil
}

func sortQueue(addrs []string, byAddr map[string]node) {
	for i := 0; i < len(addrs); i++ {
		for j := i + 1; j < len(addrs); j++ {
			a, b := byAddr[addrs[i]], byAddr[addrs[j]]
			ta, tb := defaultTier(a.Kind), defaultTier(b.Kind)
			if tb < ta || (tb == ta && addrs[j] < addrs[i]) {
				addrs[i], addrs[j] = addrs[j], addrs[i]
			}
		}
	}
}

// CheckAll returns drift for all resources.
func (e *Engine) CheckAll(ctx context.Context) DriftReport {
	return e.driftReport(e.checkAll(ctx))
}

func (e *Engine) checkAll(ctx context.Context) map[string]executor.CheckResult {
	checks := make(map[string]executor.CheckResult, len(e.nodes))
	for _, n := range e.nodes {
		checks[n.Address] = executor.Check(ctx, n.Handler)
	}
	return checks
}

func (e *Engine) driftReport(checks map[string]executor.CheckResult) DriftReport {
	var items []DriftItem
	inCompliance := true
	for _, n := range e.nodes {
		check := checks[n.Address]
		if check.Status != executor.Compliant {
			inCompliance = false
		}
		if check.Status == executor.Drifted {
			items = append(items, DriftItem{
				Address:     n.Address,
				Name:        n.Name,
				Description: n.Handler.Description(),
			})
		}
	}
	return DriftReport{Items: items, InCompliance: inCompliance}
}

// ApplyAll applies drifted resources in order when policy is auto.
func (e *Engine) ApplyAll(ctx context.Context, policy Policy) ApplyResult {
	checks := e.checkAll(ctx)
	result := ApplyResult{}
	if policy == PolicyReport {
		for _, n := range e.nodes {
			if checks[n.Address].Status == executor.Drifted {
				result.Skipped = append(result.Skipped, n.Address)
			}
		}
		return result
	}
	applied := make(map[string]bool, len(e.nodes))
	applyResults := make([]executor.ApplyResult, 0, len(e.nodes))
	for _, n := range e.nodes {
		check := checks[n.Address]
		if check.Status == executor.Compliant {
			continue
		}
		if check.Status != executor.Drifted || !dependenciesReady(n, checks, applied) {
			result.Skipped = append(result.Skipped, n.Address)
			continue
		}
		if n.Risk.RequiresPreflight() && (n.Enforce == nil || !*n.Enforce) {
			result.Skipped = append(result.Skipped, n.Address)
			continue
		}
		releaseLocks, err := e.acquireOperationLocks(ctx, n)
		if err != nil {
			result.Failed = &ApplyFailure{Address: n.Address, Err: err}
			return result
		}
		if err := e.runPreflight(ctx, n); err != nil {
			releaseLocks()
			result.Failed = &ApplyFailure{Address: n.Address, Err: err}
			return result
		}
		applyResult := e.executor.ApplyState(ctx, n.Handler)
		releaseLocks()
		switch applyResult.Status {
		case executor.Failed:
			result.Failed = &ApplyFailure{Address: n.Address, Err: applyResult.Err, Rollback: applyResult.Rollback}
			return result
		case executor.ApplyDeferred:
			result.Skipped = append(result.Skipped, n.Address)
		case executor.Changed:
			applied[n.Address] = true
			result.Applied = append(result.Applied, n.Address)
			applyResults = append(applyResults, applyResult)
		case executor.NoChange:
			applied[n.Address] = true
		}
	}
	result.Activations = executor.CollectActivations(applyResults)
	if len(result.Activations) > 0 {
		if err := e.activator.Activate(ctx, result.Activations); err != nil {
			result.Failed = &ApplyFailure{Address: "activation", Err: err}
		}
	}
	return result
}

func (e *Engine) acquireOperationLocks(ctx context.Context, n node) (func(), error) {
	releaseDomains, err := e.locks.Acquire(ctx, n.LockDomains)
	if err != nil {
		return nil, fmt.Errorf("acquire lock domains for %s: %w", n.Address, err)
	}
	native, ok := n.Handler.(executor.NativeLocker)
	if !ok {
		return releaseDomains, nil
	}
	releaseNative, err := native.AcquireNativeLocks(ctx)
	if err != nil {
		releaseDomains()
		return nil, fmt.Errorf("acquire native locks for %s: %w", n.Address, err)
	}
	if releaseNative == nil {
		releaseNative = func() {}
	}
	return func() {
		releaseNative()
		releaseDomains()
	}, nil
}

func (e *Engine) runPreflight(ctx context.Context, n node) error {
	if n.Risk.RequiresPreflight() {
		preflighter, ok := n.Handler.(executor.Preflighter)
		if !ok {
			return fmt.Errorf("resource %s requires a %s-risk preflight", n.Address, n.Risk)
		}
		if err := preflighter.Preflight(ctx); err != nil {
			return fmt.Errorf("preflight for %s: %w", n.Address, err)
		}
	}
	return e.runPreApplyValidation(n)
}

func dependenciesReady(n node, checks map[string]executor.CheckResult, applied map[string]bool) bool {
	for _, dependency := range n.DependsOn {
		if checks[dependency].Status == executor.Compliant || applied[dependency] {
			continue
		}
		return false
	}
	return true
}

func (e *Engine) runPreApplyValidation(n node) error {
	for _, cmdline := range n.PreApplyValidation {
		parts := strings.Fields(cmdline)
		if len(parts) == 0 {
			continue
		}
		_, _, err := e.exec.Run(parts[0], parts[1:]...)
		if err != nil {
			return fmt.Errorf("pre-apply validation %q for %s: %w", cmdline, n.Address, err)
		}
	}
	return nil
}

// NodeCount returns the number of resources (for tests).
func (e *Engine) NodeCount() int { return len(e.nodes) }

// NodeOrder returns resource addresses in apply order (for tests).
func (e *Engine) NodeOrder() []string {
	out := make([]string, len(e.nodes))
	for i, n := range e.nodes {
		out[i] = n.Address
	}
	return out
}
