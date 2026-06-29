package configcompose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func resolveRelativeRef(manifestDir, ref string) string {
	ref = normalizeRelPath(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "modules/") || strings.HasPrefix(ref, "applications/") ||
		strings.HasPrefix(ref, "fleets/") || strings.HasPrefix(ref, "endpoints/") ||
		strings.HasPrefix(ref, "crons/") {
		return ref
	}
	if strings.HasPrefix(ref, "./") || !strings.Contains(ref, "/") {
		return normalizeRelPath(filepath.Join(manifestDir, ref))
	}
	return ref
}

func resolveModuleRefs(repoRoot, manifestDir string, refs []string) ([]string, error) {
	return resolveKindRefs(repoRoot, manifestDir, refs, types.KindModule)
}

func resolveCronRefs(repoRoot, manifestDir string, refs []string) ([]string, error) {
	return resolveKindRefs(repoRoot, manifestDir, refs, types.KindCrons)
}

func resolveKindRefs(repoRoot, manifestDir string, refs []string, want types.Kind) ([]string, error) {
	var out []string
	seen := map[string]struct{}{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		resolved := resolveRelativeRef(manifestDir, ref)
		path := filepath.Join(repoRoot, filepath.FromSlash(resolved))
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("reference %q: %w", ref, err)
		}
		if info.IsDir() {
			files, err := DiscoverFilesOfKind(repoRoot, resolved, want)
			if err != nil {
				return nil, fmt.Errorf("reference %q: %w", ref, err)
			}
			if len(files) == 0 {
				return nil, fmt.Errorf("reference %q: no %s files found", ref, want)
			}
			for _, f := range files {
				if _, dup := seen[f]; dup {
					continue
				}
				seen[f] = struct{}{}
				out = append(out, f)
			}
			continue
		}
		kind, err := parseFileKind(repoRoot, resolved)
		if err != nil {
			return nil, fmt.Errorf("reference %q (%s): %w", ref, resolved, err)
		}
		if kind != want {
			return nil, fmt.Errorf("reference %q (%s): want kind %s, got %s", ref, resolved, want, kind)
		}
		if _, dup := seen[resolved]; dup {
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	return out, nil
}

func resolveApplicationRefs(repoRoot, manifestDir string, refs []string) ([]string, error) {
	var out []string
	seen := map[string]struct{}{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		resolved, err := resolveApplicationRef(repoRoot, manifestDir, ref)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[resolved]; dup {
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	return out, nil
}

func resolveApplicationRef(repoRoot, manifestDir, ref string) (string, error) {
	originalRef := strings.TrimSpace(ref)
	if originalRef == "" {
		return "", fmt.Errorf("empty application reference")
	}

	rel := resolveRelativeRef(manifestDir, originalRef)
	if resolved, ok := resolveApplicationDirectPath(repoRoot, rel); ok {
		return resolved, nil
	}

	if !strings.HasPrefix(rel, "applications/") {
		underApps := filepath.ToSlash(filepath.Join("applications", rel))
		if resolved, ok := resolveApplicationDirectPath(repoRoot, underApps); ok {
			return resolved, nil
		}
	}

	if strings.Contains(originalRef, "/") {
		underApps := filepath.ToSlash(filepath.Join("applications", normalizeRelPath(originalRef)))
		if resolved, ok := resolveApplicationDirectPath(repoRoot, underApps); ok {
			return resolved, nil
		}
		return "", fmt.Errorf("application %q not found", originalRef)
	}

	matches, err := findApplicationFilesByBasename(repoRoot, originalRef)
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("application %q not found", originalRef)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("application %q is ambiguous (%d matches: %s); use an explicit path",
			originalRef, len(matches), strings.Join(matches, ", "))
	}
}
