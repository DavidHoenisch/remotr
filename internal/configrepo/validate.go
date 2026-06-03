package configrepo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		if err := validateDownloads(cfg, name); err != nil {
			return err
		}
		if err := validateUsers(cfg, name); err != nil {
			return err
		}
		if err := validateSystemd(cfg, name); err != nil {
			return err
		}
		if err := validateSystemdUser(cfg, name); err != nil {
			return err
		}
		if err := validateBootstrap(cfg, name); err != nil {
			return err
		}
		if err := validateAgentInstall(cfg, name); err != nil {
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
		path := strings.TrimSpace(f.Path)
		if path == "" {
			return fmt.Errorf("configuration %q: file %q missing path", cfgName, f.Name)
		}
		if !filepath.IsAbs(filepath.Clean(path)) {
			return fmt.Errorf("configuration %q: file %q path must be absolute", cfgName, f.Name)
		}
		if f.UpdateExisting && strings.TrimSpace(f.WithRegx) != "" && strings.TrimSpace(f.Content) == "" {
			return fmt.Errorf("configuration %q: file %q line edit requires content", cfgName, f.Name)
		}
		if f.UpdateExisting && strings.TrimSpace(f.WithRegx) != "" {
			if _, err := regexp.Compile(strings.TrimSpace(f.WithRegx)); err != nil {
				return fmt.Errorf("configuration %q: file %q invalid withRegx: %w", cfgName, f.Name, err)
			}
		}
		if rep := strings.TrimSpace(f.ReplaceRegx); rep != "" {
			if _, err := regexp.Compile(rep); err != nil {
				return fmt.Errorf("configuration %q: file %q invalid replaceRegx: %w", cfgName, f.Name, err)
			}
		}
	}
	return nil
}

func validateDownloads(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, d := range cfg.Downloads {
		if strings.TrimSpace(d.Name) == "" {
			return fmt.Errorf("configuration %q: download resource missing name", cfgName)
		}
		if _, dup := seen[d.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate download %q", cfgName, d.Name)
		}
		seen[d.Name] = struct{}{}
		if strings.TrimSpace(d.URL) == "" {
			return fmt.Errorf("configuration %q: download %q missing url", cfgName, d.Name)
		}
		dest := strings.TrimSpace(d.Dest)
		if dest == "" {
			return fmt.Errorf("configuration %q: download %q missing dest", cfgName, d.Name)
		}
		if !filepath.IsAbs(filepath.Clean(dest)) {
			return fmt.Errorf("configuration %q: download %q dest must be absolute", cfgName, d.Name)
		}
		if strings.Contains(filepath.Clean(dest), "..") {
			return fmt.Errorf("configuration %q: download %q invalid dest path", cfgName, d.Name)
		}
		if d.Checksum != "" {
			if _, err := parseDownloadChecksum(d.Checksum); err != nil {
				return fmt.Errorf("configuration %q: download %q: %w", cfgName, d.Name, err)
			}
		}
	}
	return nil
}

func parseDownloadChecksum(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "sha256:") {
		return "", fmt.Errorf("checksum must be sha256:<hex>")
	}
	hexPart := strings.TrimPrefix(s, "sha256:")
	if len(hexPart) != 64 {
		return "", fmt.Errorf("checksum hex must be 64 characters")
	}
	for _, c := range hexPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return "", fmt.Errorf("invalid checksum hex")
		}
	}
	return strings.ToLower(hexPart), nil
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

func validateSystemdUser(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, s := range cfg.SystemdUser {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("configuration %q: systemdUser resource missing name", cfgName)
		}
		if strings.TrimSpace(s.Unit) == "" {
			return fmt.Errorf("configuration %q: systemdUser resource %q missing unit", cfgName, s.Name)
		}
		users := strings.TrimSpace(s.Users)
		if users == "" {
			return fmt.Errorf("configuration %q: systemdUser resource %q missing users", cfgName, s.Name)
		}
		if users != "interactive" {
			return fmt.Errorf("configuration %q: systemdUser resource %q: users must be %q (got %q)", cfgName, s.Name, "interactive", s.Users)
		}
		if _, dup := seen[s.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate systemdUser resource %q", cfgName, s.Name)
		}
		seen[s.Name] = struct{}{}
	}
	return nil
}

func validateBootstrap(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, b := range cfg.Bootstrap {
		if strings.TrimSpace(b.Name) == "" {
			return fmt.Errorf("configuration %q: bootstrap resource missing name", cfgName)
		}
		if _, dup := seen[b.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate bootstrap resource %q", cfgName, b.Name)
		}
		seen[b.Name] = struct{}{}

		pathMissing := strings.TrimSpace(b.When.PathMissing)
		pathExists := strings.TrimSpace(b.When.PathExists)
		if pathMissing == "" && pathExists == "" {
			return fmt.Errorf("configuration %q: bootstrap %q: when requires pathMissing or pathExists", cfgName, b.Name)
		}
		if pathMissing != "" && pathExists != "" {
			return fmt.Errorf("configuration %q: bootstrap %q: when must set only one of pathMissing or pathExists", cfgName, b.Name)
		}
		if len(b.Steps) == 0 {
			return fmt.Errorf("configuration %q: bootstrap %q: at least one step required", cfgName, b.Name)
		}
		for i, step := range b.Steps {
			hasSystemd := step.Systemd != nil
			hasExec := len(step.Exec) > 0
			if hasSystemd == hasExec {
				return fmt.Errorf("configuration %q: bootstrap %q: step %d must set exactly one of systemd or exec", cfgName, b.Name, i+1)
			}
			if hasSystemd {
				if strings.TrimSpace(step.Systemd.Unit) == "" {
					return fmt.Errorf("configuration %q: bootstrap %q: step %d: systemd unit required", cfgName, b.Name, i+1)
				}
				if step.Systemd.Enabled == nil && step.Systemd.Active == nil {
					return fmt.Errorf("configuration %q: bootstrap %q: step %d: systemd requires enabled and/or active", cfgName, b.Name, i+1)
				}
			}
			if hasExec && strings.TrimSpace(step.Exec[0]) == "" {
				return fmt.Errorf("configuration %q: bootstrap %q: step %d: exec command required", cfgName, b.Name, i+1)
			}
		}
	}
	return nil
}

func validateAgentInstall(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, ag := range cfg.AgentInstall {
		if strings.TrimSpace(ag.Name) == "" {
			return fmt.Errorf("configuration %q: agentInstall resource missing name", cfgName)
		}
		if _, dup := seen[ag.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate agentInstall %q", cfgName, ag.Name)
		}
		seen[ag.Name] = struct{}{}
		if strings.TrimSpace(ag.Version) == "" {
			return fmt.Errorf("configuration %q: agentInstall %q missing version", cfgName, ag.Name)
		}
		if strings.TrimSpace(ag.ArtifactURL) == "" {
			return fmt.Errorf("configuration %q: agentInstall %q missing artifactURL", cfgName, ag.Name)
		}
		if strings.TrimSpace(ag.ExtractDir) == "" {
			return fmt.Errorf("configuration %q: agentInstall %q missing extractDir", cfgName, ag.Name)
		}
		if strings.TrimSpace(ag.FleetURL) == "" {
			return fmt.Errorf("configuration %q: agentInstall %q missing fleetURL", cfgName, ag.Name)
		}
		sec := strings.TrimSpace(ag.EnrollmentTokenSecret)
		if sec == "" {
			return fmt.Errorf("configuration %q: agentInstall %q missing enrollmentTokenSecret", cfgName, ag.Name)
		}
		if !strings.HasPrefix(sec, "file:") {
			return fmt.Errorf("configuration %q: agentInstall %q: enrollmentTokenSecret must be file:/absolute/path", cfgName, ag.Name)
		}
		path := strings.TrimSpace(sec[len("file:"):])
		if !filepath.IsAbs(filepath.Clean(path)) {
			return fmt.Errorf("configuration %q: agentInstall %q: enrollment token path must be absolute", cfgName, ag.Name)
		}
		if strings.TrimSpace(ag.RunningCheck.Process) == "" {
			return fmt.Errorf("configuration %q: agentInstall %q: runningCheck.process required", cfgName, ag.Name)
		}
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
