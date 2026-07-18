package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
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
	KindDesktopSetting   = models.ResourceKindDesktopSetting
	KindSessionPolicy    = models.ResourceKindSessionPolicy
	KindBrowserPolicy    = models.ResourceKindBrowserPolicy
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
	ProviderRevision   string
	EffectiveHash      string
	Handler            executor.Handler
	DesiredSummary     executor.SafeSummary
	DependsOn          []string
	PreApplyValidation []string
	Risk               models.RiskClass
	RollbackClass      executor.RollbackClass
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
	ProviderRevision   string
	EffectiveHash      string
	Handler            executor.Handler
	DesiredSummary     executor.SafeSummary
	DependsOn          []string
	PreApplyValidation []string
	Risk               models.RiskClass
	RollbackClass      executor.RollbackClass
	Enforce            *bool
	LockDomains        []string
}

// PreflightStatus is the closed, non-enforcing provider readiness result
// attached to authenticated endpoint plan evidence.
type PreflightStatus string

const (
	PreflightNotRequired PreflightStatus = "not_required"
	PreflightReady       PreflightStatus = "ready"
	PreflightBlocked     PreflightStatus = "blocked"
)

// DriftItem describes one resource Check outcome.
type DriftItem struct {
	Address             string
	Name                string
	Description         string
	Provider            string
	ProviderRevision    string
	EffectiveHash       string
	Status              executor.CheckStatus
	ReasonCode          executor.ReasonCode
	PreflightStatus     PreflightStatus
	PreflightReason     executor.ReasonCode
	DesiredSummary      executor.SafeSummary
	ObservedSummary     executor.SafeSummary
	Subresults          []CheckSubresult
	SubresultsTruncated bool
}

// CheckSubresult is the sink-safe child outcome of a provider Check. Provider
// supplied desired/observed prose is replaced by classified health summaries.
type CheckSubresult struct {
	Target          string
	Status          executor.CheckStatus
	ReasonCode      executor.ReasonCode
	DesiredSummary  executor.SafeSummary
	ObservedSummary executor.SafeSummary
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
	Address          string
	Name             string
	Provider         string
	ProviderRevision string
	EffectiveHash    string
	Status           executor.ApplyStatus
	ReasonCode       executor.ReasonCode
	DesiredSummary   executor.SafeSummary
	ObservedSummary  executor.SafeSummary
	Activation       []executor.ActivationSignal
	RebootRequired   executor.RebootRequirement
	RollbackClass    executor.RollbackClass
	RollbackStatus   executor.RollbackStatus
	Diagnostics      []executor.SafeSummary
}

type ApplyFailure struct {
	Address        string
	Err            executor.SafeError
	RollbackStatus executor.RollbackStatus
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

// WithExecutionLeases supplies the authenticated, bounded authorizations that
// may admit high-risk mutation for their exact canonical resource hashes.
func WithExecutionLeases(leases []changecontrol.ExecutionLease) Option {
	return func(e *Engine) {
		e.executionLeases = make([]changecontrol.ExecutionLease, len(leases))
		for index, lease := range leases {
			e.executionLeases[index] = lease
			e.executionLeases[index].ResourceHashes = cloneStringMap(lease.ResourceHashes)
		}
	}
}

// WithClock injects the authorization clock for deterministic lease expiry.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
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
	nodes           []node
	exec            executil.Runner
	executor        *executor.Applicator
	locks           *executor.LockManager
	activator       executor.Activator
	syncURL         string
	stateDir        string
	secretResolver  secrets.Resolver
	artifactDigest  string
	executionLeases []changecontrol.ExecutionLease
	now             func() time.Time
	aptRefreshMu    sync.Mutex
	aptRefreshDone  bool
}

// New builds an engine from resolved state.
func New(resolved resolve.ResolvedState, f facts.Facts, exec executil.Runner, pkgURLs apppackages.URLResolver, opts ...Option) (*Engine, error) {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	e := &Engine{exec: exec, executor: executor.New(), locks: executor.NewLockManager(), activator: systemActivator{runner: exec}, now: time.Now}
	for _, opt := range opts {
		opt(e)
	}
	if err := e.validateExecutionLeases(); err != nil {
		return nil, err
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
	e := &Engine{exec: exec, executor: executor.New(), locks: executor.NewLockManager(), activator: systemActivator{runner: exec}, now: time.Now}
	for _, opt := range opts {
		opt(e)
	}
	if err := e.validateExecutionLeases(); err != nil {
		return nil, err
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
		rollbackClass := resource.RollbackClass
		if rollbackClass == "" {
			rollbackClass = executor.RollbackNone
		}
		switch rollbackClass {
		case executor.RollbackNone, executor.RollbackBestEffort, executor.RollbackTransactional:
		default:
			return nil, fmt.Errorf("execution resource %q has unknown rollback class %q", resource.Address, rollbackClass)
		}
		addresses[resource.Address] = struct{}{}
		if err := resource.DesiredSummary.Validate(); err != nil {
			return nil, fmt.Errorf("execution resource %q desired summary: %w", resource.Address, err)
		}
		nodes = append(nodes, node{
			Address:            resource.Address,
			Name:               resource.Name,
			Kind:               resource.Kind,
			Provider:           providerIdentity(resource.Provider, resource.Handler),
			ProviderRevision:   resource.ProviderRevision,
			EffectiveHash:      resource.EffectiveHash,
			Handler:            resource.Handler,
			DesiredSummary:     resource.DesiredSummary.Clone(),
			DependsOn:          append([]string(nil), resource.DependsOn...),
			PreApplyValidation: append([]string(nil), resource.PreApplyValidation...),
			Risk:               risk,
			RollbackClass:      rollbackClass,
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
			if source, ok := resolved.ResourceSources[address]; ok {
				resource, err = resource.BindSource(&source)
				if err != nil {
					return nil, fmt.Errorf("resource %q source: %w", address, err)
				}
			}
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
			desiredSummary, err := resource.SafeSummary()
			if err != nil {
				return nil, fmt.Errorf("resource %q safe projection: %w", address, err)
			}
			kind := resource.Kind()
			if kind == KindFile && resource.OrderingTier() == defaultTier(KindFileCritical) {
				kind = KindFileCritical
			}
			selectedProvider := providerIdentity("", handler)
			planDescriptor, err := resource.PlanDescriptor(selectedProvider)
			if err != nil {
				return nil, fmt.Errorf("resource %q plan descriptor: %w", address, err)
			}
			effectiveHash := ""
			if _, ok := resolved.ResourceSources[address]; ok {
				effectiveHash, err = resource.ResolveEffectiveHash(context.Background(), address, selectedProvider, artifactDigest, secretResolver)
				if err != nil {
					return nil, fmt.Errorf("resource %q effective hash: %w", address, err)
				}
			}
			meta := resource.Metadata()
			add(node{
				Address:            address,
				ConfigName:         cfg.Name,
				Name:               resource.Name(),
				Kind:               kind,
				Provider:           selectedProvider,
				ProviderRevision:   resource.ProviderContractRevision(),
				EffectiveHash:      effectiveHash,
				Handler:            handler,
				DesiredSummary:     desiredSummary,
				DependsOn:          append([]string(nil), meta.DependsOn...),
				PreApplyValidation: append([]string(nil), meta.PreApplyValidation...),
				Risk:               meta.EffectiveRisk(resource.DefaultRisk()),
				RollbackClass:      planDescriptor.RollbackClass,
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
	preflights := make(map[string]struct {
		status PreflightStatus
		reason executor.ReasonCode
	}, len(e.nodes))
	for _, n := range e.nodes {
		status, reason := e.planPreflight(ctx, n, checks[n.Address])
		if status != PreflightBlocked {
			for _, dependency := range n.DependsOn {
				dependencyEvidence := preflights[dependency]
				dependencyCheck := checks[dependency]
				if dependencyEvidence.status == PreflightBlocked || (dependencyCheck.Status != executor.Compliant && dependencyCheck.Status != executor.Drifted) {
					status = PreflightBlocked
					reason = executor.ReasonDependencyBlocked
					break
				}
			}
		}
		preflights[n.Address] = struct {
			status PreflightStatus
			reason executor.ReasonCode
		}{status: status, reason: reason}
	}
	items := make([]DriftItem, 0, len(e.nodes))
	runtime := make([]ScheduleRuntimeItem, 0)
	inCompliance := true
	for _, n := range e.nodes {
		check := checks[n.Address]
		preflightStatus, preflightReason := preflights[n.Address].status, preflights[n.Address].reason
		if check.Status != executor.Compliant {
			inCompliance = false
		}
		items = append(items, DriftItem{
			Address:             n.Address,
			Name:                n.Name,
			Description:         string(n.Kind),
			Provider:            n.Provider,
			ProviderRevision:    n.ProviderRevision,
			EffectiveHash:       n.EffectiveHash,
			Status:              check.Status,
			ReasonCode:          check.ReasonCode,
			PreflightStatus:     preflightStatus,
			PreflightReason:     preflightReason,
			DesiredSummary:      n.DesiredSummary.Clone(),
			ObservedSummary:     safeHealthSummary(check.Status, check.ReasonCode),
			Subresults:          safeCheckSubresults(check.Subresults),
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

func (e *Engine) planPreflight(ctx context.Context, n node, check executor.CheckResult) (PreflightStatus, executor.ReasonCode) {
	requiresRollback := n.RollbackClass == executor.RollbackTransactional || n.RollbackClass == executor.RollbackBestEffort
	if !n.Risk.RequiresPreflight() && !requiresRollback {
		return PreflightNotRequired, ""
	}
	if check.Status != executor.Compliant && check.Status != executor.Drifted {
		return PreflightBlocked, check.ReasonCode
	}
	if n.Risk.RequiresPreflight() {
		if err := e.runPreflight(ctx, n); err != nil {
			return PreflightBlocked, executor.ReasonPreflightFailed
		}
	}
	if requiresRollback {
		preflighter, ok := n.Handler.(executor.RollbackPreflighter)
		if !ok || preflighter.PreflightRollback(ctx) != nil {
			return PreflightBlocked, executor.ReasonRollbackReservationFailed
		}
	}
	return PreflightReady, executor.ReasonPreflightReady
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
	if failure := e.verifyHighRiskHashes(checks); failure != nil {
		result.Failed = failure
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
			result.Failed = safeApplyFailure(n.Address, "acquire_operation_locks", "lock_acquisition_failed", err, nil)
			return result
		}
		if err := e.runPreflight(ctx, n); err != nil {
			releaseLocks()
			result.Failed = safeApplyFailure(n.Address, "preflight", "preflight_failed", err, nil)
			return result
		}
		if err := e.refreshAPTMetadata(ctx, n, applied); err != nil {
			releaseLocks()
			result.Failed = safeApplyFailure(n.Address, "refresh_package_metadata", "package_metadata_failed", err, nil)
			return result
		}
		applyResult := e.executor.ApplyState(ctx, n.Handler)
		releaseLocks()
		result.Items = append(result.Items, applyItem(n, check, applyResult))
		switch applyResult.Status {
		case executor.Failed:
			result.Failed = safeApplyFailure(n.Address, "provider_apply", "apply_failed", applyResult.Err, applyResult.Rollback)
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
			result.Failed = safeApplyFailure("activation", "activate", "activation_failed", err, nil)
		}
	}
	return result
}

func (e *Engine) validateExecutionLeases() error {
	for index, lease := range e.executionLeases {
		if lease.HashContractVersion != effectivehash.SchemaVersion || strings.TrimSpace(lease.ID) == "" || strings.TrimSpace(lease.ChangeRequestID) == "" || strings.TrimSpace(lease.EndpointID) == "" || len(lease.ResourceHashes) == 0 || lease.IssuedAt.IsZero() || lease.ExpiresAt.IsZero() || !lease.IssuedAt.Before(lease.ExpiresAt) {
			return fmt.Errorf("execution lease %d has invalid identity, bounds, or resource hashes", index)
		}
		for address, hash := range lease.ResourceHashes {
			if strings.TrimSpace(address) == "" || address != strings.TrimSpace(address) {
				return fmt.Errorf("execution lease %q has invalid resource address", lease.ID)
			}
			if err := effectivehash.Validate(hash); err != nil {
				return fmt.Errorf("execution lease %q resource %q: %w", lease.ID, address, err)
			}
		}
	}
	return nil
}

func (e *Engine) verifyHighRiskHashes(checks map[string]executor.CheckResult) *ApplyFailure {
	for _, n := range e.nodes {
		if checks[n.Address].Status != executor.Drifted || !n.Risk.RequiresPreflight() || n.Enforce == nil || !*n.Enforce {
			continue
		}
		if err := effectivehash.Validate(n.EffectiveHash); err != nil {
			return safeApplyFailure(n.Address, "verify_effective_hash", "hash_mismatch", err, nil)
		}
		matchedAddress := false
		for _, lease := range e.executionLeases {
			now := e.now().UTC()
			if lease.Completed || now.Before(lease.IssuedAt) || !now.Before(lease.ExpiresAt) {
				continue
			}
			leasedHash, ok := lease.ResourceHashes[n.Address]
			if !ok {
				continue
			}
			matchedAddress = true
			if leasedHash == n.EffectiveHash {
				matchedAddress = false
				break
			}
		}
		if matchedAddress {
			return safeApplyFailure(n.Address, "verify_effective_hash", "hash_mismatch", fmt.Errorf("execution lease hash differs from effective desired state"), nil)
		}
		if !e.activeLeaseAuthorizes(n.Address, n.EffectiveHash) {
			return safeApplyFailure(n.Address, "verify_effective_hash", "authorization_required", fmt.Errorf("no active execution lease authorizes the effective desired state"), nil)
		}
	}
	return nil
}

func (e *Engine) activeLeaseAuthorizes(address, hash string) bool {
	now := e.now().UTC()
	for _, lease := range e.executionLeases {
		if lease.Completed || now.Before(lease.IssuedAt) || !now.Before(lease.ExpiresAt) {
			continue
		}
		if lease.ResourceHashes[address] == hash {
			return true
		}
	}
	return false
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
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
		Address:          n.Address,
		Name:             n.Name,
		Provider:         n.Provider,
		ProviderRevision: n.ProviderRevision,
		EffectiveHash:    n.EffectiveHash,
		Status:           result.Status,
		ReasonCode:       applyReasonCode(result),
		DesiredSummary:   n.DesiredSummary.Clone(),
		ObservedSummary:  safeHealthSummary(check.Status, check.ReasonCode),
		Activation:       append([]executor.ActivationSignal(nil), result.Activation...),
		RebootRequired:   result.RebootRequired,
		RollbackClass:    result.RollbackClass,
		Diagnostics:      []executor.SafeSummary{safeHealthSummary(executor.CheckStatus(result.Status), applyReasonCode(result))},
	}
	if result.Rollback != nil {
		item.RollbackStatus = result.Rollback.Status
	}
	return item
}

func safeHealthSummary(status executor.CheckStatus, reason executor.ReasonCode) executor.SafeSummary {
	summary, err := executor.NewSafeSummary([]executor.SafeField{
		{Path: "status", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: string(status)},
		{Path: "reasonCode", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: string(reason)},
	})
	if err != nil {
		panic(err)
	}
	return summary
}

func safeCheckSubresults(results []executor.CheckSubresult) []CheckSubresult {
	if len(results) == 0 {
		return nil
	}
	out := make([]CheckSubresult, len(results))
	for i, result := range results {
		out[i] = CheckSubresult{
			Target: result.Target, Status: result.Status, ReasonCode: result.ReasonCode,
			ObservedSummary: safeHealthSummary(result.Status, result.ReasonCode),
		}
	}
	return out
}

func safeApplyFailure(address, operation string, reason executor.ReasonCode, err error, rollback *executor.RollbackResult) *ApplyFailure {
	safeError := executor.NewSafeError(reason, operation, err)
	var actionError *ServiceActionError
	if errors.As(err, &actionError) {
		details, detailErr := executor.NewSafeSummary([]executor.SafeField{
			{Path: "provider", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: actionError.Provider},
			{Path: "unit", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: actionError.Unit},
			{Path: "operation", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: actionError.Operation},
			{Path: "exitStatus", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: fmt.Sprintf("%d", actionError.ExitStatus)},
		})
		if detailErr == nil {
			safeError = executor.NewSafeErrorWithDetails(reason, operation, err, details)
		}
	}
	failure := &ApplyFailure{Address: address, Err: safeError}
	if rollback != nil {
		failure.RollbackStatus = rollback.Status
	}
	return failure
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
