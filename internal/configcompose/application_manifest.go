package configcompose

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

const applicationsSliceName = "applications"

// ApplicationManifest is the source document for composing application packages into desired.yaml.
type ApplicationManifest struct {
	Extends   string           `yaml:"extends,omitempty"`
	Modules   []string         `yaml:"modules,omitempty"`
	Overrides []models.Package `yaml:"overrides,omitempty"`
	Mode      string           `yaml:"mode,omitempty"`
}

// ApplicationsSource is the flexible applications field on manifest.yaml.
type ApplicationsSource struct {
	ManifestPath string
	Inline       ApplicationManifest
}

func (a *ApplicationsSource) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		a.ManifestPath = strings.TrimSpace(value.Value)
		return nil
	case yaml.SequenceNode:
		var modules []string
		if err := value.Decode(&modules); err != nil {
			return err
		}
		a.Inline.Modules = modules
		return nil
	case yaml.MappingNode:
		return value.Decode(&a.Inline)
	default:
		return fmt.Errorf("applications must be a path, module list, or manifest object")
	}
}

type applicationConfigMeta struct {
	Name          string               `yaml:"name"`
	Description   string               `yaml:"description,omitempty"`
	TargetDistros []types.Distro       `yaml:"targetDistros,omitempty"`
	TargetArch    []types.Architecture `yaml:"targetArch,omitempty"`
}

func parseApplicationManifest(data []byte) (ApplicationManifest, error) {
	var m ApplicationManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return ApplicationManifest{}, err
	}
	return m, nil
}

func loadApplicationManifest(repoRoot, relPath string) (ApplicationManifest, error) {
	data, err := readRepoRelative(repoRoot, relPath)
	if err != nil {
		return ApplicationManifest{}, err
	}
	return parseApplicationManifest(data)
}

func resolveApplicationManifestChain(repoRoot, manifestRel string, seen map[string]struct{}) (ApplicationManifest, error) {
	manifestRel = normalizeRelPath(manifestRel)
	if manifestRel == "" {
		return ApplicationManifest{}, fmt.Errorf("empty applications manifest path")
	}
	if _, ok := seen[manifestRel]; ok {
		return ApplicationManifest{}, fmt.Errorf("extends cycle at %q", manifestRel)
	}
	if len(seen) >= maxExtendsDepth {
		return ApplicationManifest{}, fmt.Errorf("extends depth exceeds %d at %q", maxExtendsDepth, manifestRel)
	}
	seen[manifestRel] = struct{}{}

	m, err := loadApplicationManifest(repoRoot, manifestRel)
	if err != nil {
		return ApplicationManifest{}, fmt.Errorf("load applications manifest %q: %w", manifestRel, err)
	}

	var merged ApplicationManifest
	if ext := strings.TrimSpace(m.Extends); ext != "" {
		parent, err := resolveApplicationManifestChain(repoRoot, ext, seen)
		if err != nil {
			return ApplicationManifest{}, err
		}
		merged.Modules = append(merged.Modules, parent.Modules...)
		merged.Overrides = append(merged.Overrides, parent.Overrides...)
		if strings.TrimSpace(parent.Mode) != "" {
			merged.Mode = parent.Mode
		}
	}
	merged.Modules = append(merged.Modules, m.Modules...)
	merged.Overrides = append(merged.Overrides, m.Overrides...)
	if strings.TrimSpace(m.Mode) != "" {
		merged.Mode = m.Mode
	}
	return merged, nil
}

func applicationsManifestPathForManifest(manifestRel string) string {
	dir := filepath.Dir(manifestRel)
	return filepath.ToSlash(filepath.Join(dir, "applications.manifest.yaml"))
}

func resolveApplicationsSource(repoRoot, manifestRel string, inheritSeen map[string]struct{}) (ApplicationManifest, bool, error) {
	m, err := loadManifest(repoRoot, manifestRel)
	if err != nil {
		return ApplicationManifest{}, false, err
	}

	if m.Applications != nil {
		src := *m.Applications
		if path := strings.TrimSpace(src.ManifestPath); path != "" {
			merged, err := resolveApplicationManifestChain(repoRoot, path, map[string]struct{}{})
			return merged, true, err
		}
		if len(src.Inline.Modules) > 0 || len(src.Inline.Overrides) > 0 || strings.TrimSpace(src.Inline.Extends) != "" {
			merged, err := resolveInlineApplicationManifest(repoRoot, src.Inline)
			return merged, true, err
		}
	}

	sibling := applicationsManifestPathForManifest(manifestRel)
	if fileExists(filepath.Join(repoRoot, filepath.FromSlash(sibling))) {
		merged, err := resolveApplicationManifestChain(repoRoot, sibling, map[string]struct{}{})
		return merged, true, err
	}

	if ext := strings.TrimSpace(m.Extends); ext != "" {
		if inheritSeen == nil {
			inheritSeen = map[string]struct{}{}
		}
		cur := normalizeRelPath(manifestRel)
		if _, ok := inheritSeen[cur]; ok {
			return ApplicationManifest{}, false, fmt.Errorf("extends cycle at %q", cur)
		}
		inheritSeen[cur] = struct{}{}
		return resolveApplicationsSource(repoRoot, ext, inheritSeen)
	}

	return ApplicationManifest{}, false, nil
}

func resolveInlineApplicationManifest(repoRoot string, inline ApplicationManifest) (ApplicationManifest, error) {
	if ext := strings.TrimSpace(inline.Extends); ext != "" {
		return resolveApplicationManifestChain(repoRoot, ext, map[string]struct{}{})
	}
	return inline, nil
}

type applicationModuleParse struct {
	dedicated *models.Configuration
	packages  []models.Package
}

func parseApplicationModule(data []byte, modulePath string) (applicationModuleParse, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return applicationModuleParse{}, err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return applicationModuleParse{}, fmt.Errorf("empty document")
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return applicationModuleParse{}, fmt.Errorf("application module must be a mapping")
	}

	if mappingHasKey(doc, "packages") {
		var bundle struct {
			TargetDistros []types.Distro       `yaml:"targetDistros,omitempty"`
			TargetArch    []types.Architecture `yaml:"targetArch,omitempty"`
			Configuration *applicationConfigMeta `yaml:"configuration,omitempty"`
			Packages      []models.Package       `yaml:"packages"`
		}
		if err := doc.Decode(&bundle); err != nil {
			return applicationModuleParse{}, err
		}
		if len(bundle.Packages) == 0 {
			return applicationModuleParse{}, fmt.Errorf("packages list is empty")
		}
		return buildApplicationModuleResult(bundle.Configuration, bundle.TargetDistros, bundle.TargetArch, bundle.Packages, modulePath)
	}

	var wrapped struct {
		TargetDistros []types.Distro         `yaml:"targetDistros,omitempty"`
		TargetArch    []types.Architecture   `yaml:"targetArch,omitempty"`
		Configuration *applicationConfigMeta `yaml:"configuration,omitempty"`
	}
	if err := doc.Decode(&wrapped); err != nil {
		return applicationModuleParse{}, err
	}

	var pkg models.Package
	if err := doc.Decode(&pkg); err != nil {
		return applicationModuleParse{}, err
	}
	if strings.TrimSpace(pkg.Name) == "" {
		return applicationModuleParse{}, fmt.Errorf("application missing name")
	}
	return buildApplicationModuleResult(wrapped.Configuration, wrapped.TargetDistros, wrapped.TargetArch, []models.Package{pkg}, modulePath)
}

func buildApplicationModuleResult(
	cfgMeta *applicationConfigMeta,
	fileDistros []types.Distro,
	fileArch []types.Architecture,
	packages []models.Package,
	modulePath string,
) (applicationModuleParse, error) {
	distros := fileDistros
	arch := fileArch
	desc := ""
	name := applicationsSliceName + "/" + strings.TrimSuffix(filepath.Base(modulePath), filepath.Ext(modulePath))
	if cfgMeta != nil {
		if len(cfgMeta.TargetDistros) > 0 {
			distros = cfgMeta.TargetDistros
		}
		if len(cfgMeta.TargetArch) > 0 {
			arch = cfgMeta.TargetArch
		}
		if n := strings.TrimSpace(cfgMeta.Name); n != "" {
			name = n
		}
		desc = strings.TrimSpace(cfgMeta.Description)
	}
	if cfgMeta == nil && len(distros) == 0 && len(arch) == 0 {
		return applicationModuleParse{packages: packages}, nil
	}
	if cfgMeta == nil && len(packages) == 1 {
		name = applicationsSliceName + "/" + strings.TrimSpace(packages[0].Name)
	}
	return applicationModuleParse{
		dedicated: &models.Configuration{
			Name:          name,
			Description:   desc,
			TargetDistros: distros,
			TargetArch:    arch,
			Packages:      packages,
		},
	}, nil
}

func mappingHasKey(node *yaml.Node, key string) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

func loadApplicationModule(repoRoot, modulePath string) (applicationModuleParse, error) {
	data, err := readRepoRelative(repoRoot, modulePath)
	if err != nil {
		return applicationModuleParse{}, fmt.Errorf("read application module %q: %w", modulePath, err)
	}
	out, err := parseApplicationModule(data, modulePath)
	if err != nil {
		return applicationModuleParse{}, fmt.Errorf("parse application module %q: %w", modulePath, err)
	}
	return out, nil
}

func mergePackages(base []models.Package, overrides []models.Package) ([]models.Package, error) {
	byName := make(map[string]int, len(base))
	out := append([]models.Package(nil), base...)
	for i, pkg := range out {
		name := strings.TrimSpace(pkg.Name)
		if name == "" {
			return nil, fmt.Errorf("application package missing name")
		}
		byName[name] = i
	}

	for _, override := range overrides {
		name := strings.TrimSpace(override.Name)
		if name == "" {
			return nil, fmt.Errorf("application override missing name")
		}
		idx, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("application override %q: no matching package", name)
		}
		out[idx] = mergePackage(out[idx], override)
	}
	return out, nil
}

func mergePackage(base, override models.Package) models.Package {
	out := base
	if n := strings.TrimSpace(override.Name); n != "" {
		out.Name = n
	}
	if override.PM != "" || override.PWAURL != "" || override.FlatpakRemote != "" || override.FlatpakRemoteURL != "" {
		out.Present = override.Present
	}
	if v := strings.TrimSpace(override.Version); v != "" {
		out.Version = v
	}
	if override.Arch != "" {
		out.Arch = override.Arch
	}
	if override.PM != "" {
		out.PM = override.PM
	}
	if r := strings.TrimSpace(override.FlatpakRemote); r != "" {
		out.FlatpakRemote = r
	}
	if u := strings.TrimSpace(override.FlatpakRemoteURL); u != "" {
		out.FlatpakRemoteURL = u
	}
	if u := strings.TrimSpace(override.PWAURL); u != "" {
		out.PWAURL = u
	}
	if t := strings.TrimSpace(override.PWATitle); t != "" {
		out.PWATitle = t
	}
	if i := strings.TrimSpace(override.PWAIcon); i != "" {
		out.PWAIcon = i
	}
	if b := strings.TrimSpace(override.PWABrowser); b != "" {
		out.PWABrowser = b
	}
	if u := strings.TrimSpace(override.PWAUsers); u != "" {
		out.PWAUsers = u
	}
	if len(override.DependsOn) > 0 {
		out.DependsOn = override.DependsOn
	}
	if len(override.PreApplyValidation) > 0 {
		out.PreApplyValidation = override.PreApplyValidation
	}
	return out
}

func perModuleMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "per-module")
}

func composeApplications(repoRoot, manifestRel string) ([]models.Configuration, error) {
	appManifest, ok, err := resolveApplicationsSource(repoRoot, manifestRel, nil)
	if err != nil {
		return nil, err
	}
	if !ok || len(appManifest.Modules) == 0 {
		return nil, nil
	}

	modulePaths, err := resolveApplicationModuleRefs(repoRoot, appManifest.Modules)
	if err != nil {
		return nil, fmt.Errorf("applications manifest for %q: %w", manifestRel, err)
	}

	perModule := perModuleMode(appManifest.Mode)
	var sharedPackages []models.Package
	var dedicated []models.Configuration
	seenSharedPkg := map[string]struct{}{}
	seenDedicatedPkg := map[string]struct{}{}
	seenCfg := map[string]struct{}{}

	for _, modulePath := range modulePaths {
		parsed, err := loadApplicationModule(repoRoot, modulePath)
		if err != nil {
			return nil, err
		}
		if parsed.dedicated != nil {
			cfg := *parsed.dedicated
			name := strings.TrimSpace(cfg.Name)
			if name == "" {
				return nil, fmt.Errorf("application module %q: dedicated configuration missing name", modulePath)
			}
			if _, dup := seenCfg[name]; dup {
				return nil, fmt.Errorf("duplicate application configuration %q (module %q)", name, modulePath)
			}
			seenCfg[name] = struct{}{}
			for _, pkg := range cfg.Packages {
				if err := trackDedicatedPackageName(seenDedicatedPkg, name, pkg.Name, modulePath); err != nil {
					return nil, err
				}
			}
			dedicated = append(dedicated, cfg)
			continue
		}
		for _, pkg := range parsed.packages {
			if perModule {
				name := applicationsSliceName + "/" + strings.TrimSpace(pkg.Name)
				if _, dup := seenCfg[name]; dup {
					return nil, fmt.Errorf("duplicate application configuration %q (module %q)", name, modulePath)
				}
				seenCfg[name] = struct{}{}
				if err := trackDedicatedPackageName(seenDedicatedPkg, name, pkg.Name, modulePath); err != nil {
					return nil, err
				}
				dedicated = append(dedicated, models.Configuration{
					Name:     name,
					Packages: []models.Package{pkg},
				})
				continue
			}
			if err := trackPackageName(seenSharedPkg, pkg.Name, modulePath); err != nil {
				return nil, err
			}
			sharedPackages = append(sharedPackages, pkg)
		}
	}

	if len(sharedPackages) > 0 {
		merged, err := mergePackages(sharedPackages, appManifest.Overrides)
		if err != nil {
			return nil, fmt.Errorf("applications manifest for %q: %w", manifestRel, err)
		}
		sharedPackages = merged
	} else if len(appManifest.Overrides) > 0 {
		return nil, fmt.Errorf("applications manifest for %q: overrides without shared application packages", manifestRel)
	}

	var out []models.Configuration
	if len(sharedPackages) > 0 {
		if _, exists := seenCfg[applicationsSliceName]; exists {
			return nil, fmt.Errorf("applications manifest for %q: configuration name %q conflicts with composed slice", manifestRel, applicationsSliceName)
		}
		out = append(out, models.Configuration{
			Name:        applicationsSliceName,
			Description: "Composed from applications manifest",
			Packages:    sharedPackages,
		})
	}
	out = append(out, dedicated...)
	if len(out) == 0 {
		return nil, fmt.Errorf("applications manifest for %q: composed applications are empty", manifestRel)
	}
	return out, nil
}

func trackDedicatedPackageName(seen map[string]struct{}, configName, pkgName, modulePath string) error {
	pkgName = strings.TrimSpace(pkgName)
	if pkgName == "" {
		return fmt.Errorf("application module %q: package missing name", modulePath)
	}
	key := strings.TrimSpace(configName) + "/" + pkgName
	if _, dup := seen[key]; dup {
		return fmt.Errorf("duplicate application package %q in configuration %q (module %q)", pkgName, configName, modulePath)
	}
	seen[key] = struct{}{}
	return nil
}

func trackPackageName(seen map[string]struct{}, name, modulePath string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("application module %q: package missing name", modulePath)
	}
	if _, dup := seen[name]; dup {
		return fmt.Errorf("duplicate application package %q (module %q)", name, modulePath)
	}
	seen[name] = struct{}{}
	return nil
}

func mergeApplicationsIntoState(repoRoot, manifestRel string, state models.State) (models.State, error) {
	appConfigs, err := composeApplications(repoRoot, manifestRel)
	if err != nil {
		return models.State{}, err
	}
	if len(appConfigs) == 0 {
		return state, nil
	}

	seen := map[string]struct{}{}
	for _, cfg := range state.Configurations {
		seen[strings.TrimSpace(cfg.Name)] = struct{}{}
	}
	for _, cfg := range appConfigs {
		name := strings.TrimSpace(cfg.Name)
		if _, dup := seen[name]; dup {
			return models.State{}, fmt.Errorf("manifest %q: application configuration %q conflicts with existing slice", manifestRel, name)
		}
		seen[name] = struct{}{}
	}

	state.Configurations = append(state.Configurations, appConfigs...)
	return state, nil
}
