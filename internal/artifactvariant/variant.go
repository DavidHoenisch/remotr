// Package artifactvariant defines one bounded compiled artifact and its exact
// processing requirements.
package artifactvariant

import "github.com/DavidHoenisch/remotr/internal/artifactrequirements"

// Variant is one canonical schema output for a shared Fleet or endpoint
// override source. It is never tailored by deleting endpoint-incompatible
// resources or fields.
type Variant struct {
	Artifact          []byte
	Digest            string
	SourceDigest      string
	SchemaVersion     int
	Requirements      artifactrequirements.Set
	RequirementDigest string
}
