package changecontrol

import (
	"encoding/json"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

// EffectCode is a closed machine-readable predicted effect. Human or
// resource-derived details must use the separately classified summary.
type EffectCode string

const (
	EffectNetworkDNSReplace     EffectCode = "network_dns_replace"
	EffectDefaultRouteReplace   EffectCode = "network_default_route_replace"
	EffectFirewallPolicyReplace EffectCode = "network_firewall_policy_replace"
	EffectSudoPolicyReplace     EffectCode = "access_sudo_policy_replace"
	EffectSecretVersionActivate EffectCode = "secret_version_activate"
	EffectResourceUpdate        EffectCode = "resource_update"
	EffectLegacyUnclassified    EffectCode = "legacy_unclassified_effect"
)

func (c EffectCode) valid() bool {
	switch c {
	case EffectNetworkDNSReplace, EffectDefaultRouteReplace, EffectFirewallPolicyReplace, EffectSudoPolicyReplace,
		EffectSecretVersionActivate, EffectResourceUpdate, EffectLegacyUnclassified:
		return true
	default:
		return false
	}
}

// PredictedEffect is the only effect shape admitted to Change-control durable
// state and output.
type PredictedEffect struct {
	Code    EffectCode           `json:"code"`
	Details executor.SafeSummary `json:"details,omitempty"`
}

func (e PredictedEffect) Validate() error {
	if !e.Code.valid() {
		return fmt.Errorf("invalid predicted effect code %q", e.Code)
	}
	if err := e.Details.Validate(); err != nil {
		return fmt.Errorf("invalid predicted effect details: %w", err)
	}
	return nil
}

func (e PredictedEffect) String() string {
	if details := e.Details.String(); details != "" {
		return string(e.Code) + ": " + details
	}
	return string(e.Code)
}

func clonePredictedEffects(effects []PredictedEffect) []PredictedEffect {
	cloned := append([]PredictedEffect(nil), effects...)
	for index := range cloned {
		cloned[index].Details = cloned[index].Details.Clone()
	}
	return cloned
}

func (e PredictedEffect) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	type wire PredictedEffect
	return json.Marshal(wire(e))
}

func (e *PredictedEffect) UnmarshalJSON(data []byte) error {
	type wire PredictedEffect
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	effect := PredictedEffect(decoded)
	if err := effect.Validate(); err != nil {
		return err
	}
	*e = effect
	return nil
}
