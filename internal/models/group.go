package models

import (
	"fmt"
	"strings"
)

// GroupResource manages one local Unix group.
type GroupResource struct {
	ResourceMeta         `yaml:",inline"`
	Name                 string `yaml:"name"`
	Group                string `yaml:"group"`
	GID                  int    `yaml:"gid,omitempty"`
	System               *bool  `yaml:"system,omitempty"`
	AllowGIDReassignment bool   `yaml:"allowGIDReassignment,omitempty"`
}

// Validate rejects group settings that cannot be safely converged.
func (g GroupResource) Validate() error {
	if strings.TrimSpace(g.Name) == "" || strings.TrimSpace(g.Group) == "" {
		return fmt.Errorf("group resource requires name and group")
	}
	if !validLocalAccountName(g.Group) {
		return fmt.Errorf("group %q has invalid group name", g.Name)
	}
	if g.Lifecycle != LifecyclePresent && g.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("group %q lifecycle must be present or absent", g.Name)
	}
	if g.GID < 0 {
		return fmt.Errorf("group %q gid must be positive when specified", g.Name)
	}
	if g.AllowGIDReassignment && g.GID == 0 {
		return fmt.Errorf("group %q allowGIDReassignment requires gid", g.Name)
	}
	if g.System != nil && g.GID == 0 {
		return fmt.Errorf("group %q system class requires a fixed gid", g.Name)
	}
	return nil
}

func validLocalAccountName(value string) bool {
	if value == "" || len(value) > 32 || value == "." || value == ".." || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
