// Package capabilitydoc models the bounded endpoint evidence carried by every
// modern authenticated Sync request.
package capabilitydoc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const CurrentDocumentVersion = 1

var ErrDigestMismatch = errors.New("capability document digest mismatch")

// Capability is one stable resource or provider contract supported by an
// endpoint. Revision is the implemented contract revision, not an agent
// release number.
type Capability struct {
	ID       string   `json:"id"`
	Revision string   `json:"revision"`
	Features []string `json:"features,omitempty"`
}

// Fact is one normalized, non-secret endpoint provider selection fact.
type Fact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Document is the versioned capability evidence submitted by an agent.
type Document struct {
	DocumentVersion        int          `json:"documentVersion"`
	ArtifactSchemaVersions []int        `json:"artifactSchemaVersions"`
	Capabilities           []Capability `json:"capabilities"`
	Facts                  []Fact       `json:"facts"`
	AgentVersion           string       `json:"agentVersion"`
	Digest                 string       `json:"digest"`
}

type canonicalDocument struct {
	DocumentVersion        int          `json:"documentVersion"`
	ArtifactSchemaVersions []int        `json:"artifactSchemaVersions"`
	Capabilities           []Capability `json:"capabilities"`
	Facts                  []Fact       `json:"facts"`
	AgentVersion           string       `json:"agentVersion"`
}

// CanonicalBody returns the compact, deterministic JSON body over which the
// document digest is computed. It never mutates caller-owned slices.
func (d Document) CanonicalBody() ([]byte, error) {
	schemas := append([]int(nil), d.ArtifactSchemaVersions...)
	capabilities := cloneCapabilities(d.Capabilities)
	facts := append([]Fact(nil), d.Facts...)
	sort.Ints(schemas)
	sort.Slice(capabilities, func(i, j int) bool {
		if capabilities[i].ID == capabilities[j].ID {
			return capabilities[i].Revision < capabilities[j].Revision
		}
		return capabilities[i].ID < capabilities[j].ID
	})
	for index := range capabilities {
		sort.Strings(capabilities[index].Features)
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Key == facts[j].Key {
			return facts[i].Value < facts[j].Value
		}
		return facts[i].Key < facts[j].Key
	})
	return json.Marshal(canonicalDocument{
		DocumentVersion:        d.DocumentVersion,
		ArtifactSchemaVersions: schemas,
		Capabilities:           capabilities,
		Facts:                  facts,
		AgentVersion:           d.AgentVersion,
	})
}

func cloneCapabilities(input []Capability) []Capability {
	output := append([]Capability(nil), input...)
	for index := range output {
		output[index].Features = append([]string(nil), input[index].Features...)
	}
	return output
}

// CanonicalDigest recomputes the submitted digest from the canonical body.
func (d Document) CanonicalDigest() (string, error) {
	body, err := d.CanonicalBody()
	if err != nil {
		return "", fmt.Errorf("canonical capability document: %w", err)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// WithCanonicalDigest returns a copy containing its recomputed digest.
func (d Document) WithCanonicalDigest() (Document, error) {
	digest, err := d.CanonicalDigest()
	if err != nil {
		return Document{}, err
	}
	d.Digest = digest
	return d, nil
}

// Validate verifies structural bounds, grammar, uniqueness, internal
// consistency, and the submitted canonical digest.
func (d Document) Validate() error {
	return validateDocument(d)
}
