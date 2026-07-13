package models

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var mountResourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var mountFilesystem = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
var mountOption = regexp.MustCompile(`^[A-Za-z0-9._=:-]+$`)

// UnmountMode makes potentially disruptive unmount behavior intentional.
type UnmountMode string

const (
	UnmountNormal UnmountMode = "normal"
	UnmountLazy   UnmountMode = "lazy"
	UnmountForce  UnmountMode = "force"
)

func (r MountResource) Validate() error {
	if !mountResourceName.MatchString(r.Name) {
		return fmt.Errorf("mount resource name %q is invalid", r.Name)
	}
	if strings.TrimSpace(r.Source) == "" || strings.ContainsAny(r.Source, "\r\n\t ") {
		return fmt.Errorf("mount source %q is invalid", r.Source)
	}
	if !filepath.IsAbs(r.Target) || filepath.Clean(r.Target) != r.Target {
		return fmt.Errorf("mount target %q must be a clean absolute path", r.Target)
	}
	if !mountFilesystem.MatchString(r.FilesystemType) {
		return fmt.Errorf("mount filesystem type %q is invalid", r.FilesystemType)
	}
	if r.Mounted == nil && r.Persistent == nil && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("mount requires mounted or persistent state")
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("mount lifecycle %q is unsupported", r.Lifecycle)
	}
	if r.Dump < 0 || r.Pass < 0 {
		return fmt.Errorf("mount dump and pass must be non-negative")
	}
	for _, option := range r.Options {
		if !mountOption.MatchString(option) {
			return fmt.Errorf("mount option %q is invalid", option)
		}
	}
	if r.UnmountMode != "" && r.UnmountMode != UnmountNormal && r.UnmountMode != UnmountLazy && r.UnmountMode != UnmountForce {
		return fmt.Errorf("mount unmountMode %q is invalid", r.UnmountMode)
	}
	if r.UnmountMode == UnmountForce && (r.Enforce == nil || !*r.Enforce) {
		return fmt.Errorf("forced unmount requires explicit enforce authorization")
	}
	return nil
}

// NormalizedOptions is the canonical fstab and mount argv option sequence.
// Sorting and deduplication ensure equivalent authored lists converge to one
// declaration rather than producing a perpetual fstab diff.
func (r MountResource) NormalizedOptions() []string {
	options := append([]string(nil), r.Options...)
	slices.Sort(options)
	return slices.Compact(options)
}

func (r MountResource) DesiredPersistent() (bool, bool) {
	if r.Lifecycle == LifecycleAbsent {
		return false, true
	}
	if r.Persistent == nil {
		return false, false
	}
	return *r.Persistent, true
}
