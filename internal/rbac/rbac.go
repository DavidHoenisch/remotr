package rbac

import "strings"

const (
	RoleGlobalAdmin    = "global_admin"
	RoleReadOnly       = "read_only"
	RoleSecurityLogger = "security_logger"
)

// Rule grants access to requests matching method and path pattern.
// Method may be "*" for any method. Path patterns support a trailing "*" prefix match.
type Rule struct {
	ID          string
	Method      string
	PathPattern string
}

// Role is a named collection of access rules.
type Role struct {
	Name        string
	Description string
	BuiltIn     bool
	Rules       []Rule
}

// Match reports whether an HTTP request matches a rule.
func Match(method, path, ruleMethod, rulePattern string) bool {
	if ruleMethod != "*" && !strings.EqualFold(ruleMethod, method) {
		return false
	}
	if rulePattern == "*" {
		return true
	}
	if strings.HasSuffix(rulePattern, "*") {
		prefix := strings.TrimSuffix(rulePattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return path == rulePattern
}

// Allow reports whether any rule grants access.
func Allow(rules []Rule, method, path string) bool {
	for _, rule := range rules {
		if Match(method, path, rule.Method, rule.PathPattern) {
			return true
		}
	}
	return false
}

// BuiltInRole returns metadata and compiled rules for a built-in role.
func BuiltInRole(name string) (Role, bool) {
	meta, ok := builtInRoleMeta[name]
	if !ok {
		return Role{}, false
	}
	rules, ok := builtInRoleRules[name]
	if !ok {
		return Role{}, false
	}
	return Role{
		Name:        name,
		Description: meta.Description,
		BuiltIn:     true,
		Rules:       append([]Rule(nil), rules...),
	}, true
}

// BuiltInRoles returns all built-in role definitions.
func BuiltInRoles() []Role {
	out := make([]Role, 0, len(builtInRoleMeta))
	for name := range builtInRoleMeta {
		role, _ := BuiltInRole(name)
		out = append(out, role)
	}
	return out
}

// IsBuiltInRole reports whether name is a reserved built-in role.
func IsBuiltInRole(name string) bool {
	_, ok := builtInRoleMeta[name]
	return ok
}

// ValidateRoleName checks custom role naming rules.
func ValidateRoleName(name string) error {
	if name == "" {
		return errEmptyRoleName
	}
	if strings.ContainsAny(name, " \t/") {
		return errInvalidRoleName
	}
	return nil
}

var (
	errEmptyRoleName   = errString("role name required")
	errInvalidRoleName = errString("invalid role name")
)

type errString string

func (e errString) Error() string { return string(e) }
