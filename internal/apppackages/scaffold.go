package apppackages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScaffoldOptions configures a new on-disk package directory.
type ScaffoldOptions struct {
	Dir     string
	Name    string
	Version string
	Mode    string
	Force   bool
}

// CreateScaffold writes a new package source tree at opts.Dir.
func CreateScaffold(opts ScaffoldOptions) (Manifest, error) {
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		return Manifest{}, fmt.Errorf("path required")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = filepath.Base(filepath.Clean(dir))
	}
	version := defaultVersion(opts.Version)
	if err := ValidateNameVersion(name, version); err != nil {
		return Manifest{}, err
	}
	mode := strings.TrimSpace(opts.Mode)
	if mode == "" {
		mode = "binary"
	}
	if mode != "binary" && mode != "script" && mode != "build" {
		return Manifest{}, fmt.Errorf("mode must be binary, script, or build")
	}

	if info, err := os.Stat(dir); err == nil {
		if !opts.Force {
			return Manifest{}, fmt.Errorf("path already exists: %s", dir)
		}
		if !info.IsDir() {
			return Manifest{}, fmt.Errorf("path exists and is not a directory: %s", dir)
		}
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, err
	}

	binName := binaryName(name)
	if err := createScaffoldTree(dir, binName); err != nil {
		return Manifest{}, err
	}

	manifestYAML := renderManifestTemplate(name, version, binName, mode)
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(manifestYAML), 0o644); err != nil { // #nosec G703
		return Manifest{}, err
	}
	return ParseManifest([]byte(manifestYAML))
}

func createScaffoldTree(dir, binName string) error {
	dirs := []string{
		filepath.Join(dir, "bin"),
		filepath.Join(dir, "lib"),
		filepath.Join(dir, "share"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	if err := writeExecutablePlaceholder(
		filepath.Join(dir, "bin", binName+"-linux-amd64"),
		binName,
	); err != nil {
		return err
	}
	if err := writeExecutablePlaceholder(
		filepath.Join(dir, "bin", binName+"-linux-arm64"),
		binName,
	); err != nil {
		return err
	}
	if err := writeFileIfMissing(
		filepath.Join(dir, "lib", binName+"-helper"),
		"#!/bin/sh\n# Optional helper script or library payload\n",
		0o755,
	); err != nil {
		return err
	}
	if err := writeFileIfMissing(
		filepath.Join(dir, "share", binName+".conf.example"),
		fmt.Sprintf("# Example config for %s\n# Copy to /etc/%s/%s.conf during install if needed\n",
			binName, binName, binName),
		0o644,
	); err != nil {
		return err
	}
	if err := writeInstallScript(filepath.Join(dir, "install.sh"), binName); err != nil {
		return err
	}
	if err := writeUninstallScript(filepath.Join(dir, "uninstall.sh"), binName); err != nil {
		return err
	}
	if err := writeFileIfMissing(
		filepath.Join(dir, "requirements.txt"),
		"# Runtime dependencies for build-mode packages\n# Example:\n# requests>=2.31.0\n",
		0o644,
	); err != nil {
		return err
	}
	return nil
}

func defaultVersion(v string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return "0.1.0"
}

func binaryName(packageName string) string {
	packageName = strings.TrimSpace(packageName)
	if i := strings.LastIndex(packageName, "/"); i >= 0 && i < len(packageName)-1 {
		return packageName[i+1:]
	}
	return packageName
}

func writeFileIfMissing(path, body string, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(body), mode) // #nosec G703
}

func writeExecutablePlaceholder(path, binName string) error {
	body := fmt.Sprintf("#!/bin/sh\necho %s placeholder — replace %s with your built binary\n", binName, filepath.Base(path))
	return writeFileIfMissing(path, body, 0o755)
}

func writeInstallScript(path, name string) error {
	body := fmt.Sprintf(`#!/bin/sh
set -eu

# Install steps for %s. Called after the package zip is extracted.
#
# Examples:
#   install -d /usr/local/bin
#   install -m 0755 bin/%s-linux-amd64 /usr/local/bin/%s
#   install -d /etc/%s
#   install -m 0644 share/%s.conf.example /etc/%s/%s.conf

install -d /usr/local/bin
# Pick the binary that matches the endpoint architecture (see install.files arch: x86 | ARM).
install -m 0755 bin/%s-linux-amd64 /usr/local/bin/%s
`, name, name, name, name, name, name, name, name, name)
	return writeFileIfMissing(path, body, 0o755)
}

func writeUninstallScript(path, name string) error {
	body := fmt.Sprintf(`#!/bin/sh
set -eu

# Uninstall steps for %s. Reference from uninstall.script in remotr-package.yaml.
#
# Examples:
#   rm -f /usr/local/bin/%s
#   rm -rf /etc/%s

rm -f /usr/local/bin/%s
`, name, name, name, name)
	return writeFileIfMissing(path, body, 0o755)
}
