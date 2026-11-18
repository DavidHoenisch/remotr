package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

const (
	maxDeriveRejectedEndpoints = 20
	maxDeriveReasonAddresses   = 5
)

// deriveRejections records, per endpoint, the first gate that rejected its
// report as evidence for the canonical Fleet plan.
type deriveRejections map[string]string

func (r deriveRejections) add(endpointID, reason string) {
	if endpointID == "" || reason == "" {
		return
	}
	if _, exists := r[endpointID]; !exists {
		r[endpointID] = reason
	}
}

// err explains why no endpoint evidence matched, listing a bounded number of
// endpoints in stable order.
func (r deriveRejections) err() error {
	const base = "no current endpoint evidence matches canonical Fleet composition"
	if len(r) == 0 {
		return fmt.Errorf("%s: no endpoint reports in Fleet", base)
	}
	ids := make([]string, 0, len(r))
	for id := range r {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, 0, min(len(ids), maxDeriveRejectedEndpoints)+1)
	for _, id := range ids[:min(len(ids), maxDeriveRejectedEndpoints)] {
		parts = append(parts, id+": "+r[id])
	}
	if extra := len(ids) - maxDeriveRejectedEndpoints; extra > 0 {
		parts = append(parts, fmt.Sprintf("(+%d more endpoints)", extra))
	}
	return fmt.Errorf("%s: %s", base, strings.Join(parts, "; "))
}

func boundedAddresses(addresses []string) string {
	sorted := append([]string(nil), addresses...)
	sort.Strings(sorted)
	shown := sorted[:min(len(sorted), maxDeriveReasonAddresses)]
	out := strings.Join(shown, ", ")
	if extra := len(sorted) - len(shown); extra > 0 {
		out += fmt.Sprintf(" (+%d more)", extra)
	}
	return out
}

// reportEvidenceReason checks that a report is current plan evidence for the
// composed Fleet artifact.
func reportEvidenceReason(report registry.StateReport, releaseRef, artifactDigest string) string {
	switch {
	case !report.HasReport():
		return "no state report"
	case report.SchemaVersion < 9:
		return fmt.Sprintf("report schema %d predates plan evidence (need 9)", report.SchemaVersion)
	case report.ReleaseRef != releaseRef:
		return fmt.Sprintf("report release %s does not match current release %s", report.ReleaseRef, releaseRef)
	case report.Digest != artifactDigest:
		return fmt.Sprintf("report digest %s does not match Fleet artifact %s (endpoint override or stale artifact)", report.Digest, artifactDigest)
	}
	return ""
}

// reportItemsReason indexes report items by address and explains the first
// item or coverage problem that makes the report unusable.
func reportItemsReason(report registry.StateReport, addresses []string, allowedHashChanges map[string]struct{}) (map[string]registry.StateReportItem, string) {
	items := make(map[string]registry.StateReportItem, len(report.Items))
	for _, item := range report.Items {
		if strings.TrimSpace(item.Address) == "" || strings.TrimSpace(item.Provider) == "" || strings.TrimSpace(item.ProviderRevision) == "" {
			return nil, fmt.Sprintf("%s lacks address, provider, or provider revision", item.Address)
		}
		if item.EffectiveHash == "" {
			_, hashMayChange := allowedHashChanges[item.Address]
			if report.SchemaVersion < 10 || !hashMayChange || item.EffectiveHashStatus != "authorization_required" {
				status := item.EffectiveHashStatus
				if status == "" {
					status = "no status"
				}
				return nil, fmt.Sprintf("%s has no effective hash (%s)", item.Address, status)
			}
		} else if item.EffectiveHashStatus != "" {
			return nil, fmt.Sprintf("%s reports both an effective hash and status %s", item.Address, item.EffectiveHashStatus)
		}
		if _, exists := items[item.Address]; exists {
			return nil, fmt.Sprintf("%s is reported more than once", item.Address)
		}
		items[item.Address] = item
	}
	var missing []string
	for _, address := range addresses {
		if _, ok := items[address]; !ok {
			missing = append(missing, address)
		}
	}
	if len(missing) > 0 {
		return nil, "composed resources not reported: " + boundedAddresses(missing)
	}
	return items, ""
}

// canonicalPlanReason explains the first reported identity that differs from
// the canonical derived identity.
func canonicalPlanReason(report registry.StateReport, identities []changecontrol.CanonicalResourceIdentity, allowedHashChanges map[string]struct{}) string {
	addresses := make([]string, len(identities))
	for index, identity := range identities {
		addresses[index] = identity.Address
	}
	items, reason := reportItemsReason(report, addresses, allowedHashChanges)
	if reason != "" {
		return reason
	}
	for _, identity := range identities {
		item := items[identity.Address]
		_, hashMayChange := allowedHashChanges[identity.Address]
		switch {
		case item.Provider != identity.Provider:
			return fmt.Sprintf("%s provider %s differs from canonical %s", identity.Address, item.Provider, identity.Provider)
		case item.ProviderRevision != identity.ProviderRevision:
			return fmt.Sprintf("%s provider revision %s differs from canonical %s", identity.Address, item.ProviderRevision, identity.ProviderRevision)
		case !hashMayChange && item.EffectiveHash != identity.EffectiveHash:
			return fmt.Sprintf("%s effective hash differs from canonical composition", identity.Address)
		}
	}
	return ""
}
