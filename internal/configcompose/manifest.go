package configcompose

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/safepath"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

const maxExtendsDepth = 32

// Manifest is the source document for composing a desired.yaml artifact.
type Manifest struct {
	Extends       string                 `yaml:"extends,omitempty"`
	Modules       []string               `yaml:"modules,omitempty"`
	Applications  *ApplicationsSource    `yaml:"applications,omitempty"`
	Overrides     []models.Configuration `yaml:"overrides,omitempty"`
}

func parseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func loadManifest(repoRoot, relPath string) (Manifest, error) {
	data, err := readRepoRelative(repoRoot, relPath)
	if err != nil {
		return Manifest{}, err
	}
	return parseManifest(data)
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
		return Manifest{}, fmt.Errorf("load manifest %q: %w", manifestRel, err)
	}

	var merged Manifest
	if ext := strings.TrimSpace(m.Extends); ext != "" {
		parent, err := resolveManifestChain(repoRoot, ext, seen)
		if err != nil {
			return Manifest{}, err
		}
		merged.Modules = append(merged.Modules, parent.Modules...)
		merged.Overrides = append(merged.Overrides, parent.Overrides...)
	}
	merged.Modules = append(merged.Modules, m.Modules...)
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
	state, err := models.ParseState(bytes.NewReader(data))
	if err != nil {
		return models.State{}, fmt.Errorf("parse module %q: %w", modulePath, err)
	}
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
	if len(merged.Modules) == 0 && len(merged.Overrides) == 0 {
		appManifest, hasApps, err := resolveApplicationsSource(repoRoot, manifestRel, nil)
		if err != nil {
			return models.State{}, err
		}
		if !hasApps || len(appManifest.Modules) == 0 {
			return models.State{}, fmt.Errorf("manifest %q: no modules or overrides", manifestRel)
		}
	}

	var configs []models.Configuration
	seen := map[string]struct{}{}
	for _, modulePath := range merged.Modules {
		modulePath = normalizeRelPath(modulePath)
		if modulePath == "" {
			continue
		}
		state, err := loadModuleState(repoRoot, modulePath)
		if err != nil {
			return models.State{}, err
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

	state := models.State{Configurations: configs}
	state, err = mergeApplicationsIntoState(repoRoot, manifestRel, state)
	if err != nil {
		return models.State{}, err
	}
	if len(state.Configurations) == 0 {
		return models.State{}, fmt.Errorf("manifest %q: composed state is empty", manifestRel)
	}
	return state, nil
}

func marshalState(state models.State) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(state); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func desiredPathForManifest(manifestRel string) string {
	dir := filepath.Dir(manifestRel)
	return filepath.ToSlash(filepath.Join(dir, "desired.yaml"))
}

func manifestExtendsFleet(repoRoot, manifestRel, fleet string) (bool, error) {
	target := normalizeRelPath(filepath.Join("fleets", fleet, "manifest.yaml"))
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
