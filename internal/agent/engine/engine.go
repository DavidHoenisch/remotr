package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// Policy controls whether drift triggers apply.
type Policy string

const (
	PolicyAuto   Policy = "auto"
	PolicyReport Policy = "report"
)

type Kind = models.ResourceKind

const (
	KindPackage          = models.ResourceKindPackage
	KindAPTSigningKey    = models.ResourceKindAPTSigningKey
	KindAPTRepository    = models.ResourceKindAPTRepository
	KindSysctl           = models.ResourceKindSysctl
	KindHostname         = models.ResourceKindHostname
	KindHostLocale       = models.ResourceKindHostLocale
	KindTimeSync         = models.ResourceKindTimeSync
	KindMount            = models.ResourceKindMount
	KindFile             = models.ResourceKindFile
	KindDownload         = models.ResourceKindDownload
	KindFileCritical     = models.ResourceKind("fileCritical")
	KindUser             = models.ResourceKindUser
	KindAuthorizedKey    = models.ResourceKindAuthorizedKey
	KindKnownHost        = models.ResourceKindKnownHost
	KindSudo             = models.ResourceKindSudo
	KindUserFile         = models.ResourceKindUserFile
	KindSystemd          = models.ResourceKindSystemd
	KindService          = models.ResourceKindService
	KindSystemdUnit      = models.ResourceKindSystemdUnit
	KindReboot           = models.ResourceKindReboot
	KindEndpointSchedule = models.ResourceKindEndpointSchedule
	KindSystemdUser      = models.ResourceKindSystemdUser
	KindBootstrap        = models.ResourceKindBootstrap
	KindAgentInstall     = models.ResourceKindAgentInstall
	KindFirewall         = models.ResourceKindFirewall
	KindHostsEntry       = models.ResourceKindHostsEntry
	KindDNSResolver      = models.ResourceKindDNSResolver
	KindRoute            = models.ResourceKindRoute
	KindNetworkProfile   = models.ResourceKindNetworkProfile
	KindCertificate      = models.ResourceKindCertificate
	KindTrustAnchor      = models.ResourceKindTrustAnchor
	KindAppArmorProfile  = models.ResourceKindAppArmorProfile
	KindAuditRules       = models.ResourceKindAuditRules
	KindAccountLimit     = models.ResourceKindAccountLimit
	KindLoginPolicy      = models.ResourceKindLoginPolicy
	KindJournald         = models.ResourceKindJournald
	KindLogrotate        = models.ResourceKindLogrotate
	KindCommand          = models.ResourceKindCommand
)

type node struct {
	Address            string
	ConfigName         string
	Name               string
	Kind               Kind
	Provider           string
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
	Provider           string
	Handler            executor.Handler
	DependsOn          []string
	PreApplyValidation []string
	Risk               models.RiskClass
	Enforce            *bool
	LockDomains        []string
}

// DriftItem describes one resource Check outcome.
type DriftItem struct {
	Address             string
	Name                string
	Description         string
	Provider            string
	Status              executor.CheckStatus
	ReasonCode          executor.ReasonCode
	DesiredSummary      executor.RedactedSummary
	ObservedSummary     executor.RedactedSummary
	Subresults          []executor.CheckSubresult
	SubresultsTruncated bool
}

// DriftReport summarizes check results.
type DriftReport struct {
	Items           []DriftItem
	ScheduleRuntime []ScheduleRuntimeItem
	InCompliance    bool
}

// ScheduleRuntimeItem is optional execution history for one endpoint-local
// schedule. It is not part of the schedule's configuration Check outcome.
type ScheduleRuntimeItem struct {
	Address           string
	Name              string
	Provider          string
	Status            executor.ScheduleRunStatus
	ExitCode          *int
	MissedRunBehavior executor.ScheduleMissedRunBehavior
}

// ApplyResult summarizes an apply run.
type ApplyResult struct {
	Applied     []string
	Skipped     []string
	Activations []executor.ActivationSignal
	Items       []ApplyItem
	Failed      *ApplyFailure
}

// ApplyItem is the redacted outcome of applying one resource. It is retained
// in Sync telemetry so the server can report more than a single failure.
type ApplyItem struct {
	Address         string
	Name            string
	Provider        string
	Status          executor.ApplyStatus
	ReasonCode      executor.ReasonCode
	DesiredSummary  executor.RedactedSummary
	ObservedSummary executor.RedactedSummary
	Activation      []executor.ActivationSignal
	RebootRequired  executor.RebootRequirement
	RollbackClass   executor.RollbackClass
	RollbackStatus  executor.RollbackStatus
	Diagnostics     []executor.RedactedSummary
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

// WithStateDir supplies durable endpoint-local state to providers that must
// survive agent restart, such as coordinated reboot.
func WithStateDir(dir string) Option {
	return func(e *Engine) {
		e.stateDir = dir
	}
}

// WithSecretResolver supplies provider-neutral local-file and Remotr secret
// resolution to secret-backed applicators.
func WithSecretResolver(resolver secrets.Resolver) Option {
	return func(e *Engine) { e.secretResolver = resolver }
}

// WithArtifactDigest binds secret resolution to the artifact being executed.
func WithArtifactDigest(digest string) Option {
	return func(e *Engine) { e.artifactDigest = digest }
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
	nodes          []node
	exec           executil.Runner
	executor       *executor.Applicator
	locks          *executor.LockManager
	activator      executor.Activator
	syncURL        string
	stateDir       string
	secretResolver secrets.Resolver
	artifactDigest string
	aptRefreshMu   sync.Mutex
	aptRefreshDone bool
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
	nodes, err := buildNodes(resolved, f, exec, pkgURLs, e.syncURL, e.stateDir, e.secretResolver, e.artifactDigest)
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
	e.configureAPTCacheRefresh(order)
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
			Provider:           providerIdentity(resource.Provider, resource.Handler),
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
	e.configureAPTCacheRefresh(order)
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

func buildNodes(resolved resolve.ResolvedState, f facts.Facts, exec executil.Runner, pkgURLs apppackages.URLResolver, syncURL, stateDir string, secretResolver secrets.Resolver, artifactDigest string) ([]node, error) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return nil, err
	}
	var nodes []node
	addresses := map[string]struct{}{}

	add := func(n node) {
		nodes = append(nodes, n)
		addresses[n.Address] = struct{}{}
	}

	for _, cfg := range resolved.Configurations {
		resources, err := registry.Resources(&cfg)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			address := models.ResourceAddress(cfg.Name, resource.Name())
			if err := resource.Validate(); err != nil {
				return nil, fmt.Errorf("resource %q: %w", address, err)
			}
			handler, err := resource.NewProvider(resourceregistry.FactoryContext{
				Facts: f, Runner: exec, PackageURLs: pkgURLs, SyncURL: syncURL, StateDir: stateDir,
				SecretResolver: secretResolver, ArtifactDigest: artifactDigest, ResourceAddress: address,
			})
			if err != nil {
				return nil, fmt.Errorf("resource %q: %w", address, err)
			}
			kind := resource.Kind()
			if kind == KindFile && resource.OrderingTier() == defaultTier(KindFileCritical) {
				kind = KindFileCritical
			}
			meta := resource.Metadata()
			add(node{
				Address:            address,
				ConfigName:         cfg.Name,
				Name:               resource.Name(),
				Kind:               kind,
				Provider:           providerIdentity("", handler),
				Handler:            handler,
				DependsOn:          append([]string(nil), meta.DependsOn...),
				PreApplyValidation: append([]string(nil), meta.PreApplyValidation...),
				Risk:               meta.EffectiveRisk(resource.DefaultRisk()),
				Enforce:            meta.Enforce,
				LockDomains:        resource.LockDomains(),
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

func defaultTier(k Kind) int {
	switch k {
	case KindPackage, KindAPTSigningKey:
		return 0
	case KindAPTRepository:
		return 1
	case KindSysctl:
		return 2
	case KindHostname:
		return 2
	case KindFile:
		return 1
	case KindDownload:
		return 2
	case KindFileCritical:
		return 3
	case KindUser:
		return 4
	case KindAuthorizedKey:
		return 5
	case KindKnownHost:
		return 5
	case KindSudo:
		return 6
	case KindUserFile:
		return 5
	case KindFirewall:
		return 6
	case KindHostsEntry, KindDNSResolver, KindRoute, KindNetworkProfile:
		return 6
	case KindSystemd:
		return 7
	case KindService:
		return 7
	case KindSystemdUnit:
		return 6
	case KindEndpointSchedule:
		return 7
	case KindSystemdUser:
		return 8
	case KindReboot, KindBootstrap:
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
	return e.driftReport(ctx, e.checkAll(ctx))
}

func (e *Engine) checkAll(ctx context.Context) map[string]executor.CheckResult {
	checks := make(map[string]executor.CheckResult, len(e.nodes))
	for _, n := range e.nodes {
		checks[n.Address] = executor.Check(ctx, n.Handler)
	}
	return checks
}

func (e *Engine) driftReport(ctx context.Context, checks map[string]executor.CheckResult) DriftReport {
	items := make([]DriftItem, 0, len(e.nodes))
	runtime := make([]ScheduleRuntimeItem, 0)
	inCompliance := true
	for _, n := range e.nodes {
		check := checks[n.Address]
		if check.Status != executor.Compliant {
			inCompliance = false
		}
		items = append(items, DriftItem{
			Address:             n.Address,
			Name:                n.Name,
			Description:         n.Handler.Description(),
			Provider:            n.Provider,
			Status:              check.Status,
			ReasonCode:          check.ReasonCode,
			DesiredSummary:      check.DesiredSummary,
			ObservedSummary:     check.ObservedSummary,
			Subresults:          append([]executor.CheckSubresult(nil), check.Subresults...),
			SubresultsTruncated: check.SubresultsTruncated,
		})
		if n.Kind == KindEndpointSchedule {
			if telemetry, ok := executor.ScheduleRuntime(ctx, n.Handler); ok {
				runtime = append(runtime, ScheduleRuntimeItem{
					Address: n.Address, Name: n.Name, Provider: n.Provider,
					Status: telemetry.Status, ExitCode: telemetry.ExitCode, MissedRunBehavior: telemetry.MissedRunBehavior,
				})
			}
		}
	}
	return DriftReport{Items: items, ScheduleRuntime: runtime, InCompliance: inCompliance}
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
		if err := e.refreshAPTMetadata(ctx, n, applied); err != nil {
			releaseLocks()
			result.Failed = &ApplyFailure{Address: n.Address, Err: err}
			return result
		}
		applyResult := e.executor.ApplyState(ctx, n.Handler)
		releaseLocks()
		result.Items = append(result.Items, applyItem(n, check, applyResult))
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

type aptCacheRefreshSetter interface {
	SetCacheRefresh(func(context.Context) error)
}

type aptCacheRefresher interface {
	RefreshCache(context.Context) error
}

func (e *Engine) configureAPTCacheRefresh(nodes []node) {
	for _, node := range nodes {
		if handler, ok := node.Handler.(aptCacheRefreshSetter); ok {
			handler.SetCacheRefresh(e.refreshAPTCache)
		}
	}
}

func (e *Engine) refreshAPTMetadata(ctx context.Context, n node, applied map[string]bool) error {
	if n.Kind != KindPackage {
		return nil
	}
	for _, dependency := range n.DependsOn {
		if !applied[dependency] || !e.isAPTRepository(dependency) {
			continue
		}
		if handler, ok := n.Handler.(aptCacheRefresher); ok {
			return handler.RefreshCache(ctx)
		}
	}
	return nil
}

func (e *Engine) isAPTRepository(address string) bool {
	for _, node := range e.nodes {
		if node.Address == address {
			return node.Kind == KindAPTRepository
		}
	}
	return false
}

func (e *Engine) refreshAPTCache(ctx context.Context) error {
	e.aptRefreshMu.Lock()
	defer e.aptRefreshMu.Unlock()
	if e.aptRefreshDone {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, stderr, err := e.exec.Run("apt-get", "update")
	if err != nil {
		return fmt.Errorf("refresh APT package metadata: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	e.aptRefreshDone = true
	return nil
}

func providerIdentity(explicit string, handler executor.Handler) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit
	}
	name := strings.TrimSpace(handler.Name())
	if provider, _, found := strings.Cut(name, ":"); found && provider != "" {
		return provider
	}
	return name
}

func applyItem(n node, check executor.CheckResult, result executor.ApplyResult) ApplyItem {
	item := ApplyItem{
		Address:         n.Address,
		Name:            n.Name,
		Provider:        n.Provider,
		Status:          result.Status,
		ReasonCode:      applyReasonCode(result),
		DesiredSummary:  check.DesiredSummary,
		ObservedSummary: check.ObservedSummary,
		Activation:      append([]executor.ActivationSignal(nil), result.Activation...),
		RebootRequired:  result.RebootRequired,
		RollbackClass:   result.RollbackClass,
		Diagnostics:     append([]executor.RedactedSummary(nil), result.Diagnostics...),
	}
	if result.Rollback != nil {
		item.RollbackStatus = result.Rollback.Status
	}
	return item
}

func applyReasonCode(result executor.ApplyResult) executor.ReasonCode {
	switch result.Status {
	case executor.Changed:
		return "applied"
	case executor.NoChange:
		return "already_compliant"
	case executor.ApplyDeferred:
		if result.DeferredWork != nil {
			return result.DeferredWork.ReasonCode
		}
		return executor.ReasonDeferred
	case executor.Failed:
		return "apply_failed"
	default:
		return "apply_unknown"
	}
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
