package configcompose

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/safepath"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

const maxExtendsDepth = 32

// Manifest is the source document for composing a deployable desired artifact.
type Manifest struct {
	Kind         types.Kind             `yaml:"kind"`
	Extends      string                 `yaml:"extends,omitempty"`
	Modules      []string               `yaml:"modules,omitempty"`
	Applications []string               `yaml:"applications,omitempty"`
	Crons        []string               `yaml:"crons,omitempty"`
	Overrides    []models.Configuration `yaml:"overrides,omitempty"`
}

func parseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	if m.Kind != types.KindManifest {
		return Manifest{}, fmt.Errorf("want kind %s, got %q", types.KindManifest, m.Kind)
	}
	return m, nil
}

func loadManifest(repoRoot, relPath string) (Manifest, error) {
	data, err := readRepoRelative(repoRoot, relPath)
	if err != nil {
		return Manifest{}, err
	}
	m, err := parseManifest(data)
	if err != nil {
		return Manifest{}, fmt.Errorf("load manifest %q: %w", relPath, err)
	}
	return m, nil
}

func readRepoRelative(repoRoot, relPath string) ([]byte, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return nil, fmt.Errorf("empty path")
	}
	if filepath.IsAbs(relPath) {
		return nil, fmt.Errorf("path must be relative to repository root: %q", relPath)
	}
	rel := filepath.FromSlash(relPath)
	parts := strings.Split(rel, string(filepath.Separator))
	return safepath.ReadUnderRoot(repoRoot, parts...)
}

func resolveManifestChain(repoRoot, manifestRel string, seen map[string]struct{}) (Manifest, error) {
	manifestRel = normalizeRelPath(manifestRel)
	if manifestRel == "" {
		return Manifest{}, fmt.Errorf("empty manifest path")
	}
	if _, ok := seen[manifestRel]; ok {
		return Manifest{}, fmt.Errorf("extends cycle at %q", manifestRel)
	}
	if len(seen) >= maxExtendsDepth {
		return Manifest{}, fmt.Errorf("extends depth exceeds %d at %q", maxExtendsDepth, manifestRel)
	}
	seen[manifestRel] = struct{}{}

	m, err := loadManifest(repoRoot, manifestRel)
	if err != nil {
		return Manifest{}, err
	}

	var merged Manifest
	if ext := strings.TrimSpace(m.Extends); ext != "" {
		parent, err := resolveManifestChain(repoRoot, ext, seen)
		if err != nil {
			return Manifest{}, err
		}
		merged.Modules = append(merged.Modules, parent.Modules...)
		merged.Applications = append(merged.Applications, parent.Applications...)
		merged.Crons = append(merged.Crons, parent.Crons...)
		merged.Overrides = append(merged.Overrides, parent.Overrides...)
	}
	merged.Modules = append(merged.Modules, m.Modules...)
	merged.Applications = append(merged.Applications, m.Applications...)
	merged.Crons = append(merged.Crons, m.Crons...)
	merged.Overrides = append(merged.Overrides, m.Overrides...)
	return merged, nil
}

func normalizeRelPath(path string) string {
	path = strings.TrimSpace(path)
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	return path
}

func loadModuleState(repoRoot, modulePath string) (models.State, error) {
	data, err := readRepoRelative(repoRoot, modulePath)
	if err != nil {
		return models.State{}, fmt.Errorf("read module %q: %w", modulePath, err)
	}
	state, diagnostics, err := models.ParseStateWithDiagnostics(bytes.NewReader(data))
	if err != nil {
		return models.State{}, fmt.Errorf("parse module %q: %w", modulePath, err)
	}
	if state.Kind != types.KindModule {
		return models.State{}, fmt.Errorf("module %q: want kind %s, got %q", modulePath, types.KindModule, state.Kind)
	}
	state.Diagnostics = diagnostics
	return state, nil
}

func mergeConfigurations(base []models.Configuration, overrides []models.Configuration) ([]models.Configuration, error) {
	byName := make(map[string]int, len(base))
	out := append([]models.Configuration(nil), base...)
	for i, cfg := range out {
		name := strings.TrimSpace(cfg.Name)
		if name == "" {
			return nil, fmt.Errorf("configuration missing name")
		}
		byName[name] = i
	}

	for _, override := range overrides {
		name := strings.TrimSpace(override.Name)
		if name == "" {
			return nil, fmt.Errorf("override missing name")
		}
		idx, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("override %q: no matching configuration", name)
		}
		out[idx] = mergeConfiguration(out[idx], override)
	}
	return out, nil
}

func mergeConfiguration(base, override models.Configuration) models.Configuration {
	out := base
	if n := strings.TrimSpace(override.Name); n != "" {
		out.Name = n
	}
	if d := strings.TrimSpace(override.Description); d != "" {
		out.Description = d
	}
	if !override.LastUpdated.IsZero() {
		out.LastUpdated = override.LastUpdated
	}
	if len(override.TargetDistros) > 0 {
		out.TargetDistros = append([]types.Distro(nil), override.TargetDistros...)
	}
	if len(override.TargetArch) > 0 {
		out.TargetArch = append([]types.Architecture(nil), override.TargetArch...)
	}
	if len(override.Packages) > 0 {
		out.Packages = override.Packages
	}
	if len(override.Files) > 0 {
		out.Files = override.Files
	}
	if len(override.Directories) > 0 {
		out.Directories = override.Directories
	}
	if len(override.Links) > 0 {
		out.Links = override.Links
	}
	if len(override.UserFiles) > 0 {
		out.UserFiles = override.UserFiles
	}
	if len(override.Downloads) > 0 {
		out.Downloads = override.Downloads
	}
	if len(override.Users) > 0 {
		out.Users = override.Users
	}
	if len(override.Systemd) > 0 {
		out.Systemd = override.Systemd
	}
	if len(override.SystemdUser) > 0 {
		out.SystemdUser = override.SystemdUser
	}
	if len(override.Bootstrap) > 0 {
		out.Bootstrap = override.Bootstrap
	}
	if len(override.AgentInstall) > 0 {
		out.AgentInstall = override.AgentInstall
	}
	if len(override.Firewall) > 0 {
		out.Firewall = override.Firewall
	}
	if len(override.Commands) > 0 {
		out.Commands = override.Commands
	}
	return out
}

func composeManifest(repoRoot, manifestRel string) (models.State, error) {
	merged, err := resolveManifestChain(repoRoot, manifestRel, map[string]struct{}{})
	if err != nil {
		return models.State{}, err
	}
	if len(merged.Modules) == 0 && len(merged.Overrides) == 0 && len(merged.Applications) == 0 {
		return models.State{}, fmt.Errorf("manifest %q: no modules, applications, or overrides", manifestRel)
	}

	manifestDir := filepath.Dir(manifestRel)
	modulePaths, err := resolveModuleRefs(repoRoot, manifestDir, merged.Modules)
	if err != nil {
		return models.State{}, fmt.Errorf("manifest %q: %w", manifestRel, err)
	}

	var configs []models.Configuration
	var diagnostics []models.Diagnostic
	schemaVersion := 0
	seen := map[string]struct{}{}
	for _, modulePath := range modulePaths {
		state, err := loadModuleState(repoRoot, modulePath)
		if err != nil {
			return models.State{}, err
		}
		diagnostics = append(diagnostics, state.Diagnostics...)
		if state.SchemaVersion > schemaVersion {
			schemaVersion = state.SchemaVersion
		}
		for _, cfg := range state.Configurations {
			name := strings.TrimSpace(cfg.Name)
			if name == "" {
				return models.State{}, fmt.Errorf("module %q: configuration missing name", modulePath)
			}
			if _, dup := seen[name]; dup {
				return models.State{}, fmt.Errorf("duplicate configuration %q (module %q)", name, modulePath)
			}
			seen[name] = struct{}{}
			configs = append(configs, cfg)
		}
	}

	configs, err = mergeConfigurations(configs, merged.Overrides)
	if err != nil {
		return models.State{}, fmt.Errorf("manifest %q: %w", manifestRel, err)
	}

	state := models.State{SchemaVersion: schemaVersion, Configurations: configs, Diagnostics: diagnostics}
	state, err = mergeApplicationsFromRefs(repoRoot, manifestDir, merged.Applications, state)
	if err != nil {
		return models.State{}, err
	}
	if len(state.Configurations) == 0 {
		return models.State{}, fmt.Errorf("manifest %q: composed state is empty", manifestRel)
	}
	return state, nil
}

func marshalState(state models.State) ([]byte, error) {
	return resourceregistry.MarshalCanonical(state)
}

func manifestExtendsFleet(repoRoot, manifestRel, fleet string) (bool, error) {
	fleetManifest, err := FindManifestInTree(repoRoot, filepath.Join("fleets", fleet))
	if err != nil {
		return false, err
	}
	target := normalizeRelPath(fleetManifest)
	seen := map[string]struct{}{}
	for cur := normalizeRelPath(manifestRel); cur != ""; {
		if cur == target {
			return true, nil
		}
		if _, ok := seen[cur]; ok {
			return false, fmt.Errorf("extends cycle at %q", cur)
		}
		seen[cur] = struct{}{}
		m, err := loadManifest(repoRoot, cur)
		if err != nil {
			return false, err
		}
		ext := strings.TrimSpace(m.Extends)
		if ext == "" {
			return false, nil
		}
		cur = normalizeRelPath(ext)
	}
	return false, nil
}
