package providercontract

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

const (
	maxPlanEffects           = 16
	maxPlanEffectFields      = 32
	maxPlanActivationTargets = 32
	maxPlanTargetBytes       = 256
)

// EffectCode is a closed machine-readable prediction emitted by a provider's
// plan contract. Providers cannot substitute prose for one of these codes.
type EffectCode string

const (
	EffectNetworkDNSReplace     EffectCode = "network_dns_replace"
	EffectDefaultRouteReplace   EffectCode = "network_default_route_replace"
	EffectFirewallPolicyReplace EffectCode = "network_firewall_policy_replace"
	EffectSudoPolicyReplace     EffectCode = "access_sudo_policy_replace"
	EffectSecretVersionActivate EffectCode = "secret_version_activate"
	EffectResourceUpdate        EffectCode = "resource_update"
)

func (c EffectCode) valid() bool {
	switch c {
	case EffectNetworkDNSReplace, EffectDefaultRouteReplace, EffectFirewallPolicyReplace, EffectSudoPolicyReplace,
		EffectSecretVersionActivate, EffectResourceUpdate:
		return true
	default:
		return false
	}
}

// PlanEffect is a bounded typed effect plus an optional classified projection.
// Raw desired values and provider-authored free-form descriptions have no
// representable field in this contract.
type PlanEffect struct {
	Code    EffectCode           `json:"code"`
	Details executor.SafeSummary `json:"details,omitempty"`
}

// Validate rejects unknown effect types and unsafe or unbounded details.
func (e PlanEffect) Validate() error {
	if !e.Code.valid() {
		return fmt.Errorf("invalid plan effect code %q", e.Code)
	}
	if len(e.Details.Fields) > maxPlanEffectFields {
		return fmt.Errorf("plan effect details exceed %d fields", maxPlanEffectFields)
	}
	if err := e.Details.Validate(); err != nil {
		return fmt.Errorf("invalid plan effect details: %w", err)
	}
	return nil
}

// ActivationTarget predicts explicit post-Apply work without performing it.
type ActivationTarget struct {
	Kind   ActivationKind `json:"kind"`
	Target string         `json:"target,omitempty"`
}

// PlanDescriptor is the complete provider-owned planning evidence for one
// resource. Server plan builders consume only validated descriptors.
type PlanDescriptor struct {
	Effects           []PlanEffect       `json:"effects"`
	RollbackClass     RollbackClass      `json:"rollback_class"`
	ActivationTargets []ActivationTarget `json:"activation_targets,omitempty"`
	BaselineEligible  bool               `json:"baseline_eligible"`
}

// Validate enforces the closed, bounded plan evidence contract.
func (d PlanDescriptor) Validate() error {
	if len(d.Effects) == 0 || len(d.Effects) > maxPlanEffects {
		return fmt.Errorf("plan descriptor requires 1 to %d effects", maxPlanEffects)
	}
	for index, effect := range d.Effects {
		if err := effect.Validate(); err != nil {
			return fmt.Errorf("effect %d: %w", index+1, err)
		}
	}
	switch d.RollbackClass {
	case RollbackTransactional, RollbackBestEffort, RollbackNone:
	default:
		return fmt.Errorf("invalid plan rollback class %q", d.RollbackClass)
	}
	if len(d.ActivationTargets) > maxPlanActivationTargets {
		return fmt.Errorf("plan descriptor exceeds %d activation targets", maxPlanActivationTargets)
	}
	seenActivations := make(map[string]struct{}, len(d.ActivationTargets))
	for index, activation := range d.ActivationTargets {
		if err := activation.validate(); err != nil {
			return fmt.Errorf("activation target %d: %w", index+1, err)
		}
		key := string(activation.Kind) + "\x00" + activation.Target
		if _, exists := seenActivations[key]; exists {
			return fmt.Errorf("duplicate activation target %q", activation.Target)
		}
		seenActivations[key] = struct{}{}
	}
	return nil
}

func (a ActivationTarget) validate() error {
	if a.Target != strings.TrimSpace(a.Target) || len(a.Target) > maxPlanTargetBytes || !utf8.ValidString(a.Target) {
		return fmt.Errorf("invalid target")
	}
	requiresTarget := false
	switch a.Kind {
	case ActivationReload, ActivationTryRestart, ActivationRestart, ActivationApplicationRestart, ActivationTrustStoreRefresh:
		requiresTarget = true
	case ActivationDaemonReload, ActivationLogoutRequired, ActivationNextBoot, ActivationRebootRequired:
	default:
		return fmt.Errorf("invalid activation kind %q", a.Kind)
	}
	if requiresTarget && a.Target == "" {
		return fmt.Errorf("activation kind %q requires a target", a.Kind)
	}
	if !requiresTarget && a.Target != "" {
		return fmt.Errorf("activation kind %q does not accept a target", a.Kind)
	}
	return nil
}
