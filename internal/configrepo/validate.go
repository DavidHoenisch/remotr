package configrepo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"gopkg.in/yaml.v3"
)

// ValidationIssue is one problem found in a configuration repository.
type ValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationResult summarizes a repository validation run.
type ValidationResult struct {
	RepoRoot string            `json:"repo_root"`
	OK       []string          `json:"ok,omitempty"`
	Issues   []ValidationIssue `json:"issues,omitempty"`
}

// ValidateRepository checks fleet and endpoint desired.yaml artifacts under repoRoot.
func ValidateRepository(repoRoot string) (ValidationResult, error) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return ValidationResult{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("repository: %w", err)
	}
	if !info.IsDir() {
		return ValidationResult{}, fmt.Errorf("repository: %s is not a directory", abs)
	}

	res := ValidationResult{RepoRoot: abs}
	validateManifest(abs, &res)
	validateFleets(abs, &res)
	validateEndpoints(abs, &res)

	if len(res.OK) == 0 && len(res.Issues) == 0 {
		res.Issues = append(res.Issues, ValidationIssue{
			Path:    abs,
			Message: "no fleet artifacts found under fleets/<fleet>/desired.yaml",
		})
	}
	return res, nil
}

func relPath(repoRoot, path string) string {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	return rel
}

func validateManifest(repoRoot string, res *ValidationResult) {
	path := filepath.Join(repoRoot, "remotr.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		res.Issues = append(res.Issues, ValidationIssue{Path: path, Message: err.Error()})
		return
	}
	var manifest struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		res.Issues = append(res.Issues, ValidationIssue{Path: path, Message: fmt.Sprintf("parse manifest: %v", err)})
		return
	}
	if manifest.Kind != "" && manifest.Kind != "remotr-config-repo" {
		res.Issues = append(res.Issues, ValidationIssue{
			Path:    path,
			Message: fmt.Sprintf("unexpected kind %q (want remotr-config-repo)", manifest.Kind),
		})
		return
	}
	res.OK = append(res.OK, relPath(repoRoot, path))
}

func validateFleets(repoRoot string, res *ValidationResult) {
	fleetsDir := filepath.Join(repoRoot, "fleets")
	entries, err := os.ReadDir(fleetsDir)
	if err != nil {
		if os.IsNotExist(err) {
			res.Issues = append(res.Issues, ValidationIssue{
				Path:    fleetsDir,
				Message: "missing fleets/ directory",
			})
			return
		}
		res.Issues = append(res.Issues, ValidationIssue{Path: fleetsDir, Message: err.Error()})
		return
	}

	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		fleet := ent.Name()
		rel := filepath.Join("fleets", fleet, "desired.yaml")
		path := filepath.Join(repoRoot, rel)
		if err := ValidateFleetName(fleet); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
			continue
		}
		if err := validateDesiredFile(path, rel); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
			continue
		}
		res.OK = append(res.OK, rel)
	}
}

func validateEndpoints(repoRoot string, res *ValidationResult) {
	endpointsDir := filepath.Join(repoRoot, "endpoints")
	entries, err := os.ReadDir(endpointsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		res.Issues = append(res.Issues, ValidationIssue{Path: endpointsDir, Message: err.Error()})
		return
	}

	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		endpointID := ent.Name()
		rel := filepath.Join("endpoints", endpointID, "desired.yaml")
		path := filepath.Join(repoRoot, rel)
		if err := ValidateEndpointID(endpointID); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
			continue
		}
		if err := validateDesiredFile(path, rel); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
			continue
		}
		res.OK = append(res.OK, rel)
	}
}

func validateDesiredFile(path, displayPath string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing desired.yaml")
		}
		return err
	}
	state, err := models.ParseState(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse artifact: %w", err)
	}
	return validateState(state, displayPath)
}

func validateState(state models.State, path string) error {
	if len(state.Configurations) == 0 {
		return fmt.Errorf("no configurations defined")
	}
	seen := make(map[string]struct{}, len(state.Configurations))
	for _, cfg := range state.Configurations {
		name := strings.TrimSpace(cfg.Name)
		if name == "" {
			return fmt.Errorf("configuration missing name")
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("duplicate configuration %q", name)
		}
		seen[name] = struct{}{}

		if err := validatePackages(cfg, name); err != nil {
			return err
		}
		if err := validateFiles(cfg, name); err != nil {
			return err
		}
		if err := validateUsers(cfg, name); err != nil {
			return err
		}
		if err := validateSystemd(cfg, name); err != nil {
			return err
		}
		if err := validateCommands(cfg, name); err != nil {
			return err
		}
	}
	return nil
}

func validatePackages(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, pkg := range cfg.Packages {
		if strings.TrimSpace(pkg.Name) == "" {
			return fmt.Errorf("configuration %q: package missing name", cfgName)
		}
		key := packageResourceKey(pkg.Name, string(pkg.PM))
		if _, dup := seen[key]; dup {
			if pkg.PM != "" {
				return fmt.Errorf("configuration %q: duplicate package %q (packageManager %q)", cfgName, pkg.Name, pkg.PM)
			}
			return fmt.Errorf("configuration %q: duplicate package %q", cfgName, pkg.Name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// packageResourceKey distinguishes packages that share a name but target different backends.
func packageResourceKey(name, packageManager string) string {
	return name + "\x00" + packageManager
}

func validateFiles(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, f := range cfg.Files {
		if strings.TrimSpace(f.Name) == "" {
			return fmt.Errorf("configuration %q: file resource missing name", cfgName)
		}
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate file %q", cfgName, f.Name)
		}
		seen[f.Name] = struct{}{}
	}
	return nil
}

func validateUsers(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, u := range cfg.Users {
		if strings.TrimSpace(u.Name) == "" {
			return fmt.Errorf("configuration %q: user resource missing name", cfgName)
		}
		if _, dup := seen[u.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate user %q", cfgName, u.Name)
		}
		seen[u.Name] = struct{}{}
	}
	return nil
}

func validateSystemd(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, s := range cfg.Systemd {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("configuration %q: systemd resource missing name", cfgName)
		}
		if _, dup := seen[s.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate systemd resource %q", cfgName, s.Name)
		}
		seen[s.Name] = struct{}{}
	}
	return nil
}

func validateCommands(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, c := range cfg.Commands {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("configuration %q: command resource missing name", cfgName)
		}
		if _, dup := seen[c.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate command %q", cfgName, c.Name)
		}
		seen[c.Name] = struct{}{}
	}
	return nil
}
