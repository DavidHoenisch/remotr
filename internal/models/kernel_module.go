package models

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var kernelModuleName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var kernelModuleParameter = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var kernelModuleValue = regexp.MustCompile(`^[A-Za-z0-9._,:+/@=-]+$`)

// KernelModuleResource manages independently optional current loaded state,
// boot-time loading, module parameters, and a module blacklist. Boolean
// pointers preserve the difference between an unmanaged field and an explicit
// false value.
type KernelModuleResource struct {
	ResourceMeta     `yaml:",inline"`
	Name             string            `yaml:"name"`
	Module           string            `yaml:"module,omitempty"`
	Loaded           *bool             `yaml:"loaded,omitempty"`
	Persistent       *bool             `yaml:"persistent,omitempty"`
	Parameters       map[string]string `yaml:"parameters,omitempty"`
	Blacklisted      *bool             `yaml:"blacklisted,omitempty"`
	ProtectedModules []string          `yaml:"protectedModules,omitempty"`
}

// Validate rejects ambiguous module state and unsafe values before a module
// provider can write an owned fragment or invoke modprobe.
func (r KernelModuleResource) Validate() error {
	if !kernelModuleName.MatchString(r.Name) {
		return fmt.Errorf("kernel module name %q is invalid", r.Name)
	}
	if !kernelModuleName.MatchString(r.Module) {
		return fmt.Errorf("kernel module %q is invalid", r.Module)
	}
	if r.Loaded == nil && r.Persistent == nil && r.Parameters == nil && r.Blacklisted == nil {
		return fmt.Errorf("kernel module requires at least one managed field")
	}
	if r.Blacklisted != nil && *r.Blacklisted && r.Loaded != nil && *r.Loaded {
		return fmt.Errorf("kernel module cannot be both loaded and blacklisted")
	}
	for key, value := range r.Parameters {
		if !kernelModuleParameter.MatchString(key) {
			return fmt.Errorf("kernel module parameter %q is invalid", key)
		}
		if !kernelModuleValue.MatchString(value) {
			return fmt.Errorf("kernel module parameter %q has an unsafe value", key)
		}
	}
	seen := make(map[string]struct{}, len(r.ProtectedModules))
	for _, module := range r.ProtectedModules {
		if !kernelModuleName.MatchString(module) {
			return fmt.Errorf("protected kernel module %q is invalid", module)
		}
		module = normalizeKernelModule(module)
		if _, exists := seen[module]; exists {
			return fmt.Errorf("protected kernel module %q is duplicated", module)
		}
		seen[module] = struct{}{}
	}
	return nil
}

// ParameterNames returns a stable list for deterministic rendering and argv.
func (r KernelModuleResource) ParameterNames() []string {
	names := make([]string, 0, len(r.Parameters))
	for name := range r.Parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeKernelModule(module string) string {
	return strings.ReplaceAll(module, "-", "_")
}
