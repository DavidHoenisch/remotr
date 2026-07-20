package providermatrix

import evidence "github.com/DavidHoenisch/remotr/test"

// Default decodes the provider evidence committed with this source tree. The
// same YAML is executed by repository gates and embedded into agent binaries.
func Default() (Matrix, error) {
	return Decode(evidence.ProviderMatrixYAML)
}
