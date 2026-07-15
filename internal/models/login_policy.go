package models

import (
	"fmt"
	"regexp"
	"strings"
)

type LoginPolicyProvider string

const (
	LoginPolicyPAMAuthUpdate LoginPolicyProvider = "pam-auth-update"
)

type PAMSection string

const (
	PAMAuth               PAMSection = "auth"
	PAMAccount            PAMSection = "account"
	PAMPassword           PAMSection = "password"
	PAMSession            PAMSection = "session"
	PAMSessionInteractive PAMSection = "session-interactive"
)

var (
	loginPolicyName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)
	pamRecoveryPrincipal = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	pamModuleName        = regexp.MustCompile(`^(?:/[^\s]+/)?pam_[A-Za-z0-9_.-]+\.so$`)
)

type PAMRule struct {
	Section   PAMSection `yaml:"section"`
	Control   string     `yaml:"control"`
	Module    string     `yaml:"module"`
	Arguments []string   `yaml:"arguments,omitempty"`
}

type LoginPolicyResource struct {
	ResourceMeta       `yaml:",inline"`
	Name               string              `yaml:"name"`
	Provider           LoginPolicyProvider `yaml:"provider"`
	Priority           int                 `yaml:"priority,omitempty"`
	RecoveryPrincipals []string            `yaml:"recoveryPrincipals"`
	Rules              []PAMRule           `yaml:"rules,omitempty"`
}

func (r LoginPolicyResource) Validate() error {
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if !loginPolicyName.MatchString(r.Name) {
		return fmt.Errorf("login policy name must be a safe provider-owned identifier")
	}
	if r.Provider != LoginPolicyPAMAuthUpdate {
		if r.Provider == "authselect" {
			return fmt.Errorf("authselect provider is deferred to the RPM-family roadmap")
		}
		return fmt.Errorf("unsupported login policy provider %q", r.Provider)
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("login policy lifecycle must be present or absent")
	}
	priority := r.Priority
	if priority == 0 {
		priority = 900
	}
	if priority < 1 || priority > 10000 {
		return fmt.Errorf("login policy priority must be between 1 and 10000")
	}
	if len(r.RecoveryPrincipals) == 0 {
		return fmt.Errorf("login policy requires at least one recovery principal")
	}
	seenPrincipals := make(map[string]struct{}, len(r.RecoveryPrincipals))
	for _, principal := range r.RecoveryPrincipals {
		if !pamRecoveryPrincipal.MatchString(principal) {
			return fmt.Errorf("login policy recovery principal %q is invalid", principal)
		}
		if _, exists := seenPrincipals[principal]; exists {
			return fmt.Errorf("login policy recovery principal %q is duplicated", principal)
		}
		seenPrincipals[principal] = struct{}{}
	}
	if lifecycle == LifecycleAbsent {
		if len(r.Rules) != 0 {
			return fmt.Errorf("absent login policy must omit PAM rules")
		}
		return nil
	}
	if len(r.Rules) == 0 {
		return fmt.Errorf("login policy requires at least one PAM rule")
	}
	for i, rule := range r.Rules {
		if !rule.Section.valid() {
			return fmt.Errorf("PAM rule %d has invalid section %q", i+1, rule.Section)
		}
		switch rule.Control {
		case "required", "requisite", "sufficient", "optional":
		default:
			return fmt.Errorf("PAM rule %d has unsupported control %q", i+1, rule.Control)
		}
		if !pamModuleName.MatchString(rule.Module) {
			return fmt.Errorf("PAM rule %d has invalid module %q", i+1, rule.Module)
		}
		for _, argument := range rule.Arguments {
			if argument == "" || strings.ContainsAny(argument, " \t\r\n\x00") {
				return fmt.Errorf("PAM rule %d has invalid argument", i+1)
			}
		}
	}
	return nil
}

func (s PAMSection) valid() bool {
	switch s {
	case PAMAuth, PAMAccount, PAMPassword, PAMSession, PAMSessionInteractive:
		return true
	default:
		return false
	}
}
