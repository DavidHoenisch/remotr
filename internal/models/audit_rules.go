package models

import (
	"fmt"
	"regexp"
	"strings"
)

var auditFragmentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)

type AuditRulesResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string   `yaml:"name"`
	Rules        []string `yaml:"rules,omitempty"`
}

func (r AuditRulesResource) Validate() error {
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if !auditFragmentName.MatchString(r.Name) {
		return fmt.Errorf("audit rule name must be a safe named-fragment identifier")
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("audit rule lifecycle must be present or absent")
	}
	if lifecycle == LifecycleAbsent {
		if len(r.Rules) != 0 {
			return fmt.Errorf("absent audit rule fragment must omit rules")
		}
		return nil
	}
	if len(r.Rules) == 0 {
		return fmt.Errorf("audit rule fragment requires at least one rule")
	}
	for i, rule := range r.Rules {
		if rule != strings.TrimSpace(rule) || strings.ContainsAny(rule, "\x00\r\n") || !strings.HasPrefix(rule, "-") {
			return fmt.Errorf("audit rule %d must be one trimmed auditctl rule", i+1)
		}
	}
	return nil
}
