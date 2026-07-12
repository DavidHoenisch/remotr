package traceability

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest is the versioned traceability evidence map.
type Manifest struct {
	Version      int                      `yaml:"version"`
	Environments map[string]string        `yaml:"environments"`
	Scenarios    map[string]ManifestEntry `yaml:"scenarios"`
}

// ManifestEntry is one scenario's lifecycle and evidence disposition.
type ManifestEntry struct {
	Source              PrefixOwnership `yaml:"source"`
	Lifecycle           string          `yaml:"lifecycle"`
	VerificationClasses []string        `yaml:"verification_classes"`
	Selectors           []string        `yaml:"selectors"`
	Environments        []string        `yaml:"environments"`
	DispositionReason   string          `yaml:"disposition_reason"`
}

// LoadManifest decodes a version-1 traceability manifest.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode traceability manifest: %w", err)
	}
	return manifest, nil
}
