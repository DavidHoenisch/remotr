// Package secretref validates provider-neutral references without resolving
// or handling the referenced material.
package secretref

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	ProviderLocalFile = "local-file"
	ProviderRemotr    = "remotr"
	SelectorActive    = "active"
)

var ErrInvalid = errors.New("invalid secret reference")

var remotrName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

type Reference struct {
	Provider string
	Name     string
	Selector string
}

func (r Reference) String() string {
	if r.Provider == ProviderRemotr {
		return r.Provider + ":" + r.Name + "@" + r.Selector
	}
	return r.Provider + ":" + r.Name
}

func (r Reference) FollowsActive() bool {
	return r.Provider == ProviderRemotr && r.Selector == SelectorActive
}

// Parse returns the canonical provider and provider-owned identifier. The
// legacy file: spelling remains readable as local-file during migration.
func Parse(raw string) (provider, identifier string, err error) {
	reference, err := ParseSelected(raw)
	if err != nil {
		return "", "", err
	}
	identifier = reference.Name
	if reference.Provider == ProviderRemotr {
		identifier += "@" + reference.Selector
	}
	return reference.Provider, identifier, nil
}

// ParseSelected validates a provider reference and separates a Remotr logical
// name from its mandatory exact-or-active version selector.
func ParseSelected(raw string) (Reference, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\x00\r\n") {
		return Reference{}, ErrInvalid
	}
	provider, identifier, ok := strings.Cut(raw, ":")
	if !ok || identifier == "" {
		return Reference{}, ErrInvalid
	}
	switch provider {
	case "file":
		provider = ProviderLocalFile
	case ProviderLocalFile, ProviderRemotr:
	default:
		return Reference{}, fmt.Errorf("%w: provider %q is not supported", ErrInvalid, provider)
	}
	if provider == ProviderLocalFile {
		if !filepath.IsAbs(identifier) || filepath.Clean(identifier) != identifier {
			return Reference{}, fmt.Errorf("%w: local-file path must be clean and absolute", ErrInvalid)
		}
		return Reference{Provider: provider, Name: identifier}, nil
	}
	if strings.Count(identifier, "@") != 1 {
		return Reference{}, fmt.Errorf("%w: remotr reference requires an explicit version or active selector", ErrInvalid)
	}
	name, selector, _ := strings.Cut(identifier, "@")
	if !remotrName.MatchString(name) || strings.Contains(name, "..") {
		return Reference{}, fmt.Errorf("%w: remotr identifier is invalid", ErrInvalid)
	}
	if selector != SelectorActive {
		if selector == "" || selector[0] == '0' || strings.ContainsAny(selector, "+-") {
			return Reference{}, fmt.Errorf("%w: remotr version selector is invalid", ErrInvalid)
		}
		version, err := strconv.ParseInt(selector, 10, 64)
		if err != nil || version <= 0 {
			return Reference{}, fmt.Errorf("%w: remotr version selector is invalid", ErrInvalid)
		}
	}
	return Reference{Provider: provider, Name: name, Selector: selector}, nil
}

func Validate(raw string) error {
	_, _, err := Parse(raw)
	return err
}
