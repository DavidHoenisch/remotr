package models

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// SudoDefault models one typed Defaults directive inside a sudoers.d fragment.
type SudoDefault struct {
	Scope  string   `yaml:"scope,omitempty"`
	Target string   `yaml:"target,omitempty"`
	Option string   `yaml:"option"`
	Value  string   `yaml:"value,omitempty"`
	Values []string `yaml:"values,omitempty"`
	Negate bool     `yaml:"negate,omitempty"`
}

// SudoCommandSpec models one structured command entry with explicit argument
// constraints and optional negation.
type SudoCommandSpec struct {
	Path   string   `yaml:"path"`
	Args   []string `yaml:"args,omitempty"`
	NoArgs bool     `yaml:"noArgs,omitempty"`
	Negate bool     `yaml:"negate,omitempty"`
}

// SudoRule models one user/group privilege rule inside a multi-rule sudoers.d
// fragment.
type SudoRule struct {
	Subjects     []string          `yaml:"subjects"`
	RunAs        []string          `yaml:"runAs,omitempty"`
	Commands     []string          `yaml:"commands,omitempty"`
	CommandSpecs []SudoCommandSpec `yaml:"commandSpecs,omitempty"`
	Tags         []string          `yaml:"tags,omitempty"`
}

// SudoResource owns a named fragment in sudoers.d. It intentionally models
// subjects, run-as identities, commands, tags, and allowlisted Defaults
// directives instead of accepting raw sudoers text.
type SudoResource struct {
	ResourceMeta              `yaml:",inline"`
	Name                      string            `yaml:"name"`
	Priority                  *int              `yaml:"priority,omitempty"`
	Subjects                  []string          `yaml:"subjects,omitempty"`
	RunAs                     []string          `yaml:"runAs,omitempty"`
	Commands                  []string          `yaml:"commands,omitempty"`
	CommandSpecs              []SudoCommandSpec `yaml:"commandSpecs,omitempty"`
	Tags                      []string          `yaml:"tags,omitempty"`
	Defaults                  []SudoDefault     `yaml:"defaults,omitempty"`
	Rules                     []SudoRule        `yaml:"rules,omitempty"`
	RecoveryPrincipals        []string          `yaml:"recoveryPrincipals,omitempty"`
	AuditUnmanaged            bool              `yaml:"auditUnmanaged,omitempty"`
	AllowedUnmanagedFragments []string          `yaml:"allowedUnmanagedFragments,omitempty"`
}

// FragmentFilename returns the effective filename managed inside sudoers.d.
func (r SudoResource) FragmentFilename() string {
	if r.Priority != nil {
		return fmt.Sprintf("%02d-%s", *r.Priority, r.Name)
	}
	return r.Name
}

// EffectiveRules returns the normalized privilege rules declared by the
// resource, whether expressed via top-level single-rule fields or rules[].
func (r SudoResource) EffectiveRules() []SudoRule {
	if len(r.Rules) > 0 {
		return r.Rules
	}
	if len(r.Subjects) == 0 && len(r.RunAs) == 0 && len(r.Commands) == 0 && len(r.CommandSpecs) == 0 && len(r.Tags) == 0 {
		return nil
	}
	return []SudoRule{{
		Subjects:     r.Subjects,
		RunAs:        r.RunAs,
		Commands:     r.Commands,
		CommandSpecs: r.CommandSpecs,
		Tags:         r.Tags,
	}}
}

// Validate ensures a resource can safely own one sudoers.d fragment and has
// an explicit local recovery identity before access-risk enforcement.
func (r SudoResource) Validate() error {
	if !validSudoFragmentName(r.Name) {
		return fmt.Errorf("sudo resource requires a safe named fragment")
	}
	if r.Priority != nil && (*r.Priority < 0 || *r.Priority > 99) {
		return fmt.Errorf("sudo %q priority %d must be between 0 and 99", r.Name, *r.Priority)
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
	hasTopLevelRule := len(r.Subjects) != 0 || len(r.RunAs) != 0 || len(r.Commands) != 0 || len(r.CommandSpecs) != 0 || len(r.Tags) != 0
	if r.Lifecycle == LifecycleAbsent {
		if hasTopLevelRule || len(r.Rules) != 0 || len(r.Defaults) != 0 || r.AuditUnmanaged || len(r.AllowedUnmanagedFragments) != 0 {
			return fmt.Errorf("sudo %q absent lifecycle must not declare policy fields", r.Name)
		}
		return nil
	}
	if len(r.AllowedUnmanagedFragments) > 0 && !r.AuditUnmanaged {
		return fmt.Errorf("sudo %q allowedUnmanagedFragments requires auditUnmanaged: true", r.Name)
	}
	seenAllowed := map[string]struct{}{}
	for _, allowed := range r.AllowedUnmanagedFragments {
		if !validSudoFragmentName(allowed) {
			return fmt.Errorf("sudo %q has invalid allowed unmanaged fragment %q", r.Name, allowed)
		}
		if _, dup := seenAllowed[allowed]; dup {
			return fmt.Errorf("sudo %q has duplicate allowed unmanaged fragment %q", r.Name, allowed)
		}
		seenAllowed[allowed] = struct{}{}
	}
	if hasTopLevelRule && len(r.Rules) > 0 {
		return fmt.Errorf("sudo %q cannot declare both top-level rule fields and rules", r.Name)
	}
	if !hasTopLevelRule && len(r.Rules) == 0 && len(r.Defaults) == 0 {
		return fmt.Errorf("sudo %q requires at least one default or rule", r.Name)
	}
	for i, d := range r.Defaults {
		if err := validateSudoDefault(r.Name, i, d); err != nil {
			return err
		}
	}
	for i, rule := range r.EffectiveRules() {
		if err := validateSudoRule(r.Name, i, rule); err != nil {
			return err
		}
	}
	return nil
}

func validateSudoRule(name string, index int, rule SudoRule) error {
	if len(rule.Subjects) == 0 || (len(rule.Commands) == 0 && len(rule.CommandSpecs) == 0) {
		return fmt.Errorf("sudo %q rule %d requires subjects and commands", name, index+1)
	}
	for _, subject := range rule.Subjects {
		if !validSudoSubject(subject) {
			return fmt.Errorf("sudo %q has invalid subject %q", name, subject)
		}
	}
	for _, runAs := range rule.RunAs {
		if !validSudoRunAs(runAs) {
			return fmt.Errorf("sudo %q has invalid runAs target %q", name, runAs)
		}
	}
	for _, command := range rule.Commands {
		command = strings.TrimSpace(command)
		if command != "ALL" && !strings.HasPrefix(command, "/") {
			return fmt.Errorf("sudo %q command %q must be ALL or an absolute path", name, command)
		}
		if strings.ContainsAny(command, ",\r\n\x00") {
			return fmt.Errorf("sudo %q command contains unsafe delimiter", name)
		}
	}
	for _, spec := range rule.CommandSpecs {
		if err := validateSudoCommandSpec(name, spec); err != nil {
			return err
		}
	}
	for _, tag := range rule.Tags {
		if !validSudoTag(tag) {
			return fmt.Errorf("sudo %q has unsupported tag %q", name, tag)
		}
	}
	return nil
}

func validateSudoCommandSpec(name string, spec SudoCommandSpec) error {
	if !validSudoAbsoluteCommandPath(spec.Path) {
		return fmt.Errorf("sudo %q commandSpec path %q must be a safe absolute path", name, spec.Path)
	}
	if spec.NoArgs && len(spec.Args) > 0 {
		return fmt.Errorf("sudo %q commandSpec %q cannot set both args and noArgs", name, spec.Path)
	}
	for _, arg := range spec.Args {
		if arg == "" || strings.ContainsAny(arg, " \t,:=\"'\\\r\n\x00") {
			return fmt.Errorf("sudo %q commandSpec %q has unsafe argument %q", name, spec.Path, arg)
		}
	}
	return nil
}

func validateSudoDefault(name string, index int, d SudoDefault) error {
	scope := d.Scope
	if scope == "" {
		scope = "global"
	}
	switch scope {
	case "global":
		if d.Target != "" {
			return fmt.Errorf("sudo %q default %d global scope must not set target", name, index+1)
		}
	case "user":
		if !validSudoSubject(d.Target) {
			return fmt.Errorf("sudo %q default %d has invalid user scope target %q", name, index+1, d.Target)
		}
	case "runAs":
		if !validSudoRunAs(d.Target) {
			return fmt.Errorf("sudo %q default %d has invalid runAs scope target %q", name, index+1, d.Target)
		}
	case "command":
		if !validSudoAbsoluteCommandPath(d.Target) {
			return fmt.Errorf("sudo %q default %d has invalid command scope target %q", name, index+1, d.Target)
		}
	default:
		return fmt.Errorf("sudo %q default %d has unsupported scope %q", name, index+1, d.Scope)
	}

	switch d.Option {
	case "use_pty", "env_reset", "requiretty", "insults", "mail_badpass", "log_input", "log_output", "pwfeedback", "rootpw", "targetpw", "stay_setuid", "noexec", "fqdn":
		if d.Value != "" || len(d.Values) > 0 {
			return fmt.Errorf("sudo %q default %q is a boolean flag and must not set value or values", name, d.Option)
		}
	case "timestamp_timeout", "passwd_tries", "passwd_timeout":
		if d.Negate || len(d.Values) > 0 {
			return fmt.Errorf("sudo %q default %q must set an integer value without negate or values", name, d.Option)
		}
		if d.Value == "" || strings.TrimSpace(d.Value) != d.Value {
			return fmt.Errorf("sudo %q default %q requires an integer value", name, d.Option)
		}
		n, err := strconv.Atoi(d.Value)
		if err != nil || n < 0 {
			return fmt.Errorf("sudo %q default %q value %q must be a non-negative integer", name, d.Option, d.Value)
		}
	case "logfile", "iolog_dir":
		if d.Negate || len(d.Values) > 0 || !validSudoAbsoluteCommandPath(d.Value) {
			return fmt.Errorf("sudo %q default %q requires a safe absolute path value", name, d.Option)
		}
	case "secure_path":
		if d.Negate || len(d.Values) > 0 || d.Value == "" {
			return fmt.Errorf("sudo %q default secure_path requires a colon-separated path value", name)
		}
		for _, segment := range strings.Split(d.Value, ":") {
			if !validSudoAbsoluteCommandPath(segment) {
				return fmt.Errorf("sudo %q default secure_path has unsafe segment %q", name, segment)
			}
		}
	case "umask":
		if d.Negate || len(d.Values) > 0 || !validSudoOctalUmask(d.Value) {
			return fmt.Errorf("sudo %q default umask requires a 4-digit octal value (0000-0777)", name)
		}
	case "lecture":
		if d.Negate || len(d.Values) > 0 || (d.Value != "always" && d.Value != "once" && d.Value != "never") {
			return fmt.Errorf("sudo %q default lecture must be always, once, or never", name)
		}
	case "timestamp_type":
		if d.Negate || len(d.Values) > 0 || (d.Value != "global" && d.Value != "ppid" && d.Value != "tty" && d.Value != "kernel") {
			return fmt.Errorf("sudo %q default timestamp_type must be global, ppid, tty, or kernel", name)
		}
	case "env_keep", "env_delete":
		if d.Negate || d.Value != "" || len(d.Values) == 0 {
			return fmt.Errorf("sudo %q default %q requires non-empty values without value or negate", name, d.Option)
		}
		for _, envName := range d.Values {
			if !validSudoEnvVarName(envName) {
				return fmt.Errorf("sudo %q default %q has invalid environment variable %q", name, d.Option, envName)
			}
		}
	default:
		return fmt.Errorf("sudo %q has unsupported default option %q", name, d.Option)
	}
	return nil
}

func validSudoTag(tag string) bool {
	switch tag {
	case "NOPASSWD", "PASSWD", "NOEXEC", "EXEC", "SETENV", "NOSETENV", "LOG_INPUT", "NOLOG_INPUT", "LOG_OUTPUT", "NOLOG_OUTPUT", "MAIL", "NOMAIL", "INTERCEPT", "NOINTERCEPT", "FOLLOW", "NOFOLLOW":
		return true
	default:
		return false
	}
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

func validSudoRunAs(value string) bool {
	if value == "ALL" {
		return true
	}
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		if len(parts) != 2 {
			return false
		}
		userPart, groupPart := parts[0], parts[1]
		if userPart == "" && groupPart == "" {
			return false
		}
		if userPart != "" && userPart != "ALL" && !validSudoSubject(userPart) {
			return false
		}
		if groupPart == "" || (groupPart != "ALL" && !validLocalAccountName(groupPart)) {
			return false
		}
		return true
	}
	return validSudoSubject(value)
}

func validSudoAbsoluteCommandPath(value string) bool {
	if !strings.HasPrefix(value, "/") || len(value) < 2 {
		return false
	}
	return !strings.ContainsAny(value, " \t,:=\"'\\*?[]\r\n\x00")
}

func validSudoOctalUmask(value string) bool {
	if len(value) != 4 || value[0] != '0' {
		return false
	}
	for i := 1; i < 4; i++ {
		if value[i] < '0' || value[i] > '7' {
			return false
		}
	}
	return true
}

func validSudoEnvVarName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isLetter {
			return false
		}
		if !isLetter && !isDigit {
			return false
		}
	}
	return true
}
