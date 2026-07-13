package models

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SudoResource owns a named fragment in sudoers.d. It intentionally models
// subjects, run-as identities, commands, and tags instead of accepting raw
// sudoers text.
type SudoResource struct {
	ResourceMeta       `yaml:",inline"`
	Name               string   `yaml:"name"`
	Subjects           []string `yaml:"subjects,omitempty"`
	RunAs              []string `yaml:"runAs,omitempty"`
	Commands           []string `yaml:"commands,omitempty"`
	Tags               []string `yaml:"tags,omitempty"`
	RecoveryPrincipals []string `yaml:"recoveryPrincipals,omitempty"`
}

// Validate ensures a resource can safely own one sudoers.d fragment and has
// an explicit local recovery identity before access-risk enforcement.
func (r SudoResource) Validate() error {
	if !validSudoFragmentName(r.Name) {
		return fmt.Errorf("sudo resource requires a safe named fragment")
	}
	if r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("sudo %q lifecycle must be present or absent", r.Name)
	}
	if r.Ownership != OwnershipFragment {
		return fmt.Errorf("sudo %q ownership must be fragment", r.Name)
	}
	if len(r.RecoveryPrincipals) == 0 {
		return fmt.Errorf("sudo %q requires at least one recoveryPrincipal", r.Name)
	}
	seen := map[string]struct{}{}
	for _, principal := range r.RecoveryPrincipals {
		if !validLocalAccountName(principal) {
			return fmt.Errorf("sudo %q has invalid recovery principal %q", r.Name, principal)
		}
		if _, exists := seen[principal]; exists {
			return fmt.Errorf("sudo %q has duplicate recovery principal %q", r.Name, principal)
		}
		seen[principal] = struct{}{}
	}
	if r.Lifecycle == LifecycleAbsent {
		if len(r.Subjects) != 0 || len(r.RunAs) != 0 || len(r.Commands) != 0 || len(r.Tags) != 0 {
			return fmt.Errorf("sudo %q absent lifecycle must not declare policy fields", r.Name)
		}
		return nil
	}
	if len(r.Subjects) == 0 || len(r.Commands) == 0 {
		return fmt.Errorf("sudo %q requires subjects and commands", r.Name)
	}
	for _, subject := range r.Subjects {
		if !validSudoSubject(subject) {
			return fmt.Errorf("sudo %q has invalid subject %q", r.Name, subject)
		}
	}
	for _, runAs := range r.RunAs {
		if runAs != "ALL" && !validSudoSubject(runAs) {
			return fmt.Errorf("sudo %q has invalid runAs target %q", r.Name, runAs)
		}
	}
	for _, command := range r.Commands {
		command = strings.TrimSpace(command)
		if command != "ALL" && !strings.HasPrefix(command, "/") {
			return fmt.Errorf("sudo %q command %q must be ALL or an absolute path", r.Name, command)
		}
		if strings.ContainsAny(command, ",\r\n\x00") {
			return fmt.Errorf("sudo %q command contains unsafe delimiter", r.Name)
		}
	}
	for _, tag := range r.Tags {
		switch tag {
		case "NOPASSWD", "PASSWD", "NOEXEC", "EXEC", "SETENV", "NOSETENV", "LOG_INPUT", "NOLOG_INPUT", "LOG_OUTPUT", "NOLOG_OUTPUT", "MAIL", "NOMAIL", "INTERCEPT", "NOINTERCEPT", "FOLLOW", "NOFOLLOW":
		default:
			return fmt.Errorf("sudo %q has unsupported tag %q", r.Name, tag)
		}
	}
	return nil
}

func validSudoFragmentName(value string) bool {
	if value == "" || filepath.Base(value) != value || strings.Contains(value, ".") {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validSudoSubject(value string) bool {
	value = strings.TrimPrefix(value, "%")
	return validLocalAccountName(value)
}
