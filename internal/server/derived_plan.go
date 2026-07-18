package server

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

func (s *Server) deriveBaselineAdoptionPlan(ctx context.Context, fleet string) (configcompose.DerivedFleetPlan, error) {
	if s.cfg.StateReports == nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("server-derived Change plan endpoint evidence is unavailable")
	}
	releaseRef := s.releaseRef(ctx)
	artifact, digest, err := resolveFleetDesiredArtifact(ctx, s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, fleet, releaseRef)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("resolve composed Fleet artifact: %w", err)
	}
	state, err := models.ParseState(bytes.NewReader(artifact))
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("parse composed Fleet artifact: %w", err)
	}
	reports, err := s.cfg.StateReports.ListFleetStateReports(ctx, fleet)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("read current Fleet endpoint evidence: %w", err)
	}
	return derivePlanFromEndpointEvidence(ctx, fleet, releaseRef, digest, state, reports.Endpoints, s.cfg.Secrets)
}

type providerSelectionCandidate struct {
	key        string
	selections map[string]configcompose.ProviderSelection
	reports    []registry.StateReport
}

func derivePlanFromEndpointEvidence(ctx context.Context, fleet, releaseRef, artifactDigest string, state models.State, reports []registry.StateReport, resolver secrets.Resolver) (configcompose.DerivedFleetPlan, error) {
	addresses, err := composedResourceAddresses(state)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, err
	}
	candidates := providerSelectionCandidates(reports, releaseRef, artifactDigest, addresses)
	for _, candidate := range candidates {
		derived, err := configcompose.DeriveFleetPlan(ctx, fleet, releaseRef, artifactDigest, state, candidate.selections, resolver)
		if err != nil {
			continue
		}
		matched := false
		for _, report := range candidate.reports {
			if reportMatchesCanonicalPlan(report, derived.TrustedIdentities) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		derived.Plan.Targets = endpointTargetEvidence(reports, releaseRef, artifactDigest, derived)
		return derived, nil
	}
	return configcompose.DerivedFleetPlan{}, fmt.Errorf("no current endpoint evidence matches canonical Fleet composition")
}

func composedResourceAddresses(state models.State) ([]string, error) {
	registered, err := resourceregistry.NewDefault()
	if err != nil {
		return nil, err
	}
	var addresses []string
	for index := range state.Configurations {
		configuration := &state.Configurations[index]
		resources, err := registered.Resources(configuration)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			addresses = append(addresses, models.ResourceAddress(configuration.Name, resource.Name()))
		}
	}
	sort.Strings(addresses)
	return addresses, nil
}

func providerSelectionCandidates(reports []registry.StateReport, releaseRef, artifactDigest string, addresses []string) []providerSelectionCandidate {
	groups := make(map[string]*providerSelectionCandidate)
	for _, report := range reports {
		if !report.HasReport() || report.SchemaVersion < 9 || report.ReleaseRef != releaseRef || report.Digest != artifactDigest {
			continue
		}
		items, ok := reportItemsByAddress(report, addresses)
		if !ok {
			continue
		}
		selections := make(map[string]configcompose.ProviderSelection, len(addresses))
		var key strings.Builder
		for _, address := range addresses {
			provider := items[address].Provider
			selections[address] = configcompose.ProviderSelection{ID: provider}
			key.WriteString(strconv.Itoa(len(address)))
			key.WriteByte(':')
			key.WriteString(address)
			key.WriteByte('=')
			key.WriteString(strconv.Itoa(len(provider)))
			key.WriteByte(':')
			key.WriteString(provider)
			key.WriteByte(';')
		}
		groupKey := key.String()
		candidate := groups[groupKey]
		if candidate == nil {
			candidate = &providerSelectionCandidate{key: groupKey, selections: selections}
			groups[groupKey] = candidate
		}
		candidate.reports = append(candidate.reports, report)
	}
	output := make([]providerSelectionCandidate, 0, len(groups))
	for _, candidate := range groups {
		output = append(output, *candidate)
	}
	sort.Slice(output, func(i, j int) bool {
		if len(output[i].reports) != len(output[j].reports) {
			return len(output[i].reports) > len(output[j].reports)
		}
		return output[i].key < output[j].key
	})
	return output
}

func reportItemsByAddress(report registry.StateReport, addresses []string) (map[string]registry.StateReportItem, bool) {
	items := make(map[string]registry.StateReportItem, len(report.Items))
	for _, item := range report.Items {
		if strings.TrimSpace(item.Address) == "" || strings.TrimSpace(item.Provider) == "" || strings.TrimSpace(item.ProviderRevision) == "" || strings.TrimSpace(item.EffectiveHash) == "" {
			return nil, false
		}
		if _, exists := items[item.Address]; exists {
			return nil, false
		}
		items[item.Address] = item
	}
	for _, address := range addresses {
		if _, ok := items[address]; !ok {
			return nil, false
		}
	}
	return items, true
}

func reportMatchesCanonicalPlan(report registry.StateReport, identities []changecontrol.CanonicalResourceIdentity) bool {
	addresses := make([]string, len(identities))
	for index, identity := range identities {
		addresses[index] = identity.Address
	}
	items, ok := reportItemsByAddress(report, addresses)
	if !ok {
		return false
	}
	for _, identity := range identities {
		item := items[identity.Address]
		if item.Provider != identity.Provider || item.ProviderRevision != identity.ProviderRevision || item.EffectiveHash != identity.EffectiveHash {
			return false
		}
	}
	return true
}

func endpointTargetEvidence(reports []registry.StateReport, releaseRef, artifactDigest string, derived configcompose.DerivedFleetPlan) []changecontrol.TargetEvidence {
	resources := make(map[string]changecontrol.ResourcePlan, len(derived.Plan.Resources))
	for _, resource := range derived.Plan.Resources {
		resources[resource.Address] = resource
	}
	targets := make([]changecontrol.TargetEvidence, 0, len(reports))
	for _, report := range reports {
		targets = append(targets, targetEvidence(report, releaseRef, artifactDigest, derived.TrustedIdentities, resources))
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].EndpointID < targets[j].EndpointID })
	return targets
}

func targetEvidence(report registry.StateReport, releaseRef, artifactDigest string, identities []changecontrol.CanonicalResourceIdentity, resources map[string]changecontrol.ResourcePlan) changecontrol.TargetEvidence {
	target := changecontrol.TargetEvidence{EndpointID: report.EndpointID}
	switch {
	case !report.HasReport():
		target.PreflightReason = "state_report_missing"
		return target
	case report.SchemaVersion < 9:
		target.PreflightReason = "plan_evidence_missing"
		return target
	case report.ReleaseRef != releaseRef:
		target.PreflightReason = "release_mismatch"
		return target
	case report.Digest != artifactDigest:
		target.PreflightReason = "artifact_digest_mismatch"
		return target
	case !reportMatchesCanonicalPlan(report, identities):
		target.PreflightReason = "canonical_identity_mismatch"
		return target
	}
	addresses := make([]string, len(identities))
	for index, identity := range identities {
		addresses[index] = identity.Address
	}
	items, _ := reportItemsByAddress(report, addresses)
	for _, identity := range identities {
		item := items[identity.Address]
		if item.Status == registry.StateUnsupported {
			target.PreflightReason = safeTargetReason(item.ReasonCode, "provider_unavailable")
			return target
		}
	}
	target.Compatible = true
	for _, identity := range identities {
		resource := resources[identity.Address]
		if !resource.Risk.RequiresPreflight() {
			continue
		}
		item := items[identity.Address]
		if item.Status != registry.StateCompliant && item.Status != registry.StateDrifted {
			target.PreflightReason = safeTargetReason(item.ReasonCode, "check_evidence_blocked")
			return target
		}
		switch item.PreflightStatus {
		case registry.PlanPreflightReady:
		case registry.PlanPreflightBlocked:
			target.PreflightReason = safeTargetReason(item.PreflightReason, "preflight_failed")
			return target
		default:
			target.PreflightReason = "preflight_evidence_missing"
			return target
		}
	}
	target.PreflightReady = true
	return target
}

func safeTargetReason(reason, fallback string) string {
	if len(reason) == 0 || len(reason) > 64 || reason[0] < 'a' || reason[0] > 'z' {
		return fallback
	}
	for _, character := range reason {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return fallback
		}
	}
	return reason
}
