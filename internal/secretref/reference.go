// Package secretref validates provider-neutral references without resolving
// or handling the referenced material.
package secretref

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ProviderLocalFile = "local-file"
	ProviderRemotr    = "remotr"
)

var ErrInvalid = errors.New("invalid secret reference")

var remotrName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// Parse returns the canonical provider and provider-owned identifier. The
// legacy file: spelling remains readable as local-file during migration.
func Parse(raw string) (provider, identifier string, err error) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\x00\r\n") {
		return "", "", ErrInvalid
	}
	provider, identifier, ok := strings.Cut(raw, ":")
	if !ok || identifier == "" {
		return "", "", ErrInvalid
	}
	switch provider {
	case "file":
		provider = ProviderLocalFile
	case ProviderLocalFile, ProviderRemotr:
	default:
		return "", "", fmt.Errorf("%w: provider %q is not supported", ErrInvalid, provider)
	}
	if provider == ProviderLocalFile {
		if !filepath.IsAbs(identifier) || filepath.Clean(identifier) != identifier {
			return "", "", fmt.Errorf("%w: local-file path must be clean and absolute", ErrInvalid)
		}
	} else if !remotrName.MatchString(identifier) || strings.Contains(identifier, "..") {
		return "", "", fmt.Errorf("%w: remotr identifier is invalid", ErrInvalid)
	}
	return provider, identifier, nil
}

func Validate(raw string) error {
	_, _, err := Parse(raw)
	return err
}
