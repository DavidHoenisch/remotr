package apppackages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
	if err := ValidateNameVersion(name, defaultVersion(opts.Version)); err != nil {
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
	manifest := Manifest{
		SchemaVersion: 1,
		Name:          name,
		Version:       defaultVersion(opts.Version),
	}

	switch mode {
	case "binary":
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
			return Manifest{}, err
		}
		binPath := filepath.Join(dir, "bin", binName)
		if err := writeExecutablePlaceholder(binPath, binName); err != nil {
			return Manifest{}, err
		}
		manifest.Install = InstallSpec{
			Mode: "binary",
			Files: []InstallFile{{
				Src:  filepath.ToSlash(filepath.Join("bin", binName)),
				Dest: filepath.Join("/usr/local/bin", binName),
				Mode: "0755",
			}},
		}
	case "script":
		if err := writeInstallScript(filepath.Join(dir, "install.sh"), name); err != nil {
			return Manifest{}, err
		}
		manifest.Install = InstallSpec{
			Mode:   "script",
			Script: []string{"./install.sh"},
		}
	case "build":
		if err := writeInstallScript(filepath.Join(dir, "install.sh"), name); err != nil {
			return Manifest{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("# add runtime dependencies\n"), 0o644); err != nil { // #nosec G703
			return Manifest{}, err
		}
		manifest.Install = InstallSpec{
			Mode: "build",
			Build: [][]string{
				{"python3", "-m", "venv", ".venv"},
				{".venv/bin/pip", "install", "-r", "requirements.txt"},
			},
			Script: []string{"./install.sh"},
		}
	}

	raw, err := yaml.Marshal(manifest)
	if err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), raw, 0o644); err != nil { // #nosec G703
		return Manifest{}, err
	}
	return manifest, nil
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

func writeExecutablePlaceholder(path, binName string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	body := fmt.Sprintf("#!/bin/sh\necho %s placeholder — replace bin/%s with your built binary\n", binName, binName)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil { // #nosec G703
		return err
	}
	return nil
}

func writeInstallScript(path, name string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	body := fmt.Sprintf(`#!/bin/sh
set -eu
# Install steps for %s
echo "install %s"
`, name, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil { // #nosec G703
		return err
	}
	return nil
}
