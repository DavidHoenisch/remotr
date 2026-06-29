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

type applicationConfigMeta struct {
	Name          string               `yaml:"name"`
	Description   string               `yaml:"description,omitempty"`
	TargetDistros []types.Distro       `yaml:"targetDistros,omitempty"`
	TargetArch    []types.Architecture `yaml:"targetArch,omitempty"`
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
		return applicationModuleParse{}, fmt.Errorf("application must be a mapping")
	}

	var kindHead struct {
		Kind types.Kind `yaml:"kind"`
	}
	if err := doc.Decode(&kindHead); err != nil {
		return applicationModuleParse{}, err
	}
	if kindHead.Kind != types.KindApplication {
		return applicationModuleParse{}, fmt.Errorf("want kind %s, got %q", types.KindApplication, kindHead.Kind)
	}

	if mappingHasKey(doc, "packages") {
		var bundle struct {
			TargetDistros []types.Distro           `yaml:"targetDistros,omitempty"`
			TargetArch    []types.Architecture     `yaml:"targetArch,omitempty"`
			Configuration *applicationConfigMeta   `yaml:"configuration,omitempty"`
			Packages      []models.Package         `yaml:"packages"`
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
		return applicationModuleParse{}, fmt.Errorf("read application %q: %w", modulePath, err)
	}
	out, err := parseApplicationModule(data, modulePath)
	if err != nil {
		return applicationModuleParse{}, fmt.Errorf("parse application %q: %w", modulePath, err)
	}
	return out, nil
}

func mergeApplicationsFromRefs(repoRoot, manifestDir string, refs []string, state models.State) (models.State, error) {
	if len(refs) == 0 {
		return state, nil
	}
	appConfigs, err := composeApplicationsFromRefs(repoRoot, manifestDir, refs)
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
			return models.State{}, fmt.Errorf("application configuration %q conflicts with existing slice", name)
		}
		seen[name] = struct{}{}
	}
	state.Configurations = append(state.Configurations, appConfigs...)
	return state, nil
}

func composeApplicationsFromRefs(repoRoot, manifestDir string, refs []string) ([]models.Configuration, error) {
	modulePaths, err := resolveApplicationRefs(repoRoot, manifestDir, refs)
	if err != nil {
		return nil, err
	}

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
				return nil, fmt.Errorf("application %q: dedicated configuration missing name", modulePath)
			}
			if _, dup := seenCfg[name]; dup {
				return nil, fmt.Errorf("duplicate application configuration %q (application %q)", name, modulePath)
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
			if err := trackPackageName(seenSharedPkg, pkg.Name, modulePath); err != nil {
				return nil, err
			}
			sharedPackages = append(sharedPackages, pkg)
		}
	}

	var out []models.Configuration
	if len(sharedPackages) > 0 {
		if _, exists := seenCfg[applicationsSliceName]; exists {
			return nil, fmt.Errorf("configuration name %q conflicts with composed slice", applicationsSliceName)
		}
		out = append(out, models.Configuration{
			Name:        applicationsSliceName,
			Description: "Composed from application references",
			Packages:    sharedPackages,
		})
	}
	out = append(out, dedicated...)
	if len(out) == 0 {
		return nil, fmt.Errorf("composed applications are empty")
	}
	return out, nil
}

func trackDedicatedPackageName(seen map[string]struct{}, configName, pkgName, modulePath string) error {
	pkgName = strings.TrimSpace(pkgName)
	if pkgName == "" {
		return fmt.Errorf("application %q: package missing name", modulePath)
	}
	key := strings.TrimSpace(configName) + "/" + pkgName
	if _, dup := seen[key]; dup {
		return fmt.Errorf("duplicate application package %q in configuration %q (application %q)", pkgName, configName, modulePath)
	}
	seen[key] = struct{}{}
	return nil
}

func trackPackageName(seen map[string]struct{}, name, modulePath string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("application %q: package missing name", modulePath)
	}
	if _, dup := seen[name]; dup {
		return fmt.Errorf("duplicate application package %q (application %q)", name, modulePath)
	}
	seen[name] = struct{}{}
	return nil
}
