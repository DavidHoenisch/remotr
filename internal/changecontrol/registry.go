// Package changecontrol owns the server-side review and rollout state for
// high-risk desired-state changes.
package changecontrol

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/google/uuid"
)

type AuthorizationState string

const (
	AuthorizationPending         AuthorizationState = "pending"
	AuthorizationApprovalPending AuthorizationState = AuthorizationPending
	AuthorizationActive          AuthorizationState = "authorized"
	AuthorizationPaused          AuthorizationState = "paused"
	AuthorizationRevoked         AuthorizationState = "revoked"
)

const (
	LegacyEnforcementNonEnforcing         = "non_enforcing"
	LegacyReplacementExplicitRegeneration = "explicit_regeneration_required"
	LegacyReasonNoCanonicalHashContract   = "legacy_plan_has_no_canonical_hash_contract_version"
)

// LegacyAuthorizationMigration keeps restored caller-authored authorization
// visible without treating it as authority under the canonical hash contract.
type LegacyAuthorizationMigration struct {
	Enforcement string `json:"enforcement"`
	Replacement string `json:"replacement"`
	Reason      string `json:"reason"`
}

func newLegacyMigration() *LegacyAuthorizationMigration {
	return &LegacyAuthorizationMigration{
		Enforcement: LegacyEnforcementNonEnforcing,
		Replacement: LegacyReplacementExplicitRegeneration,
		Reason:      LegacyReasonNoCanonicalHashContract,
	}
}

func cloneLegacyMigration(input *LegacyAuthorizationMigration) *LegacyAuthorizationMigration {
	if input == nil {
		return nil
	}
	copy := *input
	return &copy
}

type AuditAction string

const (
	AuditCreated                AuditAction = "created"
	AuditRolloutAuthorized      AuditAction = "rollout_authorized"
	AuditBaselinePromoted       AuditAction = "baseline_promoted"
	AuditBaselineInvalidated    AuditAction = "baseline_invalidated"
	AuditPaused                 AuditAction = "paused"
	AuditResumed                AuditAction = "resumed"
	AuditRevoked                AuditAction = "revoked"
	AuditBaselineAdoption       AuditAction = "baseline_adoption_created"
	AuditTargetOutcome          AuditAction = "target_outcome"
	AuditExceptionsAcknowledged AuditAction = "exceptions_acknowledged"
	AuditBreakGlassCreated      AuditAction = "break_glass_created"
	AuditBreakGlassUsed         AuditAction = "break_glass_used"
	AuditBreakGlassExpired      AuditAction = "break_glass_expired"
	AuditBreakGlassRevoked      AuditAction = "break_glass_revoked"
)

type AuditEntry struct {
	At      time.Time   `json:"at"`
	ActorID string      `json:"actor_id"`
	Action  AuditAction `json:"action"`
	Details string      `json:"details,omitempty"`
}

// TargetEvidence is one evaluated endpoint frozen into a Change request.
type TargetEvidence struct {
	EndpointID         string                      `json:"endpoint_id"`
	Compatible         bool                        `json:"compatible"`
	PreflightReady     bool                        `json:"preflight_ready"`
	PreflightReason    string                      `json:"preflight_reason,omitempty"`
	ResourcePreflights []ResourcePreflightEvidence `json:"resource_preflights,omitempty"`
}

// ResourcePreflightEvidence is the closed endpoint readiness result for one
// high-risk resource after its normal dependency evidence is propagated.
type ResourcePreflightEvidence struct {
	Address string `json:"address"`
	Ready   bool   `json:"ready"`
	Reason  string `json:"reason,omitempty"`
}

// ResourcePlan is one resource in the server's non-enforcing review plan.
type ResourcePlan struct {
	Address            string            `json:"address"`
	DesiredHash        string            `json:"desired_hash"`
	Risk               models.RiskClass  `json:"risk"`
	Provider           string            `json:"provider"`
	ProviderRevision   string            `json:"provider_revision,omitempty"`
	AuthorizationGroup string            `json:"authorization_group,omitempty"`
	DependsOn          []string          `json:"depends_on,omitempty"`
	ActivationTargets  []string          `json:"activation_targets,omitempty"`
	PredictedEffects   []PredictedEffect `json:"predicted_effects,omitempty"`
	RollbackClass      string            `json:"rollback_class"`
	BaselineEligible   bool              `json:"baseline_eligible"`
}

// FleetPlan is the complete pre-authorization evidence for one fleet and
// Release ref. A single call can produce several independent Change requests.
type FleetPlan struct {
	Fleet               string           `json:"fleet"`
	ReleaseRef          string           `json:"release_ref"`
	ArtifactDigest      string           `json:"artifact_digest"`
	HashContractVersion int              `json:"hash_contract_version,omitempty"`
	Targets             []TargetEvidence `json:"targets"`
	Resources           []ResourcePlan   `json:"resources"`
}

// CanonicalResourceIdentity is trusted composition evidence presented at the
// Change-request boundary. It is not accepted from Admin API request bodies.
type CanonicalResourceIdentity struct {
	Address             string
	EffectiveHash       string
	Provider            string
	ProviderRevision    string
	HashContractVersion int
}

// ChangeRequest is immutable review evidence plus authorization lifecycle
// state. FrozenTargets and ResourceHashes never expand after creation.
type ChangeRequest struct {
	ID                  string                        `json:"id"`
	Fleet               string                        `json:"fleet"`
	ReleaseRef          string                        `json:"release_ref"`
	ArtifactDigest      string                        `json:"artifact_digest"`
	HashContractVersion int                           `json:"hash_contract_version,omitempty"`
	AuthorizationGroup  string                        `json:"authorization_group"`
	Risk                models.RiskClass              `json:"risk"`
	Resources           []ResourcePlan                `json:"resources"`
	ResourceHashes      map[string]string             `json:"resource_hashes"`
	FrozenTargets       []TargetEvidence              `json:"frozen_targets"`
	AuthorizationState  AuthorizationState            `json:"authorization_state"`
	RequiredApprovals   int                           `json:"required_approvals"`
	Approvals           []Approval                    `json:"approvals,omitempty"`
	PolicyWarning       string                        `json:"policy_warning,omitempty"`
	Outcomes            map[string]TargetOutcome      `json:"outcomes,omitempty"`
	AuditHistory        []AuditEntry                  `json:"audit_history"`
	CreatedAt           time.Time                     `json:"created_at"`
	LegacyMigration     *LegacyAuthorizationMigration `json:"legacy_migration,omitempty"`
}

type RegistryOptions struct {
	Now           func() time.Time
	NewID         func() string
	CanApprove    func(actorID, fleet string, risk models.RiskClass) bool
	CanBreakGlass func(actorID, fleet string, risk models.RiskClass) bool
	Policy        ApprovalPolicy
}

// Registry stores immutable Change requests and their later lifecycle state.
type Registry struct {
	mu                 sync.RWMutex
	storeContext       context.Context
	stateStore         StateStore
	stateRevision      int64
	now                func() time.Time
	newID              func() string
	requests           map[string]ChangeRequest
	rollouts           map[string]RolloutAuthorization
	baselines          map[string]BaselineAuthorization
	canApprove         func(string, string, models.RiskClass) bool
	canBreakGlass      func(string, string, models.RiskClass) bool
	policy             ApprovalPolicy
	automaticPromotion map[string]AutomaticPromotionPolicy
	leases             map[string]ExecutionLease
	attempts           map[string]int
	breakGlass         map[string]BreakGlassAuthorization
}

func NewRegistry(options RegistryOptions) *Registry {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := options.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	canApprove := options.CanApprove
	if canApprove == nil {
		canApprove = func(string, string, models.RiskClass) bool { return true }
	}
	canBreakGlass := options.CanBreakGlass
	if canBreakGlass == nil {
		canBreakGlass = func(string, string, models.RiskClass) bool { return false }
	}
	return &Registry{
		now: now, newID: newID,
		requests:   make(map[string]ChangeRequest),
		rollouts:   make(map[string]RolloutAuthorization),
		baselines:  make(map[string]BaselineAuthorization),
		canApprove: canApprove, canBreakGlass: canBreakGlass, policy: cloneApprovalPolicy(options.Policy),
		automaticPromotion: make(map[string]AutomaticPromotionPolicy),
		leases:             make(map[string]ExecutionLease), attempts: make(map[string]int), breakGlass: make(map[string]BreakGlassAuthorization),
	}
}

// CreateChangeRequests groups high-risk resources without crossing the Fleet
// boundary, includes their normal prerequisites, freezes target evidence, and
// records the creation audit event.
func (r *Registry) CreateChangeRequests(plan FleetPlan, actorID string) ([]ChangeRequest, error) {
	if plan.HashContractVersion != 0 {
		return nil, fmt.Errorf("canonical Change requests require trusted composition verification")
	}
	return r.createChangeRequests(plan, actorID, "")
}

// CreateCanonicalChangeRequests admits a versioned plan only after every
// caller-visible identity exactly matches trusted composition evidence.
func (r *Registry) CreateCanonicalChangeRequests(plan FleetPlan, trusted []CanonicalResourceIdentity, actorID string) ([]ChangeRequest, error) {
	if err := verifyCanonicalPlan(plan, trusted); err != nil {
		return nil, err
	}
	return r.createChangeRequests(plan, actorID, "")
}

func verifyCanonicalPlan(plan FleetPlan, trusted []CanonicalResourceIdentity) error {
	if plan.HashContractVersion != effectivehash.SchemaVersion {
		return fmt.Errorf("canonical plan requires effective hash contract version %d", effectivehash.SchemaVersion)
	}
	identities := make(map[string]CanonicalResourceIdentity, len(trusted))
	for _, identity := range trusted {
		if identity.HashContractVersion != effectivehash.SchemaVersion || effectivehash.Validate(identity.EffectiveHash) != nil {
			return fmt.Errorf("trusted composition identity for %q is invalid", identity.Address)
		}
		if _, exists := identities[identity.Address]; exists {
			return fmt.Errorf("duplicate trusted composition identity %q", identity.Address)
		}
		identities[identity.Address] = identity
	}
	for _, resource := range plan.Resources {
		identity, ok := identities[resource.Address]
		if !ok || identity.EffectiveHash != resource.DesiredHash || identity.Provider != resource.Provider || identity.ProviderRevision != resource.ProviderRevision {
			return fmt.Errorf("resource %q does not match trusted canonical composition", resource.Address)
		}
		delete(identities, resource.Address)
	}
	if len(identities) != 0 {
		return fmt.Errorf("trusted canonical composition contains resources omitted from the plan")
	}
	return nil
}

func (r *Registry) createChangeRequests(plan FleetPlan, actorID string, additionalAudit AuditAction) ([]ChangeRequest, error) {
	if err := validateFleetPlan(plan); err != nil {
		return nil, err
	}
	groups, err := changeGroups(plan.Resources)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []ChangeRequest{}, nil
	}

	addresses := make(map[string]ResourcePlan, len(plan.Resources))
	for _, resource := range plan.Resources {
		addresses[resource.Address] = resource
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	now := r.now().UTC()
	requests := make([]ChangeRequest, 0, len(keys))
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.snapshotLocked()
	for _, key := range keys {
		included, err := dependencyClosure(groups[key], key, addresses)
		if err != nil {
			r.restoreLocked(previous)
			return nil, err
		}
		resources := make([]ResourcePlan, 0, len(included))
		hashes := make(map[string]string, len(included))
		strictest := models.RiskNormal
		for _, resource := range plan.Resources {
			if _, ok := included[resource.Address]; !ok {
				continue
			}
			frozen := cloneResource(resource)
			resources = append(resources, frozen)
			hashes[resource.Address] = resource.DesiredHash
			if riskRank(resource.Risk) > riskRank(strictest) {
				strictest = resource.Risk
			}
		}
		id := r.newID()
		if strings.TrimSpace(id) == "" {
			r.restoreLocked(previous)
			return nil, fmt.Errorf("change request id is required")
		}
		if _, exists := r.requests[id]; exists {
			r.restoreLocked(previous)
			return nil, fmt.Errorf("duplicate change request id %q", id)
		}
		auditHistory := []AuditEntry{{At: now, ActorID: actorID, Action: AuditCreated}}
		if additionalAudit != "" {
			auditHistory = append(auditHistory, AuditEntry{At: now, ActorID: actorID, Action: additionalAudit})
		}
		request := ChangeRequest{
			ID:                  id,
			Fleet:               plan.Fleet,
			ReleaseRef:          plan.ReleaseRef,
			ArtifactDigest:      plan.ArtifactDigest,
			HashContractVersion: plan.HashContractVersion,
			AuthorizationGroup:  key,
			Risk:                strictest,
			Resources:           resources,
			ResourceHashes:      hashes,
			FrozenTargets:       scopeTargetEvidence(plan.Targets, included, addresses),
			AuthorizationState:  AuthorizationPending,
			AuditHistory:        auditHistory,
			CreatedAt:           now,
		}
		request.RequiredApprovals = r.policy.threshold(request.Fleet, request.Risk)
		if request.Risk == models.RiskDestructive && request.RequiredApprovals == 1 {
			request.PolicyWarning = SingleOperatorDestructiveWarning
		}
		r.requests[id] = cloneRequest(request)
		requests = append(requests, cloneRequest(request))
	}
	if err := r.persistLocked(previous); err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *Registry) Get(id string) (ChangeRequest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	request, ok := r.requests[id]
	return cloneRequest(request), ok
}

func validateFleetPlan(plan FleetPlan) error {
	if strings.TrimSpace(plan.Fleet) == "" || strings.TrimSpace(plan.ReleaseRef) == "" || strings.TrimSpace(plan.ArtifactDigest) == "" {
		return fmt.Errorf("fleet, release ref, and artifact digest are required")
	}
	seenTargets := make(map[string]struct{}, len(plan.Targets))
	for _, target := range plan.Targets {
		if strings.TrimSpace(target.EndpointID) == "" {
			return fmt.Errorf("frozen target endpoint id is required")
		}
		if _, exists := seenTargets[target.EndpointID]; exists {
			return fmt.Errorf("duplicate frozen target %q", target.EndpointID)
		}
		seenPreflights := make(map[string]struct{}, len(target.ResourcePreflights))
		for _, preflight := range target.ResourcePreflights {
			if strings.TrimSpace(preflight.Address) == "" {
				return fmt.Errorf("target %q resource preflight address is required", target.EndpointID)
			}
			if _, exists := seenPreflights[preflight.Address]; exists {
				return fmt.Errorf("target %q has duplicate resource preflight %q", target.EndpointID, preflight.Address)
			}
			if preflight.Ready && preflight.Reason != "" {
				return fmt.Errorf("target %q ready resource preflight %q cannot have a reason", target.EndpointID, preflight.Address)
			}
			if !preflight.Ready && !validEvidenceReason(preflight.Reason) {
				return fmt.Errorf("target %q blocked resource preflight %q requires a stable reason", target.EndpointID, preflight.Address)
			}
			seenPreflights[preflight.Address] = struct{}{}
		}
		seenTargets[target.EndpointID] = struct{}{}
	}
	seenResources := make(map[string]struct{}, len(plan.Resources))
	if plan.HashContractVersion != 0 && plan.HashContractVersion != effectivehash.SchemaVersion {
		return fmt.Errorf("unsupported effective hash contract version %d", plan.HashContractVersion)
	}
	for _, resource := range plan.Resources {
		if strings.TrimSpace(resource.Address) == "" || strings.TrimSpace(resource.DesiredHash) == "" {
			return fmt.Errorf("resource address and desired hash are required")
		}
		if !resource.Risk.Valid() {
			return fmt.Errorf("resource %q has invalid risk %q", resource.Address, resource.Risk)
		}
		if plan.HashContractVersion == effectivehash.SchemaVersion {
			if strings.TrimSpace(resource.Provider) == "" || strings.TrimSpace(resource.ProviderRevision) == "" {
				return fmt.Errorf("resource %q requires provider identity and revision", resource.Address)
			}
			if err := effectivehash.Validate(resource.DesiredHash); err != nil {
				return fmt.Errorf("resource %q: %w", resource.Address, err)
			}
		}
		if _, exists := seenResources[resource.Address]; exists {
			return fmt.Errorf("duplicate resource %q", resource.Address)
		}
		for index, effect := range resource.PredictedEffects {
			if err := effect.Validate(); err != nil {
				return fmt.Errorf("resource %q predicted effect %d: %w", resource.Address, index+1, err)
			}
		}
		seenResources[resource.Address] = struct{}{}
	}
	for _, resource := range plan.Resources {
		for _, dependency := range resource.DependsOn {
			if _, exists := seenResources[dependency]; !exists {
				return fmt.Errorf("resource %q depends on unknown resource %q", resource.Address, dependency)
			}
		}
	}
	for _, target := range plan.Targets {
		for _, preflight := range target.ResourcePreflights {
			_, exists := seenResources[preflight.Address]
			if !exists {
				return fmt.Errorf("target %q preflight references unknown resource %q", target.EndpointID, preflight.Address)
			}
		}
	}
	return nil
}

func scopeTargetEvidence(targets []TargetEvidence, included map[string]struct{}, resources map[string]ResourcePlan) []TargetEvidence {
	output := make([]TargetEvidence, len(targets))
	for index, target := range targets {
		output[index] = cloneTargetEvidence(target)
		if len(target.ResourcePreflights) == 0 {
			continue
		}
		output[index].ResourcePreflights = nil
		if !target.Compatible {
			continue
		}
		byAddress := make(map[string]ResourcePreflightEvidence, len(target.ResourcePreflights))
		for _, evidence := range target.ResourcePreflights {
			byAddress[evidence.Address] = evidence
		}
		output[index].PreflightReady = true
		output[index].PreflightReason = ""
		addresses := make([]string, 0, len(included))
		for address := range included {
			addresses = append(addresses, address)
		}
		sort.Strings(addresses)
		for _, address := range addresses {
			if !resources[address].Risk.RequiresPreflight() {
				continue
			}
			evidence, ok := byAddress[address]
			if !ok {
				evidence = ResourcePreflightEvidence{Address: address, Reason: "preflight_evidence_missing"}
			}
			output[index].ResourcePreflights = append(output[index].ResourcePreflights, evidence)
			if !evidence.Ready && output[index].PreflightReady {
				output[index].PreflightReady = false
				output[index].PreflightReason = evidence.Reason
			}
		}
	}
	return output
}

func validEvidenceReason(reason string) bool {
	if len(reason) == 0 || len(reason) > 64 || reason[0] < 'a' || reason[0] > 'z' {
		return false
	}
	for _, character := range reason {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func changeGroups(resources []ResourcePlan) (map[string]map[string]struct{}, error) {
	groups := make(map[string]map[string]struct{})
	ungrouped := make(map[string]ResourcePlan)
	for _, resource := range resources {
		if !resource.Risk.RequiresPreflight() {
			continue
		}
		if resource.AuthorizationGroup != "" {
			key := resource.AuthorizationGroup
			if groups[key] == nil {
				groups[key] = make(map[string]struct{})
			}
			groups[key][resource.Address] = struct{}{}
			continue
		}
		ungrouped[resource.Address] = resource
	}

	visited := make(map[string]bool, len(ungrouped))
	addresses := make([]string, 0, len(ungrouped))
	for address := range ungrouped {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	for _, address := range addresses {
		if visited[address] {
			continue
		}
		component := connectedHighRiskComponent(address, ungrouped, visited)
		key := "component:" + component[0]
		groups[key] = make(map[string]struct{}, len(component))
		for _, member := range component {
			groups[key][member] = struct{}{}
		}
	}
	return groups, nil
}

func connectedHighRiskComponent(start string, resources map[string]ResourcePlan, visited map[string]bool) []string {
	queue := []string{start}
	var component []string
	for len(queue) > 0 {
		address := queue[0]
		queue = queue[1:]
		if visited[address] {
			continue
		}
		visited[address] = true
		component = append(component, address)
		for candidate, resource := range resources {
			if visited[candidate] {
				continue
			}
			if directlyConnected(address, resources[address], candidate, resource) {
				queue = append(queue, candidate)
			}
		}
	}
	sort.Strings(component)
	return component
}

func directlyConnected(a string, ar ResourcePlan, b string, br ResourcePlan) bool {
	if contains(ar.DependsOn, b) || contains(br.DependsOn, a) {
		return true
	}
	for _, activation := range ar.ActivationTargets {
		if contains(br.ActivationTargets, activation) {
			return true
		}
	}
	return false
}

func dependencyClosure(roots map[string]struct{}, group string, resources map[string]ResourcePlan) (map[string]struct{}, error) {
	included := make(map[string]struct{}, len(roots))
	queue := make([]string, 0, len(roots))
	for address := range roots {
		queue = append(queue, address)
	}
	for len(queue) > 0 {
		address := queue[0]
		queue = queue[1:]
		if _, exists := included[address]; exists {
			continue
		}
		resource := resources[address]
		if resource.Risk.RequiresPreflight() && resource.AuthorizationGroup != "" && resource.AuthorizationGroup != group {
			return nil, fmt.Errorf("resource %q crosses authorization groups %q and %q", address, resource.AuthorizationGroup, group)
		}
		included[address] = struct{}{}
		queue = append(queue, resource.DependsOn...)
	}
	return included, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func riskRank(risk models.RiskClass) int {
	switch risk {
	case models.RiskDestructive:
		return 6
	case models.RiskBoot:
		return 5
	case models.RiskAccess:
		return 4
	case models.RiskConnectivity:
		return 3
	case models.RiskSensitive:
		return 2
	default:
		return 1
	}
}

func cloneRequest(request ChangeRequest) ChangeRequest {
	resources := request.Resources
	request.Resources = make([]ResourcePlan, len(resources))
	for i, resource := range resources {
		request.Resources[i] = cloneResource(resource)
	}
	hashes := request.ResourceHashes
	request.ResourceHashes = make(map[string]string, len(hashes))
	for address, hash := range hashes {
		request.ResourceHashes[address] = hash
	}
	request.FrozenTargets = cloneTargetEvidenceList(request.FrozenTargets)
	request.AuditHistory = append([]AuditEntry(nil), request.AuditHistory...)
	request.Approvals = append([]Approval(nil), request.Approvals...)
	request.Outcomes = cloneOutcomes(request.Outcomes)
	request.LegacyMigration = cloneLegacyMigration(request.LegacyMigration)
	return request
}

func cloneTargetEvidenceList(targets []TargetEvidence) []TargetEvidence {
	output := make([]TargetEvidence, len(targets))
	for index, target := range targets {
		output[index] = cloneTargetEvidence(target)
	}
	return output
}

func cloneTargetEvidence(target TargetEvidence) TargetEvidence {
	target.ResourcePreflights = append([]ResourcePreflightEvidence(nil), target.ResourcePreflights...)
	return target
}

func cloneResource(resource ResourcePlan) ResourcePlan {
	resource.DependsOn = append([]string(nil), resource.DependsOn...)
	resource.ActivationTargets = append([]string(nil), resource.ActivationTargets...)
	resource.PredictedEffects = clonePredictedEffects(resource.PredictedEffects)
	return resource
}
