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
	"gopkg.in/yaml.v3"
)

// ChangePlanDeriver joins composed server state with authenticated endpoint
// capability and non-enforcing Check/preflight evidence.
type ChangePlanDeriver struct {
	ConfigRepoPath string
	ArtifactStore  ArtifactReader
	StateReports   StateReports
	Secrets        secrets.Resolver
}

func (s *Server) deriveBaselineAdoptionPlan(ctx context.Context, fleet string) (configcompose.DerivedFleetPlan, error) {
	deriver := &ChangePlanDeriver{
		ConfigRepoPath: s.cfg.ConfigRepoPath, ArtifactStore: s.cfg.ArtifactStore,
		StateReports: s.cfg.StateReports, Secrets: s.cfg.Secrets,
	}
	return deriver.derive(ctx, fleet, s.releaseRef(ctx), s.cfg.Secrets, nil)
}

func (d *ChangePlanDeriver) derive(ctx context.Context, fleet, releaseRef string, resolver secrets.Resolver, allowedHashChanges map[string]struct{}) (configcompose.DerivedFleetPlan, error) {
	if d == nil || d.StateReports == nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("server-derived Change plan endpoint evidence is unavailable")
	}
	artifact, digest, err := resolveFleetDesiredArtifact(ctx, d.ArtifactStore, d.ConfigRepoPath, fleet, releaseRef)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("resolve composed Fleet artifact: %w", err)
	}
	state, err := models.ParseState(bytes.NewReader(artifact))
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("parse composed Fleet artifact: %w", err)
	}
	reports, err := d.StateReports.ListFleetStateReports(ctx, fleet)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("read current Fleet endpoint evidence: %w", err)
	}
	if resolver != nil {
		resolver = fleetIdentityResolver{base: resolver, fleet: fleet}
	}
	return derivePlanFromEndpointEvidence(ctx, fleet, releaseRef, digest, state, reports.Endpoints, resolver, allowedHashChanges)
}

// fleetIdentityResolver resolves secret identities for the one Fleet whose
// composed plan is being derived. Effective-hash requests carry no endpoint or
// Fleet, so without this binding every Fleet-scoped secret is denied. Endpoint
// scope is never asserted, so endpoint-scoped secrets remain unresolvable here.
type fleetIdentityResolver struct {
	base  secrets.Resolver
	fleet string
}

func (r fleetIdentityResolver) Resolve(ctx context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	if request.Fleet == "" {
		request.Fleet = r.fleet
	}
	return r.base.Resolve(ctx, request)
}

type providerSelectionCandidate struct {
	key        string
	selections map[string]configcompose.ProviderSelection
	reports    []registry.StateReport
}

func derivePlanFromEndpointEvidence(ctx context.Context, fleet, releaseRef, artifactDigest string, state models.State, reports []registry.StateReport, resolver secrets.Resolver, allowedHashChanges map[string]struct{}) (configcompose.DerivedFleetPlan, error) {
	if len(allowedHashChanges) > 0 {
		return derivePlanFromTargetApplicableEvidence(ctx, fleet, releaseRef, artifactDigest, state, reports, resolver, allowedHashChanges)
	}
	addresses, err := composedResourceAddresses(state)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, err
	}
	rejections := deriveRejections{}
	candidates := providerSelectionCandidates(reports, releaseRef, artifactDigest, addresses, allowedHashChanges, rejections)
	for _, candidate := range candidates {
		derived, err := configcompose.DeriveFleetPlan(ctx, fleet, releaseRef, artifactDigest, state, candidate.selections, resolver)
		if err != nil {
			for _, report := range candidate.reports {
				rejections.add(report.EndpointID, "plan derivation failed: "+err.Error())
			}
			continue
		}
		matched := false
		for _, report := range candidate.reports {
			reason := canonicalPlanReason(report, derived.TrustedIdentities, allowedHashChanges)
			if reason == "" {
				matched = true
				break
			}
			rejections.add(report.EndpointID, reason)
		}
		if !matched {
			continue
		}
		derived.Plan.Targets = endpointTargetEvidence(reports, releaseRef, artifactDigest, derived, allowedHashChanges)
		return derived, nil
	}
	return configcompose.DerivedFleetPlan{}, rejections.err()
}

func derivePlanFromTargetApplicableEvidence(ctx context.Context, fleet, releaseRef, artifactDigest string, state models.State, reports []registry.StateReport, resolver secrets.Resolver, allowedHashChanges map[string]struct{}) (configcompose.DerivedFleetPlan, error) {
	rejections := deriveRejections{}
	for _, report := range reports {
		if reason := reportEvidenceReason(report, releaseRef, artifactDigest); reason != "" {
			rejections.add(report.EndpointID, reason)
			continue
		}
		applicable, addresses, reason := stateForReportedTargetReason(state, report)
		if reason != "" {
			rejections.add(report.EndpointID, reason)
			continue
		}
		if missing := missingAddresses(addresses, allowedHashChanges); len(missing) > 0 {
			rejections.add(report.EndpointID, "changed resources outside reported scope: "+boundedAddresses(missing))
			continue
		}
		items, reason := reportItemsReason(report, addresses, allowedHashChanges)
		if reason != "" {
			rejections.add(report.EndpointID, reason)
			continue
		}
		selections := make(map[string]configcompose.ProviderSelection, len(addresses))
		for _, address := range addresses {
			selections[address] = configcompose.ProviderSelection{ID: items[address].Provider}
		}
		derived, err := configcompose.DeriveFleetPlan(ctx, fleet, releaseRef, artifactDigest, applicable, selections, resolver)
		if err != nil {
			rejections.add(report.EndpointID, "plan derivation failed: "+err.Error())
			continue
		}
		if reason := canonicalPlanReason(report, derived.TrustedIdentities, allowedHashChanges); reason != "" {
			rejections.add(report.EndpointID, reason)
			continue
		}
		derived.Plan.Targets = endpointTargetEvidence(reports, releaseRef, artifactDigest, derived, allowedHashChanges)
		return derived, nil
	}
	return configcompose.DerivedFleetPlan{}, rejections.err()
}

func stateForReportedTarget(state models.State, report registry.StateReport) (models.State, []string, bool) {
	applicable, addresses, reason := stateForReportedTargetReason(state, report)
	return applicable, addresses, reason == ""
}

// stateForReportedTargetReason narrows the composed state to configurations the
// endpoint reported, explaining why the report cannot be scoped.
func stateForReportedTargetReason(state models.State, report registry.StateReport) (models.State, []string, string) {
	registered, err := resourceregistry.NewDefault()
	if err != nil {
		return models.State{}, nil, "resource registry unavailable"
	}
	reported := make(map[string]struct{}, len(report.Items))
	for _, item := range report.Items {
		reported[item.Address] = struct{}{}
	}
	applicable := state
	applicable.Configurations = nil
	applicable.ResourceSources = make(map[string]yaml.Node)
	var addresses []string
	for index := range state.Configurations {
		configuration := &state.Configurations[index]
		resources, err := registered.Resources(configuration)
		if err != nil {
			return models.State{}, nil, fmt.Sprintf("configuration %s resources unavailable: %v", configuration.Name, err)
		}
		matched := 0
		configurationAddresses := make([]string, len(resources))
		for resourceIndex, resource := range resources {
			address := models.ResourceAddress(configuration.Name, resource.Name())
			configurationAddresses[resourceIndex] = address
			if _, ok := reported[address]; ok {
				matched++
			}
		}
		if matched == 0 {
			continue
		}
		if matched != len(configurationAddresses) {
			var unreported []string
			for _, address := range configurationAddresses {
				if _, ok := reported[address]; !ok {
					unreported = append(unreported, address)
				}
			}
			return models.State{}, nil, fmt.Sprintf("configuration %s reported partially (%d/%d), missing %s", configuration.Name, matched, len(configurationAddresses), boundedAddresses(unreported))
		}
		applicable.Configurations = append(applicable.Configurations, *configuration)
		addresses = append(addresses, configurationAddresses...)
		for _, address := range configurationAddresses {
			source, ok := state.ResourceSources[address]
			if !ok {
				return models.State{}, nil, fmt.Sprintf("%s has no composed resource source", address)
			}
			applicable.ResourceSources[address] = source
		}
	}
	sort.Strings(addresses)
	if len(addresses) == 0 {
		return applicable, nil, "no composed configuration matches reported resources"
	}
	return applicable, addresses, ""
}

func containsAddresses(addresses []string, required map[string]struct{}) bool {
	return len(missingAddresses(addresses, required)) == 0
}

func missingAddresses(addresses []string, required map[string]struct{}) []string {
	present := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		present[address] = struct{}{}
	}
	var missing []string
	for address := range required {
		if _, ok := present[address]; !ok {
			missing = append(missing, address)
		}
	}
	sort.Strings(missing)
	return missing
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

func providerSelectionCandidates(reports []registry.StateReport, releaseRef, artifactDigest string, addresses []string, allowedHashChanges map[string]struct{}, rejections deriveRejections) []providerSelectionCandidate {
	groups := make(map[string]*providerSelectionCandidate)
	for _, report := range reports {
		if reason := reportEvidenceReason(report, releaseRef, artifactDigest); reason != "" {
			rejections.add(report.EndpointID, reason)
			continue
		}
		items, reason := reportItemsReason(report, addresses, allowedHashChanges)
		if reason != "" {
			rejections.add(report.EndpointID, reason)
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

func reportItemsByAddress(report registry.StateReport, addresses []string, allowedHashChanges map[string]struct{}) (map[string]registry.StateReportItem, bool) {
	items, reason := reportItemsReason(report, addresses, allowedHashChanges)
	return items, reason == ""
}

func reportMatchesCanonicalPlan(report registry.StateReport, identities []changecontrol.CanonicalResourceIdentity, allowedHashChanges map[string]struct{}) bool {
	return canonicalPlanReason(report, identities, allowedHashChanges) == ""
}

func endpointTargetEvidence(reports []registry.StateReport, releaseRef, artifactDigest string, derived configcompose.DerivedFleetPlan, allowedHashChanges map[string]struct{}) []changecontrol.TargetEvidence {
	resources := make(map[string]changecontrol.ResourcePlan, len(derived.Plan.Resources))
	for _, resource := range derived.Plan.Resources {
		resources[resource.Address] = resource
	}
	targets := make([]changecontrol.TargetEvidence, 0, len(reports))
	for _, report := range reports {
		targets = append(targets, targetEvidence(report, releaseRef, artifactDigest, derived.TrustedIdentities, resources, allowedHashChanges))
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].EndpointID < targets[j].EndpointID })
	return targets
}

func targetEvidence(report registry.StateReport, releaseRef, artifactDigest string, identities []changecontrol.CanonicalResourceIdentity, resources map[string]changecontrol.ResourcePlan, allowedHashChanges map[string]struct{}) changecontrol.TargetEvidence {
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
	case !reportMatchesCanonicalPlan(report, identities, allowedHashChanges):
		target.PreflightReason = "canonical_identity_mismatch"
		return target
	}
	addresses := make([]string, len(identities))
	for index, identity := range identities {
		addresses[index] = identity.Address
	}
	items, _ := reportItemsByAddress(report, addresses, allowedHashChanges)
	target.Compatible = true
	target.PreflightReady = true
	for _, identity := range identities {
		resource := resources[identity.Address]
		if !resource.Risk.RequiresPreflight() {
			continue
		}
		item := items[identity.Address]
		evidence := changecontrol.ResourcePreflightEvidence{Address: identity.Address}
		if item.Status == registry.StateUnsupported {
			evidence.Reason = safeTargetReason(item.ReasonCode, "provider_unavailable")
			target.ResourcePreflights = append(target.ResourcePreflights, evidence)
			if target.PreflightReady {
				target.PreflightReady = false
				target.PreflightReason = evidence.Reason
			}
			continue
		}
		if item.Status != registry.StateCompliant && item.Status != registry.StateDrifted {
			evidence.Reason = safeTargetReason(item.ReasonCode, "check_evidence_blocked")
			target.ResourcePreflights = append(target.ResourcePreflights, evidence)
			if target.PreflightReady {
				target.PreflightReady = false
				target.PreflightReason = evidence.Reason
			}
			continue
		}
		switch item.PreflightStatus {
		case registry.PlanPreflightReady:
			evidence.Ready = true
		case registry.PlanPreflightBlocked:
			evidence.Reason = safeTargetReason(item.PreflightReason, "preflight_failed")
		default:
			evidence.Reason = "preflight_evidence_missing"
		}
		target.ResourcePreflights = append(target.ResourcePreflights, evidence)
		if !evidence.Ready && target.PreflightReady {
			target.PreflightReady = false
			target.PreflightReason = evidence.Reason
		}
	}
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
