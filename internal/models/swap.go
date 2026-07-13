package models

import (
	"fmt"
	"path/filepath"
)

func (r SwapResource) Validate() error {
	if !mountResourceName.MatchString(r.Name) || !filepath.IsAbs(r.Path) || filepath.Clean(r.Path) != r.Path {
		return fmt.Errorf("swap name or path is invalid")
	}
	if r.Type != "file" && r.Type != "device" {
		return fmt.Errorf("swap type %q is invalid", r.Type)
	}
	if r.Type == "file" && r.SizeBytes <= 0 {
		return fmt.Errorf("swap file requires positive sizeBytes")
	}
	if r.Type == "device" && r.SizeBytes != 0 {
		return fmt.Errorf("swap device cannot declare sizeBytes")
	}
	if r.Active == nil && r.Persistent == nil && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("swap requires active or persistent state")
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("swap lifecycle %q is unsupported", r.Lifecycle)
	}
	if r.Active != nil && !*r.Active && !r.AllowRemove {
		return fmt.Errorf("swap removal requires allowRemove")
	}
	return nil
}

func (r SwapResource) DesiredPersistent() (bool, bool) {
	if r.Lifecycle == LifecycleAbsent {
		return false, true
	}
	if r.Persistent == nil {
		return false, false
	}
	return *r.Persistent, true
}
