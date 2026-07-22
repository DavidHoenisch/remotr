package secrets

import (
	"fmt"
	"strings"
)

// Scope identifies the authorization boundary for one logical secret.
// Global is deliberately explicit; an omitted scope is never interpreted as
// global.
type Scope string

const (
	ScopeGlobal   Scope = "global"
	ScopeFleet    Scope = "fleet"
	ScopeEndpoint Scope = "endpoint"
)

// ParseScope validates a canonical API/schema scope. It does not perform the
// compatibility inference used when reading older Fleet/Endpoint records.
func ParseScope(scope, fleet, endpointID string) (Scope, error) {
	kind, _, _, err := normalizeScope(Scope(scope), fleet, endpointID, false)
	return kind, err
}

func normalizeScope(scope Scope, fleet, endpointID string, allowLegacy bool) (Scope, string, string, error) {
	if fleet != strings.TrimSpace(fleet) || endpointID != strings.TrimSpace(endpointID) {
		return "", "", "", fmt.Errorf("secret scope identifier is invalid")
	}
	if len(fleet) > 256 || len(endpointID) > 256 || strings.ContainsAny(fleet+endpointID, "\x00\r\n") {
		return "", "", "", fmt.Errorf("secret scope exceeds bounds")
	}
	if scope == "" && allowLegacy {
		switch {
		case fleet != "" && endpointID == "":
			scope = ScopeFleet
		case endpointID != "" && fleet == "":
			scope = ScopeEndpoint
		}
	}
	switch scope {
	case ScopeGlobal:
		if fleet != "" || endpointID != "" {
			return "", "", "", fmt.Errorf("global secret scope does not accept an identifier")
		}
	case ScopeFleet:
		if fleet == "" || endpointID != "" {
			return "", "", "", fmt.Errorf("fleet secret scope requires exactly one Fleet identifier")
		}
	case ScopeEndpoint:
		if endpointID == "" || fleet != "" {
			return "", "", "", fmt.Errorf("endpoint secret scope requires exactly one endpoint identifier")
		}
	default:
		return "", "", "", fmt.Errorf("secret scope must be global, fleet, or endpoint")
	}
	return scope, fleet, endpointID, nil
}
