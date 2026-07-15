package models

import (
	"fmt"
	"strings"
)

// ValidateTrustAnchorReferences validates stable configuration/resource
// addresses without resolving or embedding certificate material.
func ValidateTrustAnchorReferences(references []string) error {
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		configuration, resource, ok := strings.Cut(reference, "/")
		if !ok || strings.Contains(resource, "/") || configuration == "" || resource == "" ||
			configuration != strings.TrimSpace(configuration) || resource != strings.TrimSpace(resource) ||
			strings.ContainsAny(reference, "\x00\r\n") {
			return fmt.Errorf("reference %q must use a stable <configuration>/<resource-name> address", reference)
		}
		if _, duplicate := seen[reference]; duplicate {
			return fmt.Errorf("duplicate reference %q", reference)
		}
		seen[reference] = struct{}{}
	}
	return nil
}
