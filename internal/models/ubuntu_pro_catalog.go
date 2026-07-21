package models

import "slices"

// UbuntuProServiceContract is the checked-in authoring contract for one
// stable Canonical service. Runtime qualification remains a separate exact
// release/architecture/API evidence decision.
type UbuntuProServiceContract struct {
	Name          string
	StatusAliases []string
	EnableModes   []UbuntuProEnableMode
	Variants      []string
	DisableModes  []UbuntuProDisableMode
}

var ubuntuProServiceCatalog = []UbuntuProServiceContract{
	{Name: "esm-infra", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "esm-apps", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "livepatch", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "usg", StatusAliases: []string{"cis"}, EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "fips", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "fips-updates", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "realtime-kernel", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, Variants: []string{"intel-iotg", "raspi"}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "ros", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "ros-updates", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
	{Name: "anbox-cloud", EnableModes: []UbuntuProEnableMode{UbuntuProEnableFull, UbuntuProEnableAccessOnly}, DisableModes: []UbuntuProDisableMode{UbuntuProRetainPackages, UbuntuProPurgePackages}},
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
	}
	return result
}

func ubuntuProServiceContract(name string) (UbuntuProServiceContract, bool) {
	for _, contract := range ubuntuProServiceCatalog {
		if contract.Name == name {
			return contract, true
		}
	}
	return UbuntuProServiceContract{}, false
}
