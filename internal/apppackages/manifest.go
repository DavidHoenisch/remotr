package apppackages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

const ManifestName = "remotr-package.yaml"

var (
	packageNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,127}$`)
	versionPattern     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,63}$`)
)

// Manifest describes a custom app package inside a zip archive.
type Manifest struct {
	SchemaVersion int             `yaml:"schemaVersion"`
	Name          string          `yaml:"name"`
	Version       string          `yaml:"version"`
	Install       InstallSpec     `yaml:"install"`
	Check         CheckSpec       `yaml:"check,omitempty"`
	Uninstall     *UninstallSpec  `yaml:"uninstall,omitempty"`
}

type InstallSpec struct {
	Mode   string         `yaml:"mode"`
	Files  []InstallFile  `yaml:"files,omitempty"`
	Script []string       `yaml:"script,omitempty"`
	Build  [][]string     `yaml:"build,omitempty"`
}

type InstallFile struct {
	Src  string           `yaml:"src"`
	Dest string           `yaml:"dest"`
	Mode string           `yaml:"mode,omitempty"`
	Arch types.Architecture `yaml:"arch,omitempty"`
}

type CheckSpec struct {
	VersionFile string   `yaml:"versionFile,omitempty"`
	Command     []string `yaml:"command,omitempty"`
	Expect      string   `yaml:"expect,omitempty"`
}

type UninstallSpec struct {
	Files  []string `yaml:"files,omitempty"`
	Script []string `yaml:"script,omitempty"`
}

// ValidateNameVersion checks catalog reference fields in desired state.
func ValidateNameVersion(name, version string) error {
	if !packageNamePattern.MatchString(strings.TrimSpace(name)) {
		return fmt.Errorf("invalid package name %q", name)
	}
	if !versionPattern.MatchString(strings.TrimSpace(version)) {
		return fmt.Errorf("invalid version %q", version)
	}
	return nil
}

// ParseManifest decodes and validates remotr-package.yaml bytes.
func ParseManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := ValidateManifest(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// ValidateManifest checks manifest fields without zip context.
func ValidateManifest(m Manifest) error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schemaVersion %d (want 1)", m.SchemaVersion)
	}
	name := strings.TrimSpace(m.Name)
	if !packageNamePattern.MatchString(name) {
		return fmt.Errorf("invalid package name %q", m.Name)
	}
	version := strings.TrimSpace(m.Version)
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid version %q", m.Version)
	}
	mode := strings.TrimSpace(m.Install.Mode)
	switch mode {
	case "binary":
		if len(m.Install.Files) == 0 {
			return fmt.Errorf("install.mode binary requires files")
		}
		for i, f := range m.Install.Files {
			if err := validateInstallFile(f, i); err != nil {
				return err
			}
		}
	case "script":
		if len(m.Install.Script) == 0 {
			return fmt.Errorf("install.mode script requires script")
		}
	case "build":
		if len(m.Install.Build) == 0 {
			return fmt.Errorf("install.mode build requires build steps")
		}
	default:
		return fmt.Errorf("invalid install.mode %q (want binary, script, or build)", m.Install.Mode)
	}
	if len(m.Check.Command) > 0 && strings.TrimSpace(m.Check.Expect) == "" {
		return fmt.Errorf("check.expect required when check.command is set")
	}
	if vf := strings.TrimSpace(m.Check.VersionFile); vf != "" {
		if !filepath.IsAbs(vf) {
			return fmt.Errorf("check.versionFile must be absolute")
		}
	}
	for _, dest := range m.UninstallFiles() {
		if !filepath.IsAbs(dest) {
			return fmt.Errorf("uninstall path must be absolute: %q", dest)
		}
	}
	return nil
}

func validateInstallFile(f InstallFile, idx int) error {
	src := strings.TrimSpace(f.Src)
	if src == "" || strings.HasPrefix(src, "/") || strings.Contains(src, "..") {
		return fmt.Errorf("install.files[%d].src must be a relative zip path", idx)
	}
	dest := strings.TrimSpace(f.Dest)
	if !filepath.IsAbs(dest) {
		return fmt.Errorf("install.files[%d].dest must be absolute", idx)
	}
	if f.Arch != "" && f.Arch != types.X86 && f.Arch != types.Arm {
		return fmt.Errorf("install.files[%d].arch must be x86 or ARM", idx)
	}
	return nil
}

// DefaultVersionFile returns the version marker path for a package name.
func DefaultVersionFile(name string) string {
	return filepath.Join("/var/lib/remotr/apps", sanitizeName(name), "version")
}

// VersionFile returns the configured or default version marker path.
func (m Manifest) VersionFile() string {
	if vf := strings.TrimSpace(m.Check.VersionFile); vf != "" {
		return vf
	}
	return DefaultVersionFile(m.Name)
}

// UninstallFiles returns absolute paths removed on uninstall.
func (m Manifest) UninstallFiles() []string {
	if m.Uninstall != nil && len(m.Uninstall.Files) > 0 {
		return append([]string(nil), m.Uninstall.Files...)
	}
	if m.Install.Mode != "binary" {
		return nil
	}
	out := make([]string, 0, len(m.Install.Files))
	for _, f := range m.Install.Files {
		out = append(out, f.Dest)
	}
	return out
}

func sanitizeName(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "..", "")
	if s == "" {
		return "package"
	}
	return s
}

// DefaultS3Key returns the canonical object key for a package zip.
func DefaultS3Key(name, version string) string {
	safeName := strings.ReplaceAll(name, "/", "_")
	return fmt.Sprintf("app-packages/%s/%s/%s-%s.zip", name, version, safeName, version)
}
