// Package evidence embeds the repository's reviewed provider qualification
// matrix into production binaries. The YAML file remains the single source of
// truth used by CI gates and capability generation.
package evidence

import _ "embed"

// ProviderMatrixYAML is the exact test/provider-matrix.yaml repository file.
//
//go:embed provider-matrix.yaml
var ProviderMatrixYAML []byte
