package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const (
	endpointIDMaxLen   = 63
	endpointIDMinLen   = 4
	endpointIDSuffixLen = 8
)

// ValidateEndpointID checks an endpoint identifier used in certs, the registry,
// and endpoints/<id>/desired.yaml paths. Legacy UUIDs and hostname-style slugs
// (alphanumeric plus hyphens) are accepted.
func ValidateEndpointID(id string) error {
	if id == "" {
		return fmt.Errorf("id required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("invalid id")
	}
	if len(id) < endpointIDMinLen || len(id) > endpointIDMaxLen {
		return fmt.Errorf("invalid id")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			continue
		default:
			return fmt.Errorf("invalid id")
		}
	}
	return nil
}

// ResolveEndpointID returns requested when set (after validation), otherwise a
// default derived from the machine hostname plus a random suffix.
func ResolveEndpointID(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if err := ValidateEndpointID(requested); err != nil {
			return "", err
		}
		return requested, nil
	}
	return DefaultEndpointID()
}

// DefaultEndpointID builds <sanitized-hostname>-<8 hex chars> from os.Hostname().
func DefaultEndpointID() (string, error) {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "host"
	}
	suffix, err := randomHex(endpointIDSuffixLen / 2)
	if err != nil {
		return "", err
	}
	base := SanitizeHostname(host)
	id := base + "-" + suffix
	if err := ValidateEndpointID(id); err != nil {
		return "", err
	}
	return id, nil
}

// RandomEndpointID returns <prefix>-<8 hex chars> for server-side fallback assignment.
func RandomEndpointID(prefix string) (string, error) {
	prefix = SanitizeHostname(prefix)
	if prefix == "" {
		prefix = "ep"
	}
	suffix, err := randomHex(endpointIDSuffixLen / 2)
	if err != nil {
		return "", err
	}
	id := prefix + "-" + suffix
	if err := ValidateEndpointID(id); err != nil {
		return "", err
	}
	return id, nil
}

// SanitizeHostname lowercases a hostname or FQDN into a slug-safe label.
func SanitizeHostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	var b strings.Builder
	lastHyphen := false
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == '.', r == '-', r == '_':
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "host"
	}
	const maxHostPart = 48
	if len(s) > maxHostPart {
		s = strings.TrimRight(s[:maxHostPart], "-")
	}
	return s
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
