package models

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Validate rejects directory intent the provider cannot observe or apply.
func (d DirectoryResource) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("directory resource missing name")
	}
	cleanPath := filepath.Clean(strings.TrimSpace(d.Path))
	if !filepath.IsAbs(cleanPath) || cleanPath == string(filepath.Separator) {
		return fmt.Errorf("directory %q path must be an absolute non-root path", d.Name)
	}
	if d.Lifecycle != LifecyclePresent && d.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("directory %q lifecycle must be present or absent", d.Name)
	}
	if len(d.Mode) > 1 {
		return fmt.Errorf("directory %q mode accepts one value", d.Name)
	}
	if len(d.Mode) == 1 && (d.Mode[0] < 0 || d.Mode[0] > 0o777) {
		return fmt.Errorf("directory %q mode must be between 000 and 777", d.Name)
	}
	if !d.Recursive && (d.Purge || d.CrossFilesystem || len(d.Exclusions) > 0 || d.MaxDepth != 0 || d.MaxEntries != 0) {
		return fmt.Errorf("directory %q recursive policy requires recursive: true", d.Name)
	}
	if d.Recursive && (d.MaxDepth <= 0 || d.MaxEntries <= 0) {
		return fmt.Errorf("directory %q recursive policy requires positive maxDepth and maxEntries", d.Name)
	}
	if d.Purge && d.Ownership != OwnershipAuthoritative {
		return fmt.Errorf("directory %q purge requires authoritative ownership", d.Name)
	}
	for _, pattern := range d.Exclusions {
		if strings.TrimSpace(pattern) == "" || strings.HasPrefix(pattern, "/") || strings.HasPrefix(filepath.Clean(pattern), "..") {
			return fmt.Errorf("directory %q has invalid exclusion %q", d.Name, pattern)
		}
		if _, err := path.Match(pattern, "example"); err != nil {
			return fmt.Errorf("directory %q exclusion %q: %w", d.Name, pattern, err)
		}
	}
	return nil
}
