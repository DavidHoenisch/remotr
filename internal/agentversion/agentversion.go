package agentversion

import (
	"fmt"
	"strings"
)

// Normalize returns a canonical tag form (with leading v).
func Normalize(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("version is required")
	}
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return "", fmt.Errorf("invalid version")
	}
	return "v" + s, nil
}

// Match reports whether reported satisfies desired (both normalized).
func Match(desired, reported string) bool {
	d, err := Normalize(desired)
	if err != nil {
		return false
	}
	r, err := Normalize(reported)
	if err != nil {
		return false
	}
	return d == r
}
