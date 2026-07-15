package configrepo

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"net"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/flatpak"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

// ValidationIssue is one problem found in a configuration repository.
type ValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationResult summarizes a repository validation run.
type ValidationResult struct {
	RepoRoot    string                 `json:"repo_root"`
	OK          []string               `json:"ok,omitempty"`
	Issues      []ValidationIssue      `json:"issues,omitempty"`
	Diagnostics []ValidationDiagnostic `json:"diagnostics,omitempty"`
}

// ValidationDiagnostic is a non-fatal, source-located authoring notice.
type ValidationDiagnostic struct {
	Path    string                `json:"path"`
	Code    models.DiagnosticCode `json:"code"`
	Message string                `json:"message"`
}

// ValidateRepository checks kind-tagged config sources and composition under repoRoot.
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
	validateSharedModules(abs, &res)
	validateApplicationsDir(abs, &res)
	validateFleets(abs, &res)
	validateEndpoints(abs, &res)

	if len(res.OK) == 0 && len(res.Issues) == 0 {
		res.Issues = append(res.Issues, ValidationIssue{
			Path:    abs,
			Message: "no fleet manifests found under fleets/<fleet>/",
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
		fleetRel := filepath.Join("fleets", fleet)
		if err := ValidateFleetName(fleet); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: fleetRel, Message: err.Error()})
			continue
		}
		manifest, err := findManifestInTree(repoRoot, fleetRel)
		if err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: fleetRel, Message: err.Error()})
			continue
		}
		if err := validateManifestFile(repoRoot, manifest); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: manifest, Message: err.Error()})
			continue
		}
		res.OK = append(res.OK, manifest)
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
		epRel := filepath.Join("endpoints", endpointID)
		if err := ValidateEndpointID(endpointID); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: epRel, Message: err.Error()})
			continue
		}
		manifest, err := findManifestInTree(repoRoot, epRel)
		if err != nil {
			if strings.Contains(err.Error(), "no kind: manifest") {
				continue
			}
			res.Issues = append(res.Issues, ValidationIssue{Path: epRel, Message: err.Error()})
			continue
		}
		if err := validateManifestFile(repoRoot, manifest); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: manifest, Message: err.Error()})
			continue
		}
		res.OK = append(res.OK, manifest)
	}
}

// ValidateState checks a composed deployable artifact (no kind field required).
func ValidateState(state models.State, path string) error {
	return validateState(state, path)
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
		if err := validateAPTSigningKeys(cfg, name); err != nil {
			return err
		}
		if err := validateAPTRepositories(cfg, name); err != nil {
			return err
		}
		if err := validateSysctls(cfg, name); err != nil {
			return err
		}
		if err := validateHostnames(cfg, name); err != nil {
			return err
		}
		if err := validateHostLocales(cfg, name); err != nil {
			return err
		}
		if err := validateTimeSync(cfg, name); err != nil {
			return err
		}
		if err := validateMounts(cfg, name); err != nil {
			return err
		}
		if err := validateSwaps(cfg, name); err != nil {
			return err
		}
		if err := validateFiles(cfg, name); err != nil {
			return err
		}
		if err := validateDirectories(cfg, name); err != nil {
			return err
		}
		if err := validateLinks(cfg, name); err != nil {
			return err
		}
		if err := validateGroups(cfg, name); err != nil {
			return err
		}
		if err := validateAuthorizedKeys(cfg, name); err != nil {
			return err
		}
		if err := validateKnownHosts(cfg, name); err != nil {
			return err
		}
		if err := validateSudo(cfg, name); err != nil {
			return err
		}
		if err := validateUserFiles(cfg, name); err != nil {
			return err
		}
		if err := validateDownloads(cfg, name); err != nil {
			return err
		}
		if err := validateUsers(cfg, name); err != nil {
			return err
		}
		if err := validateEndpointSchedules(cfg, name); err != nil {
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
		if err := validateFirewall(cfg, name); err != nil {
			return err
		}
		if err := validateCertificates(cfg, name); err != nil {
			return err
		}
		if err := validateTrustAnchors(cfg, name); err != nil {
			return err
		}
		if err := validateAppArmorProfiles(cfg, name); err != nil {
			return err
		}
		if err := validateAuditRules(cfg, name); err != nil {
			return err
		}
		if err := validateAccountLimits(cfg, name); err != nil {
			return err
		}
		if err := validateLoginPolicies(cfg, name); err != nil {
			return err
		}
		if err := validateJournald(cfg, name); err != nil {
			return err
		}
		if err := validateLogrotate(cfg, name); err != nil {
			return err
		}
		if err := validateCommands(cfg, name); err != nil {
			return err
		}
	}
	if err := validateCapabilityMatrix(state); err != nil {
		return err
	}
	return validateResourceGraph(state)
}

func validateCertificates(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.Certificates))
	for _, resource := range cfg.Certificates {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: certificate %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate certificate %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateTrustAnchors(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.TrustAnchors))
	for _, resource := range cfg.TrustAnchors {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: trust anchor %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate trust anchor %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateAppArmorProfiles(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.AppArmorProfiles))
	for _, resource := range cfg.AppArmorProfiles {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: AppArmor profile %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate AppArmor profile %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateAuditRules(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.AuditRules))
	for _, resource := range cfg.AuditRules {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: audit rules %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate audit rules %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateAccountLimits(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.AccountLimits))
	for _, resource := range cfg.AccountLimits {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: account limits %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate account limits %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateLoginPolicies(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.LoginPolicies))
	for _, resource := range cfg.LoginPolicies {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: login policy %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate login policy %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateJournald(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.Journald))
	for _, resource := range cfg.Journald {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: journald policy %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate journald policy %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateLogrotate(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.Logrotate))
	for _, resource := range cfg.Logrotate {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: logrotate fragment %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate logrotate fragment %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateSwaps(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, r := range cfg.Swaps {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("configuration %q: swap %q: %w", cfgName, r.Name, err)
		}
		if _, ok := seen[r.Name]; ok {
			return fmt.Errorf("configuration %q: duplicate swap %q", cfgName, r.Name)
		}
		seen[r.Name] = struct{}{}
	}
	return nil
}

func validateEndpointSchedules(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.EndpointSchedules))
	for _, resource := range cfg.EndpointSchedules {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q resource %q: %w", cfgName, models.ResourceAddress(cfgName, resource.Name), err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate endpoint schedule %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateAPTSigningKeys(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.APTSigningKeys))
	for _, key := range cfg.APTSigningKeys {
		if err := key.Validate(); err != nil {
			return fmt.Errorf("configuration %q: APT signing key %q: %w", cfgName, key.Name, err)
		}
		if _, exists := seen[key.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate APT signing key %q", cfgName, key.Name)
		}
		seen[key.Name] = struct{}{}
	}
	return nil
}

func validateAPTRepositories(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.APTRepositories))
	for _, repository := range cfg.APTRepositories {
		if err := repository.Validate(); err != nil {
			return fmt.Errorf("configuration %q: APT repository %q: %w", cfgName, repository.Name, err)
		}
		if repository.Lifecycle != models.LifecycleAbsent {
			keyAddress := models.ResourceAddress(cfgName, repository.SigningKey)
			if !containsAddress(repository.DependsOn, keyAddress) {
				return fmt.Errorf("configuration %q: APT repository %q must explicitly depend on signing key %q", cfgName, repository.Name, keyAddress)
			}
		}
		if _, exists := seen[repository.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate APT repository %q", cfgName, repository.Name)
		}
		seen[repository.Name] = struct{}{}
	}
	return nil
}

func validateSysctls(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.Sysctls))
	for _, resource := range cfg.Sysctls {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: sysctl %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate sysctl %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateHostnames(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.Hostnames))
	for _, resource := range cfg.Hostnames {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: hostname %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate hostname %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateHostLocales(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.HostLocales))
	for _, resource := range cfg.HostLocales {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: host locale %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate host locale %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateTimeSync(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.TimeSync))
	for _, resource := range cfg.TimeSync {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: time sync %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate time sync %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func validateMounts(cfg models.Configuration, cfgName string) error {
	seen := make(map[string]struct{}, len(cfg.Mounts))
	for _, resource := range cfg.Mounts {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: mount %q: %w", cfgName, resource.Name, err)
		}
		if _, exists := seen[resource.Name]; exists {
			return fmt.Errorf("configuration %q: duplicate mount %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
	}
	return nil
}

func containsAddress(addresses []string, want string) bool {
	for _, address := range addresses {
		if address == want {
			return true
		}
	}
	return false
}

func validatePackages(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, pkg := range cfg.Packages {
		if strings.TrimSpace(pkg.Name) == "" {
			return fmt.Errorf("configuration %q: package missing name", cfgName)
		}
		if err := validatePackageFields(cfgName, pkg); err != nil {
			return err
		}
		key := packageResourceKey(pkg)
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

func validatePackageFields(cfgName string, pkg models.Package) error {
	if pkg.PM == types.Yay {
		return fmt.Errorf("configuration %q: package %q: packageManager yay is unsupported until an AUR provider is implemented", cfgName, pkg.Name)
	}
	if pkg.PM == types.Dnf {
		return fmt.Errorf("configuration %q resource %q: DNF provider is deferred to the RPM-family roadmap", cfgName, models.ResourceAddress(cfgName, pkg.Name))
	}
	if pkg.Hold != nil && pkg.PM != types.Apt {
		return fmt.Errorf("configuration %q: package %q: hold is unsupported by packageManager %s", cfgName, pkg.Name, pkg.PM)
	}
	if pkg.NonInteractive != nil && !*pkg.NonInteractive {
		return fmt.Errorf("configuration %q: package %q: interactive package transactions are unsupported", cfgName, pkg.Name)
	}
	switch pkg.PM {
	case types.Remotr:
		if strings.TrimSpace(pkg.Version) == "" {
			return fmt.Errorf("configuration %q: package %q with packageManager remotr requires version", cfgName, pkg.Name)
		}
		if err := apppackages.ValidateNameVersion(pkg.Name, pkg.Version); err != nil {
			return fmt.Errorf("configuration %q: package %q: %w", cfgName, pkg.Name, err)
		}
	case types.Flatpak:
		remote := strings.TrimSpace(pkg.FlatpakRemote)
		if remote == "" {
			remote = flatpak.DefaultRemote
		}
		if remote != flatpak.DefaultRemote && strings.TrimSpace(pkg.FlatpakRemoteURL) == "" {
			return fmt.Errorf("configuration %q: flatpak package %q with remote %q requires flatpakRemoteURL", cfgName, pkg.Name, remote)
		}
		if u := strings.TrimSpace(pkg.FlatpakRemoteURL); u != "" {
			parsed, err := url.Parse(u)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return fmt.Errorf("configuration %q: flatpak package %q has invalid flatpakRemoteURL", cfgName, pkg.Name)
			}
		}
	case types.Pwa:
		if err := validatePWAFields(cfgName, pkg); err != nil {
			return err
		}
	}
	if pkg.PM != types.Remotr && pkg.PM != types.Apt && pkg.PM != types.Pacman && strings.TrimSpace(pkg.Version) != "" {
		return fmt.Errorf("configuration %q: package %q: version is unsupported by packageManager %s", cfgName, pkg.Name, pkg.PM)
	}
	return nil
}

func packageResourceKey(pkg models.Package) string {
	pm := string(pkg.PM)
	if pkg.PM == types.Flatpak {
		remote := strings.TrimSpace(pkg.FlatpakRemote)
		if remote == "" {
			remote = flatpak.DefaultRemote
		}
		return pkg.Name + "\x00" + pm + "\x00" + remote
	}
	if pkg.PM == types.Remotr {
		return pkg.Name + "\x00" + pm + "\x00" + strings.TrimSpace(pkg.Version)
	}
	if pkg.PM == types.Pwa {
		return pkg.Name + "\x00" + pm + "\x00" + strings.TrimSpace(pkg.PWAURL)
	}
	return pkg.Name + "\x00" + pm
}

func validatePWAFields(cfgName string, pkg models.Package) error {
	rawURL := strings.TrimSpace(pkg.PWAURL)
	if rawURL == "" {
		return fmt.Errorf("configuration %q: package %q with packageManager pwa requires pwaURL", cfgName, pkg.Name)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("configuration %q: package %q has invalid pwaURL", cfgName, pkg.Name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("configuration %q: package %q pwaURL must use http or https", cfgName, pkg.Name)
	}
	if icon := strings.TrimSpace(pkg.PWAIcon); icon != "" {
		iconURL, err := url.Parse(icon)
		if err != nil || iconURL.Scheme == "" || iconURL.Host == "" {
			return fmt.Errorf("configuration %q: package %q has invalid pwaIcon", cfgName, pkg.Name)
		}
	}
	users := strings.TrimSpace(pkg.PWAUsers)
	if users == "" {
		users = "interactive"
	}
	if users != "interactive" {
		return fmt.Errorf("configuration %q: package %q: pwaUsers must be %q", cfgName, pkg.Name, "interactive")
	}
	return nil
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

func validateDirectories(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, directory := range cfg.Directories {
		if _, dup := seen[directory.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate directory %q", cfgName, directory.Name)
		}
		seen[directory.Name] = struct{}{}
		if err := directory.Validate(); err != nil {
			return fmt.Errorf("configuration %q: %w", cfgName, err)
		}
	}
	return nil
}

func validateLinks(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, link := range cfg.Links {
		if _, dup := seen[link.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate link %q", cfgName, link.Name)
		}
		seen[link.Name] = struct{}{}
		if err := link.Validate(); err != nil {
			return fmt.Errorf("configuration %q: %w", cfgName, err)
		}
	}
	return nil
}

func validateGroups(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, group := range cfg.Groups {
		if _, dup := seen[group.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate group %q", cfgName, group.Name)
		}
		seen[group.Name] = struct{}{}
		if err := group.Validate(); err != nil {
			return fmt.Errorf("configuration %q: %w", cfgName, err)
		}
	}
	return nil
}

func validateAuthorizedKeys(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, resource := range cfg.AuthorizedKeys {
		if _, duplicate := seen[resource.Name]; duplicate {
			return fmt.Errorf("configuration %q: duplicate authorizedKey %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: %w", cfgName, err)
		}
	}
	return nil
}

func validateKnownHosts(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, resource := range cfg.KnownHosts {
		if _, duplicate := seen[resource.Name]; duplicate {
			return fmt.Errorf("configuration %q: duplicate knownHost %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: %w", cfgName, err)
		}
	}
	return nil
}

func validateSudo(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, resource := range cfg.Sudo {
		if _, duplicate := seen[resource.Name]; duplicate {
			return fmt.Errorf("configuration %q: duplicate sudo %q", cfgName, resource.Name)
		}
		seen[resource.Name] = struct{}{}
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("configuration %q: %w", cfgName, err)
		}
	}
	return nil
}

func validateUserFiles(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, f := range cfg.UserFiles {
		if strings.TrimSpace(f.Name) == "" {
			return fmt.Errorf("configuration %q: userFiles resource missing name", cfgName)
		}
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate userFiles %q", cfgName, f.Name)
		}
		seen[f.Name] = struct{}{}
		users := strings.TrimSpace(f.Users)
		if users != "interactive" {
			return fmt.Errorf("configuration %q: userFiles %q: users must be %q", cfgName, f.Name, "interactive")
		}
		rel := strings.TrimSpace(f.Path)
		if rel == "" {
			return fmt.Errorf("configuration %q: userFiles %q missing path", cfgName, f.Name)
		}
		if filepath.IsAbs(filepath.Clean(rel)) {
			return fmt.Errorf("configuration %q: userFiles %q path must be relative to the user home directory", cfgName, f.Name)
		}
		if strings.HasPrefix(filepath.Clean(rel), "..") {
			return fmt.Errorf("configuration %q: userFiles %q invalid path", cfgName, f.Name)
		}
		if f.UpdateExisting && strings.TrimSpace(f.WithRegx) != "" && strings.TrimSpace(f.Content) == "" {
			return fmt.Errorf("configuration %q: userFiles %q line edit requires content", cfgName, f.Name)
		}
		if f.UpdateExisting && strings.TrimSpace(f.WithRegx) != "" {
			if _, err := regexp.Compile(strings.TrimSpace(f.WithRegx)); err != nil {
				return fmt.Errorf("configuration %q: userFiles %q invalid withRegx: %w", cfgName, f.Name, err)
			}
		}
		if rep := strings.TrimSpace(f.ReplaceRegx); rep != "" {
			if _, err := regexp.Compile(rep); err != nil {
				return fmt.Errorf("configuration %q: userFiles %q invalid replaceRegx: %w", cfgName, f.Name, err)
			}
		}
		if !f.UpdateExisting && strings.TrimSpace(f.Content) == "" && strings.TrimSpace(f.WithRegx) == "" {
			return fmt.Errorf("configuration %q: userFiles %q requires content", cfgName, f.Name)
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
		if err := u.Validate(); err != nil {
			return fmt.Errorf("configuration %q: user %q: %w", cfgName, u.Name, err)
		}
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

func validateFirewall(cfg models.Configuration, cfgName string) error {
	seen := map[string]struct{}{}
	for _, fw := range cfg.Firewall {
		if strings.TrimSpace(fw.Name) == "" {
			return fmt.Errorf("configuration %q: firewall resource missing name", cfgName)
		}
		if _, dup := seen[fw.Name]; dup {
			return fmt.Errorf("configuration %q: duplicate firewall resource %q", cfgName, fw.Name)
		}
		seen[fw.Name] = struct{}{}
		if err := fw.Validate(); err != nil {
			return fmt.Errorf("configuration %q: %w", cfgName, err)
		}

		if len(fw.Rules) == 0 {
			action := strings.ToLower(strings.TrimSpace(fw.Action))
			if action == "" {
				return fmt.Errorf("configuration %q: firewall %q missing action", cfgName, fw.Name)
			}
		}

		backend := strings.ToLower(strings.TrimSpace(fw.Backend))
		if backend != "" {
			if backend != "firewalld" && backend != "nftables" {
				return fmt.Errorf("configuration %q: firewall %q: invalid backend %q (want firewalld or nftables)", cfgName, fw.Name, fw.Backend)
			}
		}

		for _, src := range fw.Sources {
			if _, _, err := net.ParseCIDR(src); err != nil {
				return fmt.Errorf("configuration %q: firewall %q: invalid source CIDR %q: %w", cfgName, fw.Name, src, err)
			}
		}
		for _, dst := range fw.Destinations {
			if _, _, err := net.ParseCIDR(dst); err != nil {
				return fmt.Errorf("configuration %q: firewall %q: invalid destination CIDR %q: %w", cfgName, fw.Name, dst, err)
			}
		}
		for _, rule := range fw.Rules {
			for _, src := range rule.Sources {
				if _, _, err := net.ParseCIDR(src); err != nil {
					return fmt.Errorf("configuration %q: firewall %q rule %q: invalid source CIDR %q: %w", cfgName, fw.Name, rule.Name, src, err)
				}
			}
			for _, dst := range rule.Destinations {
				if _, _, err := net.ParseCIDR(dst); err != nil {
					return fmt.Errorf("configuration %q: firewall %q rule %q: invalid destination CIDR %q: %w", cfgName, fw.Name, rule.Name, dst, err)
				}
			}
		}

		if backend == "nftables" || (backend == "" && len(fw.Zones) > 0) {
			// Emit warning for firewalld-specific fields when nftables is implied
			// Actual warning emission is handled by the caller (config validate CLI)
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
