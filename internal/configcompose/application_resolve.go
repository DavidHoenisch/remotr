package configcompose

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// resolveApplicationModuleRef maps a fleet reference to a repository-relative application file.
//
// Resolution order:
//  1. Path as given (repo-relative), with optional .yaml suffix
//  2. Path under applications/ (e.g. pwa/microsoft/teams → applications/pwa/microsoft/teams.yaml)
//  3. Basename crawl of applications/ when the reference has no slash (e.g. teams → **/teams.yaml)
//
// Folder layout under applications/ is not prescribed — organize arbitrarily.
func resolveApplicationModuleRef(repoRoot, ref string) (string, error) {
	ref = normalizeRelPath(ref)
	if ref == "" {
		return "", fmt.Errorf("empty application reference")
	}

	if resolved, ok := resolveApplicationDirectPath(repoRoot, ref); ok {
		return resolved, nil
	}

	if !strings.HasPrefix(ref, "applications/") {
		underApps := filepath.ToSlash(filepath.Join("applications", ref))
		if resolved, ok := resolveApplicationDirectPath(repoRoot, underApps); ok {
			return resolved, nil
		}
	}

	if strings.Contains(ref, "/") {
		return "", fmt.Errorf("application %q not found under applications/", ref)
	}

	matches, err := findApplicationFilesByBasename(repoRoot, ref)
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("application %q not found under applications/", ref)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("application %q is ambiguous (%d matches: %s); use an explicit path such as %q",
			ref, len(matches), strings.Join(matches, ", "), matches[0])
	}
}

func resolveApplicationDirectPath(repoRoot, ref string) (string, bool) {
	for _, candidate := range applicationPathCandidates(ref) {
		if !pathExistsUnderRepo(repoRoot, candidate) {
			continue
		}
		if err := validateApplicationModulePath(candidate); err != nil {
			continue
		}
		return candidate, true
	}
	return "", false
}

func applicationPathCandidates(ref string) []string {
	ref = normalizeRelPath(ref)
	if ref == "" {
		return nil
	}
	if strings.HasSuffix(ref, ".yaml") {
		return []string{ref}
	}
	return []string{ref, ref + ".yaml"}
}

func validateApplicationModulePath(relPath string) error {
	relPath = normalizeRelPath(relPath)
	base := filepath.Base(relPath)
	if base == "manifest.yaml" || base == "applications.manifest.yaml" {
		return fmt.Errorf("%q is a manifest file, not an application", relPath)
	}
	return nil
}

func findApplicationFilesByBasename(repoRoot, ref string) ([]string, error) {
	baseName := strings.TrimSuffix(ref, ".yaml")
	if baseName == "" {
		return nil, nil
	}
	targetFile := baseName + ".yaml"

	appsRoot := filepath.Join(repoRoot, "applications")
	if _, err := os.Stat(appsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var matches []string
	err := filepath.WalkDir(appsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".yaml") {
			return nil
		}
		if name == "manifest.yaml" {
			return nil
		}
		if name != targetFile {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := validateApplicationModulePath(rel); err != nil {
			return nil
		}
		matches = append(matches, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func pathExistsUnderRepo(repoRoot, relPath string) bool {
	relPath = normalizeRelPath(relPath)
	if relPath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relPath)))
	return err == nil
}

func resolveApplicationModuleRefs(repoRoot string, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		resolved, err := resolveApplicationModuleRef(repoRoot, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, resolved)
	}
	return out, nil
}
