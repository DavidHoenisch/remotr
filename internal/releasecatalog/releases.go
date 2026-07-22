package releasecatalog

import (
	"bytes"
	"fmt"
	"slices"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:generate go run -mod=vendor ./cmd/generate-releases -source releases.yaml -output generated_releases.go

type AgentRelease struct {
	Version            string   `yaml:"version"`
	CapabilityDocument bool     `yaml:"capability_document"`
	UpgradeEligible    bool     `yaml:"upgrade_eligible"`
	Revoked            bool     `yaml:"revoked,omitempty"`
	Integrity          string   `yaml:"integrity,omitempty"`
	Schemas            []int    `yaml:"schemas"`
	Platforms          []string `yaml:"platforms"`
	Architectures      []string `yaml:"architectures"`
}

type agentReleaseCatalog struct {
	Version  int            `yaml:"version"`
	Releases []AgentRelease `yaml:"releases"`
}

var (
	releasesOnce sync.Once
	releasesByID map[string]AgentRelease
	releasesErr  error
)

func AgentReleaseByVersion(version string) (AgentRelease, bool, error) {
	releasesOnce.Do(func() {
		catalog, err := decodeAgentReleases(generatedAgentReleasesYAML)
		if err != nil {
			releasesErr = err
			return
		}
		releasesByID = make(map[string]AgentRelease, len(catalog.Releases))
		for _, release := range catalog.Releases {
			releasesByID[release.Version] = release
		}
	})
	if releasesErr != nil {
		return AgentRelease{}, false, releasesErr
	}
	release, ok := releasesByID[version]
	release.Schemas = slices.Clone(release.Schemas)
	release.Platforms = slices.Clone(release.Platforms)
	release.Architectures = slices.Clone(release.Architectures)
	return release, ok, nil
}

// ValidateAgentReleases rejects release metadata that cannot safely drive an
// upgrade decision. It is exported for the deterministic catalog generator;
// runtime lookup validates the embedded bytes again before trusting them.
func ValidateAgentReleases(raw []byte) error {
	_, err := decodeAgentReleases(raw)
	return err
}

func decodeAgentReleases(raw []byte) (agentReleaseCatalog, error) {
	var catalog agentReleaseCatalog
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&catalog); err != nil {
		return agentReleaseCatalog{}, fmt.Errorf("decode agent release catalog: %w", err)
	}
	if catalog.Version != 1 {
		return agentReleaseCatalog{}, fmt.Errorf("agent release catalog version = %d", catalog.Version)
	}
	seen := make(map[string]bool, len(catalog.Releases))
	for _, release := range catalog.Releases {
		if release.Version == "" || seen[release.Version] {
			return agentReleaseCatalog{}, fmt.Errorf("duplicate or empty agent release")
		}
		seen[release.Version] = true
		if release.Revoked && release.UpgradeEligible {
			return agentReleaseCatalog{}, fmt.Errorf("agent release %q is both revoked and upgrade eligible", release.Version)
		}
		if release.UpgradeEligible && release.Integrity != "sha256-manifest" {
			return agentReleaseCatalog{}, fmt.Errorf("agent release %q has no approved integrity control", release.Version)
		}
		if len(release.Schemas) == 0 || len(release.Platforms) == 0 || len(release.Architectures) == 0 {
			return agentReleaseCatalog{}, fmt.Errorf("agent release %q has incomplete eligibility metadata", release.Version)
		}
		for _, schema := range release.Schemas {
			if schema != 0 && schema != 1 {
				return agentReleaseCatalog{}, fmt.Errorf("agent release %q has unsupported schema", release.Version)
			}
		}
		for _, platform := range release.Platforms {
			if platform != "linux" {
				return agentReleaseCatalog{}, fmt.Errorf("agent release %q has unsupported platform", release.Version)
			}
		}
		for _, architecture := range release.Architectures {
			if architecture != "x86" && architecture != "arm" {
				return agentReleaseCatalog{}, fmt.Errorf("agent release %q has unsupported architecture", release.Version)
			}
		}
	}
	return catalog, nil
}
