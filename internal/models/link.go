package models

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LinkType identifies the filesystem link primitive a LinkResource manages.
type LinkType string

const (
	LinkTypeSymbolic LinkType = "symbolic"
	LinkTypeHard     LinkType = "hard"
)

// LinkResource manages one symbolic or hard link. Link metadata is limited to
// symbolic links because hard links share inode metadata with their target.
type LinkResource struct {
	ResourceMeta         `yaml:",inline"`
	Name                 string   `yaml:"name"`
	Path                 string   `yaml:"path"`
	Target               string   `yaml:"target,omitempty"`
	LinkType             LinkType `yaml:"linkType"`
	Owner                string   `yaml:"owner,omitempty"`
	Group                string   `yaml:"group,omitempty"`
	AllowTypeReplacement bool     `yaml:"allowTypeReplacement,omitempty"`
}

// Validate rejects link intent that the provider cannot safely converge.
func (l LinkResource) Validate() error {
	if strings.TrimSpace(l.Name) == "" {
		return fmt.Errorf("link resource missing name")
	}
	path := filepath.Clean(strings.TrimSpace(l.Path))
	if !filepath.IsAbs(path) || path == string(filepath.Separator) {
		return fmt.Errorf("link %q path must be an absolute non-root path", l.Name)
	}
	if l.Lifecycle != LifecyclePresent && l.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("link %q lifecycle must be present or absent", l.Name)
	}
	if l.Lifecycle == LifecycleAbsent {
		return nil
	}
	if strings.TrimSpace(l.Target) == "" {
		return fmt.Errorf("link %q target is required when present", l.Name)
	}
	switch l.LinkType {
	case LinkTypeSymbolic:
	case LinkTypeHard:
		if !filepath.IsAbs(filepath.Clean(l.Target)) {
			return fmt.Errorf("hard link %q target must be absolute", l.Name)
		}
		if l.Owner != "" || l.Group != "" {
			return fmt.Errorf("hard link %q cannot manage owner or group independently", l.Name)
		}
	default:
		return fmt.Errorf("link %q has unsupported linkType %q", l.Name, l.LinkType)
	}
	return nil
}
