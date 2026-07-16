package aisetup

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const manifestName = ".remotr-ai.json"

// InstallOptions configures copying a bundle tree into an agent skill directory.
type InstallOptions struct {
	Target        Target
	Source        fs.FS
	SourceRoot    string
	SourceLabel   string
	SourceVersion string
	Force         bool
	BoundaryRoot  string
}

// Install copies the bundle and writes an install manifest.
func Install(opt InstallOptions) (InstallManifest, error) {
	if opt.Source == nil {
		return InstallManifest{}, fmt.Errorf("bundle source is required")
	}
	root := strings.TrimSpace(opt.SourceRoot)
	if root == "" {
		return InstallManifest{}, fmt.Errorf("bundle source root is required")
	}

	var installed bool
	var err error
	if opt.BoundaryRoot == "" {
		installed, err = opt.Target.Installed()
	} else {
		installed, err = InstalledWithin(opt.BoundaryRoot, opt.Target.InstallDir)
	}
	if err != nil {
		return InstallManifest{}, err
	}
	if installed && !opt.Force {
		return InstallManifest{}, fmt.Errorf("already installed at %s (use --force to replace)", opt.Target.InstallDir)
	}

	if opt.BoundaryRoot == "" {
		if err := os.MkdirAll(opt.Target.InstallDir, 0o755); err != nil {
			return InstallManifest{}, err
		}
		if err := copyTree(opt.Source, root, opt.Target.InstallDir); err != nil {
			return InstallManifest{}, err
		}
	} else if err := copyTreeWithin(opt.Source, root, opt.BoundaryRoot, opt.Target.InstallDir); err != nil {
		return InstallManifest{}, err
	}

	bundleVersion, _ := readBundleVersion(opt.Source, root)
	manifest := InstallManifest{
		Agent:         string(opt.Target.Agent),
		Scope:         string(opt.Target.Scope),
		InstallDir:    opt.Target.InstallDir,
		BundleVersion: bundleVersion,
		Source:        opt.SourceLabel,
		SourceVersion: opt.SourceVersion,
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if opt.BoundaryRoot == "" {
		err = writeManifest(opt.Target.InstallDir, manifest)
	} else {
		err = writeManifestWithin(opt.BoundaryRoot, opt.Target.InstallDir, manifest)
	}
	if err != nil {
		return InstallManifest{}, err
	}
	return manifest, nil
}

// InstalledWithin reports installation state without permitting path or
// symlink traversal outside boundaryRoot.
func InstalledWithin(boundaryRoot, installDir string) (bool, error) {
	root, relative, err := openInstallBoundary(boundaryRoot, installDir)
	if err != nil {
		return false, err
	}
	defer root.Close()
	if _, err := root.Stat(filepath.Join(relative, "SKILL.md")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ReadManifestWithin reads an installation manifest without permitting path
// or symlink traversal outside boundaryRoot.
func ReadManifestWithin(boundaryRoot, installDir string) (InstallManifest, error) {
	root, relative, err := openInstallBoundary(boundaryRoot, installDir)
	if err != nil {
		return InstallManifest{}, err
	}
	defer root.Close()
	data, err := root.ReadFile(filepath.Join(relative, manifestName))
	if err != nil {
		return InstallManifest{}, err
	}
	var manifest InstallManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return InstallManifest{}, err
	}
	return manifest, nil
}

func copyTree(src fs.FS, srcRoot, destRoot string) error {
	return fs.WalkDir(src, srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(destRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if err := copyFile(src, path, dest, d); err != nil {
			return err
		}
		return nil
	})
}

func copyTreeWithin(src fs.FS, srcRoot, boundaryRoot, installDir string) error {
	root, destRoot, err := openInstallBoundary(boundaryRoot, installDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(destRoot, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(src, srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(destRoot, relative)
		if d.IsDir() {
			return root.MkdirAll(destination, 0o755)
		}
		return copyFileWithin(src, path, root, destination, d)
	})
}

func copyFile(src fs.FS, srcPath, destPath string, d fs.DirEntry) error {
	in, err := src.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	mode := fs.FileMode(0o644)
	if info, err := d.Info(); err == nil && info.Mode().Perm() != 0 {
		mode = info.Mode().Perm()
	}
	if strings.HasSuffix(srcPath, ".sh") {
		mode = 0o755
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode) // #nosec G304 G703
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func copyFileWithin(src fs.FS, srcPath string, root *os.Root, destPath string, d fs.DirEntry) error {
	in, err := src.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	mode := fs.FileMode(0o644)
	if info, infoErr := d.Info(); infoErr == nil && info.Mode().Perm() != 0 {
		mode = info.Mode().Perm()
	}
	if strings.HasSuffix(srcPath, ".sh") {
		mode = 0o755
	}
	if err := root.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := root.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func readBundleVersion(src fs.FS, root string) (string, error) {
	data, err := fs.ReadFile(src, filepath.ToSlash(filepath.Join(root, "VERSION")))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeManifest(dir string, manifest InstallManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestName), data, 0o644)
}

func writeManifestWithin(boundaryRoot, installDir string, manifest InstallManifest) error {
	root, relative, err := openInstallBoundary(boundaryRoot, installDir)
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return root.WriteFile(filepath.Join(relative, manifestName), data, 0o644)
}

func openInstallBoundary(boundaryRoot, installDir string) (*os.Root, string, error) {
	boundaryRoot = filepath.Clean(boundaryRoot)
	installDir = filepath.Clean(installDir)
	if !filepath.IsAbs(boundaryRoot) || !filepath.IsAbs(installDir) {
		return nil, "", fmt.Errorf("installation boundary and target must be absolute")
	}
	relative, err := filepath.Rel(boundaryRoot, installDir)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("installation target is outside the selected boundary")
	}
	root, err := os.OpenRoot(boundaryRoot)
	if err != nil {
		return nil, "", err
	}
	return root, relative, nil
}

func ReadManifest(dir string) (InstallManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return InstallManifest{}, err
	}
	var manifest InstallManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return InstallManifest{}, err
	}
	return manifest, nil
}
