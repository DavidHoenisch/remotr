package models

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// OsqueryMode controls whether and how Remotr provisions or reuses osquery.
type OsqueryMode string

const (
	OsqueryModeDisabled OsqueryMode = "disabled"
	OsqueryModeExisting OsqueryMode = "existing"
	OsqueryModeManaged  OsqueryMode = "managed"
)

// OsqueryVisibility controls whether a Remotr-managed osquery install is isolated
// from regular users (private) or exposed globally (global).
type OsqueryVisibility string

const (
	OsqueryVisibilityPrivate OsqueryVisibility = "private"
	OsqueryVisibilityGlobal  OsqueryVisibility = "global"
)

const (
	DefaultOsqueryPrivateInstallDir = "/usr/lib/remotr/osquery"
	DefaultOsqueryGlobalInstallDir  = "/opt/remotr-osquery"
	DefaultOsqueryGlobalSymlinkPath = "/usr/local/bin/osqueryi"
)

// OsqueryResource declares if, when, and where osquery is installed or reused on
// an endpoint without clobbering third-party security agents.
type OsqueryResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string            `yaml:"name"`
	Mode         OsqueryMode       `yaml:"mode"`
	Visibility   OsqueryVisibility `yaml:"visibility,omitempty"`
	InstallDir   string            `yaml:"installDir,omitempty"`
	SymlinkPath  string            `yaml:"symlinkPath,omitempty"`
	BinaryPaths  []string          `yaml:"binaryPaths,omitempty"`
	AllowedOwner string            `yaml:"allowedOwner,omitempty"`
	Version      string            `yaml:"version,omitempty"`
	SourceURL    string            `yaml:"sourceUrl,omitempty"`
	SHA256       string            `yaml:"sha256,omitempty"`
}

// EffectiveVisibility returns private by default for managed mode.
func (r OsqueryResource) EffectiveVisibility() OsqueryVisibility {
	if r.Visibility == "" {
		return OsqueryVisibilityPrivate
	}
	return r.Visibility
}

// EffectiveInstallDir returns the managed install directory for the configured visibility.
func (r OsqueryResource) EffectiveInstallDir() string {
	if strings.TrimSpace(r.InstallDir) != "" {
		return filepath.Clean(strings.TrimSpace(r.InstallDir))
	}
	if r.EffectiveVisibility() == OsqueryVisibilityGlobal {
		return DefaultOsqueryGlobalInstallDir
	}
	return DefaultOsqueryPrivateInstallDir
}

// EffectiveSymlinkPath returns the global symlink path when visibility is global.
func (r OsqueryResource) EffectiveSymlinkPath() string {
	if r.EffectiveVisibility() != OsqueryVisibilityGlobal {
		return ""
	}
	if strings.TrimSpace(r.SymlinkPath) != "" {
		return filepath.Clean(strings.TrimSpace(r.SymlinkPath))
	}
	return DefaultOsqueryGlobalSymlinkPath
}

// EffectiveAllowedOwner returns the expected owner of a reused binary (default root).
func (r OsqueryResource) EffectiveAllowedOwner() string {
	if strings.TrimSpace(r.AllowedOwner) != "" {
		return strings.TrimSpace(r.AllowedOwner)
	}
	return "root"
}

// Validate checks mode-specific invariants and anti-clobbering path constraints.
func (r OsqueryResource) Validate() error {
	if !validQueryName(r.Name) {
		return fmt.Errorf("osquery resource name %q must be a safe identifier", r.Name)
	}
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("osquery %q lifecycle must be present or absent", r.Name)
	}
	switch r.Mode {
	case OsqueryModeDisabled, OsqueryModeExisting, OsqueryModeManaged:
	default:
		return fmt.Errorf("osquery %q mode %q must be disabled, existing, or managed", r.Name, r.Mode)
	}

	if lifecycle == LifecycleAbsent {
		if r.InstallDir != "" && !validOsqueryInstallDir(r.InstallDir) {
			return fmt.Errorf("osquery %q has invalid installDir %q", r.Name, r.InstallDir)
		}
		if r.SymlinkPath != "" && !validOsquerySymlinkPath(r.SymlinkPath) {
			return fmt.Errorf("osquery %q has invalid symlinkPath %q", r.Name, r.SymlinkPath)
		}
		return nil
	}

	switch r.Mode {
	case OsqueryModeDisabled:
		if r.Visibility != "" || r.InstallDir != "" || r.SymlinkPath != "" || len(r.BinaryPaths) > 0 ||
			r.AllowedOwner != "" || r.Version != "" || r.SourceURL != "" || r.SHA256 != "" {
			return fmt.Errorf("osquery %q mode disabled must not declare install or binary fields", r.Name)
		}
		return nil

	case OsqueryModeExisting:
		if r.Visibility != "" || r.InstallDir != "" || r.SymlinkPath != "" || r.Version != "" || r.SourceURL != "" || r.SHA256 != "" {
			return fmt.Errorf("osquery %q mode existing must not declare managed install fields", r.Name)
		}
		if len(r.BinaryPaths) == 0 {
			return fmt.Errorf("osquery %q mode existing requires at least one binaryPaths entry", r.Name)
		}
		if r.AllowedOwner != "" && !validLocalAccountName(r.AllowedOwner) {
			return fmt.Errorf("osquery %q has invalid allowedOwner %q", r.Name, r.AllowedOwner)
		}
		for _, candidate := range r.BinaryPaths {
			if !validOsqueryBinaryCandidate(candidate) {
				return fmt.Errorf("osquery %q has unsafe binaryPaths entry %q", r.Name, candidate)
			}
		}
		return nil

	case OsqueryModeManaged:
		if len(r.BinaryPaths) > 0 || r.AllowedOwner != "" {
			return fmt.Errorf("osquery %q mode managed must not declare binaryPaths or allowedOwner", r.Name)
		}
		vis := r.EffectiveVisibility()
		if vis != OsqueryVisibilityPrivate && vis != OsqueryVisibilityGlobal {
			return fmt.Errorf("osquery %q visibility %q must be private or global", r.Name, r.Visibility)
		}
		if vis == OsqueryVisibilityPrivate && strings.TrimSpace(r.SymlinkPath) != "" {
			return fmt.Errorf("osquery %q private visibility must not set symlinkPath", r.Name)
		}
		if !validOsqueryInstallDir(r.EffectiveInstallDir()) {
			return fmt.Errorf("osquery %q installDir %q must be a dedicated absolute path outside standard system bin directories", r.Name, r.EffectiveInstallDir())
		}
		if vis == OsqueryVisibilityGlobal && !validOsquerySymlinkPath(r.EffectiveSymlinkPath()) {
			return fmt.Errorf("osquery %q symlinkPath %q must be a clean absolute binary path", r.Name, r.EffectiveSymlinkPath())
		}
		if strings.TrimSpace(r.Version) == "" || strings.ContainsAny(r.Version, " \t/\r\n\x00") {
			return fmt.Errorf("osquery %q mode managed requires a valid version", r.Name)
		}
		if err := validateHTTPSURL(r.SourceURL); err != nil {
			return fmt.Errorf("osquery %q sourceUrl: %w", r.Name, err)
		}
		if !validSHA256Hex(r.SHA256) {
			return fmt.Errorf("osquery %q mode managed requires a 64-character hex sha256", r.Name)
		}
		return nil
	}
	return nil
}

func validateHTTPSURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("must be a valid https URL")
	}
	return nil
}

func validSHA256Hex(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, r := range v {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func validOsqueryInstallDir(dir string) bool {
	if !strings.HasPrefix(dir, "/") || strings.ContainsAny(dir, "\r\n\x00*?[]") {
		return false
	}
	if strings.Contains(dir, "..") {
		return false
	}
	clean := filepath.Clean(dir)
	switch clean {
	case "/", "/bin", "/sbin", "/usr", "/usr/bin", "/usr/sbin", "/usr/local", "/usr/local/bin", "/usr/local/sbin", "/etc", "/var", "/opt", "/tmp":
		return false
	}
	return true
}

func validOsquerySymlinkPath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, " \t\r\n\x00*?[]") || strings.Contains(p, "..") {
		return false
	}
	clean := filepath.Clean(p)
	base := filepath.Base(clean)
	return clean != "/" && (base == "osqueryi" || base == "osqueryd")
}

// validOsqueryBinaryCandidate allows exact absolute binary paths or anchored
// single-segment wildcard patterns (e.g. /opt/Elastic/Agent/data/elastic-agent-*/components/osqueryd)
// while rejecting relative paths, traversal, wildcards in the basename, or multiple wildcards.
func validOsqueryBinaryCandidate(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, " \t\r\n\x00?[]") {
		return false
	}
	segments := strings.Split(p, "/")
	if len(segments) < 3 {
		return false
	}
	base := segments[len(segments)-1]
	if base != "osqueryi" && base != "osqueryd" {
		return false
	}
	wildcardSegments := 0
	for _, seg := range segments[1 : len(segments)-1] {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
		if strings.Contains(seg, "*") {
			if seg == "*" {
				return false // must be anchored with a prefix/suffix
			}
			wildcardSegments++
			if wildcardSegments > 1 {
				return false
			}
		}
	}
	return true
}
