// Package services defines provider-neutral service capability contracts. It
// does not make a provider available merely because its identity is known.
package services

import (
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/models"
)

type Capability string

const (
	CapabilityEnabled   Capability = "enabled"
	CapabilityActive    Capability = "active"
	CapabilityMasked    Capability = "masked"
	CapabilityUserScope Capability = "user-scope"
	CapabilityLinger    Capability = "linger"
)

// Evidence is the minimum gate for a provider support claim. Unit tests or a
// detected init binary alone are not sufficient real-provider evidence.
type Evidence struct {
	FullContractSuite bool
	RealEnvironment   bool
}

func (e Evidence) Complete() bool { return e.FullContractSuite && e.RealEnvironment }

type Contract struct {
	Provider     models.ServiceProvider
	Revision     string
	Capabilities map[Capability]bool
	evidence     Evidence
}

func (c Contract) Supports(capability Capability) bool { return c.Capabilities[capability] }
func (c Contract) Advertised() bool                    { return c.evidence.Complete() }

func (c Contract) WithEvidence(evidence Evidence) Contract {
	c.evidence = evidence
	return c
}

func (c Contract) Require(capability Capability) error {
	if !c.Supports(capability) {
		return UnsupportedCapabilityError{Provider: c.Provider, Capability: capability}
	}
	if !c.Advertised() {
		return ProviderNotAdvertisedError{Provider: c.Provider}
	}
	return nil
}

func (c Contract) RequireAdvertised() error {
	if c.Advertised() {
		return nil
	}
	return ProviderNotAdvertisedError{Provider: c.Provider}
}

type UnsupportedCapabilityError struct {
	Provider   models.ServiceProvider
	Capability Capability
}

func (e UnsupportedCapabilityError) Error() string {
	return fmt.Sprintf("service provider %s does not support %s state", e.Provider, e.Capability)
}

type ProviderNotAdvertisedError struct{ Provider models.ServiceProvider }

func (e ProviderNotAdvertisedError) Error() string {
	return fmt.Sprintf("service provider %s is not advertised until its full provider contract and real-environment evidence pass", e.Provider)
}

var contracts = map[models.ServiceProvider]Contract{
	models.ServiceProviderSystemd: {
		Provider: models.ServiceProviderSystemd, Revision: "service-state-v1",
		Capabilities: capabilitySet(CapabilityEnabled, CapabilityActive, CapabilityMasked, CapabilityUserScope, CapabilityLinger),
		evidence:     Evidence{FullContractSuite: true, RealEnvironment: true},
	},
	models.ServiceProviderOpenRC: {
		Provider: models.ServiceProviderOpenRC, Revision: "service-state-v1",
		Capabilities: capabilitySet(CapabilityEnabled, CapabilityActive),
	},
	models.ServiceProviderSysV: {
		Provider: models.ServiceProviderSysV, Revision: "service-state-v1",
		Capabilities: capabilitySet(CapabilityEnabled, CapabilityActive),
	},
}

func ContractFor(provider models.ServiceProvider) (Contract, bool) {
	contract, ok := contracts[provider]
	if !ok {
		return Contract{}, false
	}
	capabilities := make(map[Capability]bool, len(contract.Capabilities))
	for capability, supported := range contract.Capabilities {
		capabilities[capability] = supported
	}
	contract.Capabilities = capabilities
	return contract, true
}

func capabilitySet(capabilities ...Capability) map[Capability]bool {
	set := make(map[Capability]bool, len(capabilities))
	for _, capability := range capabilities {
		set[capability] = true
	}
	return set
}
