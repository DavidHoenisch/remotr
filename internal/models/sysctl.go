package models

import (
	"fmt"
	"regexp"
	"strings"
)

// SysctlActivation controls when a persistent value reaches the live kernel.
type SysctlActivation string

const (
	SysctlSingleKey SysctlActivation = "single-key"
	SysctlReload    SysctlActivation = "reload"
	SysctlNextBoot  SysctlActivation = "next-boot"
)

var sysctlKey = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)+$`)
var sysctlName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Validate ensures every accepted sysctl field has an unambiguous provider
// behavior. next-boot deliberately avoids changing a managed live value.
func (s SysctlResource) Validate() error {
	if !sysctlName.MatchString(s.Name) {
		return fmt.Errorf("sysctl name %q must contain only lowercase letters, digits, '.', '_' or '-'", s.Name)
	}
	if !sysctlKey.MatchString(s.Key) {
		return fmt.Errorf("sysctl key %q is invalid", s.Key)
	}
	if strings.TrimSpace(s.Value) == "" || strings.ContainsAny(s.Value, "\r\n") {
		return fmt.Errorf("sysctl value must be a non-empty single line")
	}
	if !s.Runtime && !s.Persistent {
		return fmt.Errorf("sysctl requires runtime or persistent scope")
	}
	if s.Activation == "" {
		s.Activation = SysctlSingleKey
	}
	switch s.Activation {
	case SysctlSingleKey, SysctlReload:
	case SysctlNextBoot:
		if s.Runtime {
			return fmt.Errorf("sysctl next-boot activation cannot manage runtime scope")
		}
		if !s.Persistent {
			return fmt.Errorf("sysctl next-boot activation requires persistent scope")
		}
	default:
		return fmt.Errorf("sysctl activation %q is unsupported", s.Activation)
	}
	return nil
}
