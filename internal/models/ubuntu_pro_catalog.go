package models

import "slices"

type UbuntuProServiceObservation string

const (
	UbuntuProObserveEnabledService UbuntuProServiceObservation = "enabled-service"
	UbuntuProObserveEnabledVariant UbuntuProServiceObservation = "enabled-variant"
	UbuntuProObserveToolingOnly    UbuntuProServiceObservation = "tooling-only"
)

type UbuntuProServiceActivation string

const UbuntuProActivationNativeReboot UbuntuProServiceActivation = "native-reboot-result"

type UbuntuProServiceRecovery string

const (
	UbuntuProRecoverBestEffort UbuntuProServiceRecovery = "best-effort-control-state"
	UbuntuProRecoverNone       UbuntuProServiceRecovery = "no-automatic-rollback"
)

// UbuntuProServiceContract is the checked-in authoring contract for one
// stable Canonical service. Runtime qualification remains a separate exact
// release/architecture/API evidence decision.
type UbuntuProServiceContract struct {
	Name             string
	StatusAliases    []string
	EnableModes      []UbuntuProEnableMode
	Variants         []string
	DisableModes     []UbuntuProDisableMode
	EnableRisk       RiskClass
	LockDomains      []string
	Observation      UbuntuProServiceObservation
	Activation       UbuntuProServiceActivation
	Recovery         UbuntuProServiceRecovery
	IncompatibleWith []string
}

var ubuntuProServiceCatalog = []UbuntuProServiceContract{
	{Name: "esm-infra", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskSensitive, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverBestEffort},
	{Name: "esm-apps", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskSensitive, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverBestEffort},
	{Name: "livepatch", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskSensitive, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverBestEffort, IncompatibleWith: []string{"fips", "fips-updates", "realtime-kernel"}},
	{Name: "usg", StatusAliases: []string{"cis"}, EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskSensitive, Observation: UbuntuProObserveToolingOnly, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverBestEffort},
	{Name: "fips", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskBoot, LockDomains: []string{"boot"}, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverNone, IncompatibleWith: []string{"fips-updates", "livepatch", "realtime-kernel"}},
	{Name: "fips-updates", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskBoot, LockDomains: []string{"boot"}, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverNone, IncompatibleWith: []string{"fips", "livepatch", "realtime-kernel"}},
	{Name: "realtime-kernel", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, Variants: []string{"intel-iotg", "raspi"}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskBoot, LockDomains: []string{"boot"}, Observation: UbuntuProObserveEnabledVariant, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverNone, IncompatibleWith: []string{"fips", "fips-updates", "livepatch"}},
	{Name: "ros", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskSensitive, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverBestEffort},
	{Name: "ros-updates", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskSensitive, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverBestEffort},
	{Name: "anbox-cloud", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}, EnableRisk: RiskSensitive, LockDomains: []string{"package-manager:snap"}, Observation: UbuntuProObserveEnabledService, Activation: UbuntuProActivationNativeReboot, Recovery: UbuntuProRecoverBestEffort},
}

var ubuntuProHistoricalServices = map[string]string{
	"cis":              "historical service name; use usg only on an explicitly qualified release row",
	"cc-eal":           "historical service is not qualified on Ubuntu 20.04 through 26.04",
	"esm-infra-legacy": "legacy ESM service is not qualified on Ubuntu 20.04 through 26.04",
	"esm-apps-legacy":  "legacy ESM service is not qualified on Ubuntu 20.04 through 26.04",
}

func UbuntuProServiceCatalog() []UbuntuProServiceContract {
	result := slices.Clone(ubuntuProServiceCatalog)
	for index := range result {
		result[index].StatusAliases = slices.Clone(result[index].StatusAliases)
		result[index].EnableModes = slices.Clone(result[index].EnableModes)
		result[index].Variants = slices.Clone(result[index].Variants)
		result[index].DisableModes = slices.Clone(result[index].DisableModes)
		result[index].LockDomains = slices.Clone(result[index].LockDomains)
		result[index].IncompatibleWith = slices.Clone(result[index].IncompatibleWith)
	}
	return result
}

func UbuntuProServiceContractFor(name string) (UbuntuProServiceContract, bool) {
	contract, ok := ubuntuProServiceContract(name)
	if !ok {
		return UbuntuProServiceContract{}, false
	}
	contract.StatusAliases = slices.Clone(contract.StatusAliases)
	contract.EnableModes = slices.Clone(contract.EnableModes)
	contract.Variants = slices.Clone(contract.Variants)
	contract.DisableModes = slices.Clone(contract.DisableModes)
	contract.LockDomains = slices.Clone(contract.LockDomains)
	contract.IncompatibleWith = slices.Clone(contract.IncompatibleWith)
	return contract, true
}

func ubuntuProServiceContract(name string) (UbuntuProServiceContract, bool) {
	for _, contract := range ubuntuProServiceCatalog {
		if contract.Name == name {
			return contract, true
		}
	}
	return UbuntuProServiceContract{}, false
}
