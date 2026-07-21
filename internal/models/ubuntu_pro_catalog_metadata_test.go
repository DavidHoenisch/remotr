package models_test

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestUbuntuProSpecializedServiceCatalogMetadata(t *testing.T) {
	byName := make(map[string]models.UbuntuProServiceContract)
	for _, contract := range models.UbuntuProServiceCatalog() {
		byName[contract.Name] = contract
	}
	tests := []struct {
		name         string
		risk         models.RiskClass
		locks        []string
		observation  models.UbuntuProServiceObservation
		recovery     models.UbuntuProServiceRecovery
		incompatible []string
	}{
		{name: "esm-apps", risk: models.RiskSensitive, observation: models.UbuntuProObserveEnabledService, recovery: models.UbuntuProRecoverBestEffort},
		{name: "usg", risk: models.RiskSensitive, observation: models.UbuntuProObserveToolingOnly, recovery: models.UbuntuProRecoverBestEffort},
		{name: "livepatch", risk: models.RiskSensitive, observation: models.UbuntuProObserveEnabledService, recovery: models.UbuntuProRecoverBestEffort, incompatible: []string{"fips", "fips-updates", "realtime-kernel"}},
		{name: "fips", risk: models.RiskBoot, locks: []string{"boot"}, observation: models.UbuntuProObserveEnabledService, recovery: models.UbuntuProRecoverNone, incompatible: []string{"fips-updates", "livepatch", "realtime-kernel"}},
		{name: "fips-updates", risk: models.RiskBoot, locks: []string{"boot"}, observation: models.UbuntuProObserveEnabledService, recovery: models.UbuntuProRecoverNone, incompatible: []string{"fips", "livepatch", "realtime-kernel"}},
		{name: "realtime-kernel", risk: models.RiskBoot, locks: []string{"boot"}, observation: models.UbuntuProObserveEnabledVariant, recovery: models.UbuntuProRecoverNone, incompatible: []string{"fips", "fips-updates", "livepatch"}},
		{name: "anbox-cloud", risk: models.RiskSensitive, locks: []string{"package-manager:snap"}, observation: models.UbuntuProObserveEnabledService, recovery: models.UbuntuProRecoverBestEffort},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract, ok := byName[test.name]
			if !ok {
				t.Fatalf("catalog is missing %q", test.name)
			}
			if contract.EnableRisk != test.risk || contract.Observation != test.observation || contract.Activation != models.UbuntuProActivationNativeReboot || contract.Recovery != test.recovery || !slices.Equal(contract.LockDomains, test.locks) || !slices.Equal(contract.IncompatibleWith, test.incompatible) {
				t.Fatalf("catalog[%q] = %+v", test.name, contract)
			}
		})
	}
}

func TestUbuntuProServiceCatalogReturnsIndependentMetadata(t *testing.T) {
	first := models.UbuntuProServiceCatalog()
	first[0].LockDomains = append(first[0].LockDomains, "mutated")
	first[0].IncompatibleWith = append(first[0].IncompatibleWith, "mutated")
	second := models.UbuntuProServiceCatalog()
	if slices.Contains(second[0].LockDomains, "mutated") || slices.Contains(second[0].IncompatibleWith, "mutated") {
		t.Fatalf("catalog metadata aliases mutable storage: %+v", second[0])
	}
}

func TestUbuntuProServiceContractLookupMatchesCatalog(t *testing.T) {
	for _, want := range models.UbuntuProServiceCatalog() {
		got, ok := models.UbuntuProServiceContractFor(want.Name)
		if !ok || got.Name != want.Name || got.EnableRisk != want.EnableRisk || got.Observation != want.Observation || got.Recovery != want.Recovery {
			t.Fatalf("UbuntuProServiceContractFor(%q) = (%+v, %t), want %+v", want.Name, got, ok, want)
		}
	}
	if got, ok := models.UbuntuProServiceContractFor("unknown-service"); ok || got.Name != "" {
		t.Fatalf("UbuntuProServiceContractFor(unknown-service) = (%+v, %t), want zero/false", got, ok)
	}
}
