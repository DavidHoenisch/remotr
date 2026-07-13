package services_test

import (
	"errors"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/services"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestDeferredProviderContractsDescribeCapabilitiesWithoutAdvertising(t *testing.T) {
	for _, provider := range []models.ServiceProvider{models.ServiceProviderOpenRC, models.ServiceProviderSysV} {
		t.Run(string(provider), func(t *testing.T) {
			contract, ok := services.ContractFor(provider)
			if !ok {
				t.Fatalf("ContractFor(%q) is missing", provider)
			}
			if !contract.Supports(services.CapabilityEnabled) || !contract.Supports(services.CapabilityActive) {
				t.Fatalf("%s state capabilities = %+v", provider, contract.Capabilities)
			}
			if contract.Supports(services.CapabilityMasked) || contract.Supports(services.CapabilityUserScope) || contract.Supports(services.CapabilityLinger) {
				t.Fatalf("%s overstates capabilities = %+v", provider, contract.Capabilities)
			}
			if contract.Advertised() {
				t.Fatalf("%s must remain unadvertised without full provider evidence", provider)
			}
			if err := contract.Require(services.CapabilityMasked); err == nil {
				t.Fatalf("%s masking unexpectedly accepted", provider)
			} else {
				var unsupported services.UnsupportedCapabilityError
				if !errors.As(err, &unsupported) {
					t.Fatalf("masked error = %T %v", err, err)
				}
			}
		})
	}
}

func TestProviderAdvertisementRequiresContractAndRealEnvironmentEvidence(t *testing.T) {
	base, ok := services.ContractFor(models.ServiceProviderOpenRC)
	if !ok {
		t.Fatal("OpenRC contract is missing")
	}
	for _, test := range []struct {
		name     string
		evidence services.Evidence
		want     bool
	}{
		{"none", services.Evidence{}, false},
		{"contract only", services.Evidence{FullContractSuite: true}, false},
		{"environment only", services.Evidence{RealEnvironment: true}, false},
		{"complete", services.Evidence{FullContractSuite: true, RealEnvironment: true}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := base.WithEvidence(test.evidence).Advertised(); got != test.want {
				t.Fatalf("Advertised() = %t, want %t", got, test.want)
			}
		})
	}
}
