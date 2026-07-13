package models

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Validate rejects directory intent the provider cannot observe or apply.
func (d DirectoryResource) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("directory resource missing name")
	}
	path := filepath.Clean(strings.TrimSpace(d.Path))
	if !filepath.IsAbs(path) || path == string(filepath.Separator) {
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
	return nil
}
