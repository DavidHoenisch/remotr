package configcompose

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

// ErrNoManifest indicates no kind: manifest file exists under the requested directory tree.
var ErrNoManifest = errors.New("no kind: manifest found")

// DiscoveredFiles groups repository YAML paths by kind.
type DiscoveredFiles struct {
	Manifests    []string
	Modules      []string
	Applications []string
	Crons        []string
	Unknown      []string
}

// DiscoverFiles walks root and classifies every .yaml file by its kind field.
func DiscoverFiles(root string) (DiscoveredFiles, error) {
	root, err := absRepoRoot(root)
	if err != nil {
		return DiscoveredFiles{}, err
	}
	var out DiscoveredFiles
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		kind, err := parseFileKind(root, rel)
		if err != nil {
			out.Unknown = append(out.Unknown, rel)
			return nil
		}
		switch kind {
		case types.KindManifest:
			out.Manifests = append(out.Manifests, rel)
		case types.KindModule:
			out.Modules = append(out.Modules, rel)
		case types.KindApplication:
			out.Applications = append(out.Applications, rel)
		case types.KindCrons:
			out.Crons = append(out.Crons, rel)
		default:
			out.Unknown = append(out.Unknown, rel)
		}
		return nil
	})
	if err != nil {
		return DiscoveredFiles{}, err
	}
	return out, nil
}

// FindManifestInTree returns the repo-relative path of the single kind: manifest under dir.
func FindManifestInTree(repoRoot, dirRel string) (string, error) {
	dirRel = normalizeRelPath(dirRel)
	dirPath := filepath.Join(repoRoot, filepath.FromSlash(dirRel))
	info, err := os.Stat(dirPath)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", dirRel, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", dirRel)
	}

	var manifests []string
	err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		kind, err := parseFileKind(repoRoot, rel)
		if err != nil {
			return nil
		}
		if kind == types.KindManifest {
			manifests = append(manifests, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	switch len(manifests) {
	case 0:
		return "", fmt.Errorf("%w under %q", ErrNoManifest, dirRel)
	case 1:
		return manifests[0], nil
	default:
		return "", fmt.Errorf("multiple kind: manifest files under %q: %s", dirRel, strings.Join(manifests, ", "))
	}
}

// DiscoverFilesOfKind collects kind-matching YAML files under dir (recursive).
func DiscoverFilesOfKind(repoRoot, dirRel string, want types.Kind) ([]string, error) {
	dirRel = normalizeRelPath(dirRel)
	dirPath := filepath.Join(repoRoot, filepath.FromSlash(dirRel))
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", dirRel, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dirRel)
	}

	var out []string
	err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		kind, err := parseFileKind(repoRoot, rel)
		if err != nil {
			return nil
		}
		if kind == want {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListFleetNames returns fleet directory names under fleets/.
func ListFleetNames(repoRoot string) ([]string, error) {
	fleetsDir := filepath.Join(repoRoot, "fleets")
	entries, err := os.ReadDir(fleetsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, ent := range entries {
		if ent.IsDir() && !strings.HasPrefix(ent.Name(), ".") {
			out = append(out, ent.Name())
		}
	}
	return out, nil
}

// ListEndpointOverrideIDs returns endpoint ids with a manifest under endpoints/.
func ListEndpointOverrideIDs(repoRoot string) ([]string, error) {
	endpointsDir := filepath.Join(repoRoot, "endpoints")
	entries, err := os.ReadDir(endpointsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		id := ent.Name()
		_, err := FindManifestInTree(repoRoot, filepath.Join("endpoints", id))
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

func parseFileKind(repoRoot, relPath string) (types.Kind, error) {
	data, err := readRepoRelative(repoRoot, relPath)
	if err != nil {
		return "", err
	}
	var head struct {
		Kind types.Kind `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return "", err
	}
	kind := types.Kind(strings.TrimSpace(head.Kind.String()))
	if kind == "" {
		return "", fmt.Errorf("missing kind")
	}
	return kind, nil
}

func absRepoRoot(repoRoot string) (string, error) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("repository: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository: %s is not a directory", abs)
	}
	return abs, nil
}
