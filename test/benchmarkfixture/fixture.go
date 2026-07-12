// Package benchmarkfixture provides versioned, deterministic benchmark inputs.
package benchmarkfixture

import (
	"fmt"
	"strings"
)

// GeneratorVersion changes only when the deterministic fixture shape changes.
const GeneratorVersion = "v1"

// ResourceCount is the number of package resources in an artifact fixture.
type ResourceCount int

// String returns the decimal resource count for stable benchmark names.
func (r ResourceCount) String() string { return fmt.Sprintf("%d", r) }

// Sizes returns the pinned representative resource counts used by benchmarks.
func Sizes() []ResourceCount { return []ResourceCount{10, 100, 500, 1000} }

// Artifact returns a deterministic, valid desired-state YAML artifact with the
// requested number of independent package resources.
func Artifact(resourceCount ResourceCount) []byte {
	if resourceCount <= 0 {
		panic("benchmark fixture resource count must be positive")
	}

	var out strings.Builder
	out.Grow(96 + int(resourceCount)*58)
	out.WriteString("configurations:\n  - name: benchmark\n    packages:\n")
	for i := 0; i < int(resourceCount); i++ {
		fmt.Fprintf(&out, "      - name: pkg-%04d\n        present: true\n", i)
	}
	return []byte(out.String())
}
