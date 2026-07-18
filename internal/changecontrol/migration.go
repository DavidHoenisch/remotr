package changecontrol

import (
	"fmt"
	"sort"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
)

const (
	LegacyResourceUnchanged = "unchanged"
	LegacyResourceChanged   = "changed"
	LegacyResourceMissing   = "missing_in_canonical"
	LegacyResourceAdded     = "added_in_canonical"
)

// LegacyPlanComparison is a safe, bounded comparison between the visible
// legacy request and a separately created canonical replacement request.
type LegacyPlanComparison struct {
	CanonicalReleaseRef     string                     `json:"canonical_release_ref"`
	CanonicalArtifactDigest string                     `json:"canonical_artifact_digest"`
	ReleaseRefMatches       bool                       `json:"release_ref_matches"`
	ArtifactDigestMatches   bool                       `json:"artifact_digest_matches"`
	Resources               []LegacyResourceComparison `json:"resources"`
}

// LegacyResourceComparison never reclassifies or copies legacy free-form
// effects. The old request remains the visible record of its authored hash.
type LegacyResourceComparison struct {
	Address                   string `json:"address"`
	Status                    string `json:"status"`
	CanonicalHash             string `json:"canonical_hash,omitempty"`
	CanonicalProvider         string `json:"canonical_provider,omitempty"`
	CanonicalProviderRevision string `json:"canonical_provider_revision,omitempty"`
}

type LegacyRegenerationResult struct {
	LegacyRequest      ChangeRequest        `json:"legacy_request"`
	ReplacementRequest ChangeRequest        `json:"replacement_request"`
	Comparison         LegacyPlanComparison `json:"comparison"`
}

// RegenerateLegacyBaselineAdoption atomically records a visible comparison
// and creates a distinct pending canonical request. It never rewrites the
// legacy request, rollout, baseline, approvals, or caller-authored hashes.
func (r *Registry) RegenerateLegacyBaselineAdoption(legacyRequestID string, plan FleetPlan, trusted []CanonicalResourceIdentity, actorID string) (LegacyRegenerationResult, error) {
	if err := verifyCanonicalPlan(plan, trusted); err != nil {
		return LegacyRegenerationResult{}, err
	}
	for index := range plan.Resources {
		if plan.Resources[index].Risk.RequiresPreflight() {
			plan.Resources[index].AuthorizationGroup = "baseline-adoption"
		}
	}
	if err := validateFleetPlan(plan); err != nil {
		return LegacyRegenerationResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	legacy, ok := r.requests[legacyRequestID]
	if !ok {
		return LegacyRegenerationResult{}, fmt.Errorf("change request %q not found", legacyRequestID)
	}
	if legacy.HashContractVersion == effectivehash.SchemaVersion || legacy.LegacyMigration == nil {
		return LegacyRegenerationResult{}, fmt.Errorf("change request %q is already canonical", legacyRequestID)
	}
	if legacy.LegacyMigration.ReplacementChangeRequestID != "" {
		return LegacyRegenerationResult{}, fmt.Errorf("legacy Change request %q already has an explicit replacement", legacyRequestID)
	}
	if legacy.Fleet != plan.Fleet {
		return LegacyRegenerationResult{}, fmt.Errorf("canonical replacement Fleet does not match legacy Change request")
	}

	previous := r.snapshotLocked()
	replacements, err := r.createChangeRequestsLocked(plan, actorID, AuditBaselineAdoption)
	if err != nil {
		r.restoreLocked(previous)
		return LegacyRegenerationResult{}, err
	}
	if len(replacements) != 1 {
		r.restoreLocked(previous)
		return LegacyRegenerationResult{}, fmt.Errorf("canonical regeneration requires at least one high-risk resource")
	}
	replacement := replacements[0]
	comparison := compareLegacyPlan(legacy, replacement)
	migration := &LegacyAuthorizationMigration{
		Enforcement:                LegacyEnforcementNonEnforcing,
		Replacement:                LegacyReplacementRegenerated,
		Reason:                     LegacyReasonNoCanonicalHashContract,
		ReplacementChangeRequestID: replacement.ID,
		Comparison:                 &comparison,
	}
	now := r.now().UTC()
	legacy.LegacyMigration = cloneLegacyMigration(migration)
	legacy.AuditHistory = append(legacy.AuditHistory, AuditEntry{At: now, ActorID: actorID, Action: AuditLegacyRegenerated, Details: replacement.ID})
	r.requests[legacy.ID] = cloneRequest(legacy)
	replacement.AuditHistory = append(replacement.AuditHistory, AuditEntry{At: now, ActorID: actorID, Action: AuditLegacyRegenerated, Details: legacy.ID})
	r.requests[replacement.ID] = cloneRequest(replacement)
	if rollout, exists := r.rollouts[legacy.ID]; exists {
		rollout.LegacyMigration = cloneLegacyMigration(migration)
		r.rollouts[legacy.ID] = cloneRollout(rollout)
	}
	for key, baseline := range r.baselines {
		if baseline.ChangeRequestID == legacy.ID {
			baseline.LegacyMigration = cloneLegacyMigration(migration)
			r.baselines[key] = cloneBaseline(baseline)
		}
	}
	if err := r.persistLocked(previous); err != nil {
		return LegacyRegenerationResult{}, err
	}
	return LegacyRegenerationResult{
		LegacyRequest:      cloneRequest(legacy),
		ReplacementRequest: cloneRequest(replacement),
		Comparison:         *cloneLegacyPlanComparison(&comparison),
	}, nil
}

func compareLegacyPlan(legacy, canonical ChangeRequest) LegacyPlanComparison {
	comparison := LegacyPlanComparison{
		CanonicalReleaseRef:     canonical.ReleaseRef,
		CanonicalArtifactDigest: canonical.ArtifactDigest,
		ReleaseRefMatches:       legacy.ReleaseRef == canonical.ReleaseRef,
		ArtifactDigestMatches:   legacy.ArtifactDigest == canonical.ArtifactDigest,
	}
	legacyByAddress := make(map[string]ResourcePlan, len(legacy.Resources))
	canonicalByAddress := make(map[string]ResourcePlan, len(canonical.Resources))
	addresses := make(map[string]struct{}, len(legacy.Resources)+len(canonical.Resources))
	for _, resource := range legacy.Resources {
		legacyByAddress[resource.Address] = resource
		addresses[resource.Address] = struct{}{}
	}
	for _, resource := range canonical.Resources {
		canonicalByAddress[resource.Address] = resource
		addresses[resource.Address] = struct{}{}
	}
	ordered := make([]string, 0, len(addresses))
	for address := range addresses {
		ordered = append(ordered, address)
	}
	sort.Strings(ordered)
	for _, address := range ordered {
		oldResource, hadLegacy := legacyByAddress[address]
		current, hasCanonical := canonicalByAddress[address]
		item := LegacyResourceComparison{Address: address}
		switch {
		case !hasCanonical:
			item.Status = LegacyResourceMissing
		case !hadLegacy:
			item.Status = LegacyResourceAdded
		default:
			item.Status = LegacyResourceChanged
			if oldResource.DesiredHash == current.DesiredHash && oldResource.Provider == current.Provider && oldResource.ProviderRevision == current.ProviderRevision {
				item.Status = LegacyResourceUnchanged
			}
		}
		if hasCanonical {
			item.CanonicalHash = current.DesiredHash
			item.CanonicalProvider = current.Provider
			item.CanonicalProviderRevision = current.ProviderRevision
		}
		comparison.Resources = append(comparison.Resources, item)
	}
	return comparison
}

func cloneLegacyPlanComparison(input *LegacyPlanComparison) *LegacyPlanComparison {
	if input == nil {
		return nil
	}
	copy := *input
	copy.Resources = append([]LegacyResourceComparison(nil), input.Resources...)
	return &copy
}

func validateLegacyMigration(migration *LegacyAuthorizationMigration) error {
	if migration == nil || migration.Enforcement != LegacyEnforcementNonEnforcing || migration.Reason != LegacyReasonNoCanonicalHashContract {
		return fmt.Errorf("legacy authorization migration must remain visibly non-enforcing")
	}
	switch migration.Replacement {
	case LegacyReplacementExplicitRegeneration:
		if migration.ReplacementChangeRequestID != "" || migration.Comparison != nil {
			return fmt.Errorf("legacy authorization awaiting regeneration cannot name a replacement")
		}
	case LegacyReplacementRegenerated:
		if migration.ReplacementChangeRequestID == "" || migration.Comparison == nil {
			return fmt.Errorf("regenerated legacy authorization requires a replacement and comparison")
		}
	default:
		return fmt.Errorf("legacy authorization has unknown replacement state")
	}
	return nil
}

func equalLegacyMigration(left, right *LegacyAuthorizationMigration) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Enforcement == right.Enforcement && left.Replacement == right.Replacement && left.Reason == right.Reason &&
		left.ReplacementChangeRequestID == right.ReplacementChangeRequestID && equalLegacyPlanComparison(left.Comparison, right.Comparison)
}

func equalLegacyPlanComparison(left, right *LegacyPlanComparison) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.CanonicalReleaseRef != right.CanonicalReleaseRef || left.CanonicalArtifactDigest != right.CanonicalArtifactDigest ||
		left.ReleaseRefMatches != right.ReleaseRefMatches || left.ArtifactDigestMatches != right.ArtifactDigestMatches || len(left.Resources) != len(right.Resources) {
		return false
	}
	for index := range left.Resources {
		if left.Resources[index] != right.Resources[index] {
			return false
		}
	}
	return true
}
