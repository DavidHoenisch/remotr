package models

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/secretref"
)

var pacmanArchitecture = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_+-]*$`)

type PacmanSignatureLevel string

const (
	PacmanSignatureRequired                 PacmanSignatureLevel = "required"
	PacmanSignatureRequiredDatabaseOptional PacmanSignatureLevel = "required-database-optional"
)

func (level PacmanSignatureLevel) Valid() bool {
	return level == PacmanSignatureRequired || level == PacmanSignatureRequiredDatabaseOptional
}

// PacmanSigningKey declares one narrowly owned provider-native trust identity.
type PacmanSigningKey struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Source       string `yaml:"source,omitempty"`
	Fingerprint  string `yaml:"fingerprint,omitempty"`
}

func (key PacmanSigningKey) Validate() error {
	if !aptResourceName.MatchString(key.Name) {
		return fmt.Errorf("Pacman signing key name %q must contain only lowercase letters, digits, '.', '_' or '-'", key.Name)
	}
	if key.Ownership != "" && key.Ownership != OwnershipNamed {
		return fmt.Errorf("Pacman signing key ownership must be %q", OwnershipNamed)
	}
	lifecycle := key.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("Pacman signing key lifecycle %q is unsupported", lifecycle)
	}
	if lifecycle == LifecycleAbsent {
		return nil
	}
	parsed, err := url.Parse(key.Source)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("Pacman signing key source must be an unauthenticated HTTPS URL without query or fragment")
	}
	if !aptKeyFingerprint.MatchString(strings.ToUpper(strings.ReplaceAll(key.Fingerprint, " ", ""))) {
		return fmt.Errorf("Pacman signing key fingerprint must be a 40- or 64-character hexadecimal OpenPGP fingerprint")
	}
	return nil
}

func (key PacmanSigningKey) NormalizedFingerprint() string {
	return strings.ToUpper(strings.ReplaceAll(key.Fingerprint, " ", ""))
}

// PacmanRepository declares one Remotr-owned repository fragment and its
// explicit trust dependencies.
type PacmanRepository struct {
	ResourceMeta   `yaml:",inline"`
	Name           string               `yaml:"name"`
	Servers        []string             `yaml:"servers,omitempty"`
	Architecture   string               `yaml:"architecture,omitempty"`
	SignatureLevel PacmanSignatureLevel `yaml:"signatureLevel,omitempty"`
	SigningKeys    []string             `yaml:"signingKeys,omitempty"`
	CredentialRef  string               `yaml:"credentialRef,omitempty"`
}

func (repository PacmanRepository) Validate() error {
	if !aptResourceName.MatchString(repository.Name) {
		return fmt.Errorf("Pacman repository name %q must contain only lowercase letters, digits, '.', '_' or '-'", repository.Name)
	}
	if repository.Ownership != "" && repository.Ownership != OwnershipFragment {
		return fmt.Errorf("Pacman repository ownership must be %q", OwnershipFragment)
	}
	lifecycle := repository.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleDisabled && lifecycle != LifecycleAbsent {
		return fmt.Errorf("Pacman repository lifecycle %q is unsupported", lifecycle)
	}
	if lifecycle == LifecycleAbsent {
		return nil
	}
	if len(repository.Servers) == 0 || len(repository.Servers) > 16 {
		return fmt.Errorf("Pacman repository requires between 1 and 16 servers")
	}
	seenServers := make(map[string]struct{}, len(repository.Servers))
	for _, server := range repository.Servers {
		parsed, err := url.Parse(server)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("Pacman repository server must be an unauthenticated HTTP(S) URL without query or fragment")
		}
		if _, duplicate := seenServers[server]; duplicate {
			return fmt.Errorf("Pacman repository repeats server %q", server)
		}
		seenServers[server] = struct{}{}
	}
	if !pacmanArchitecture.MatchString(repository.Architecture) {
		return fmt.Errorf("Pacman repository architecture %q is invalid", repository.Architecture)
	}
	if !repository.SignatureLevel.Valid() {
		return fmt.Errorf("Pacman repository signatureLevel %q is unsupported", repository.SignatureLevel)
	}
	if len(repository.SigningKeys) == 0 || len(repository.SigningKeys) > 16 {
		return fmt.Errorf("Pacman repository requires between 1 and 16 signing keys")
	}
	seenKeys := make(map[string]struct{}, len(repository.SigningKeys))
	for _, key := range repository.SigningKeys {
		if !aptResourceName.MatchString(key) {
			return fmt.Errorf("Pacman repository signing key %q is invalid", key)
		}
		if _, duplicate := seenKeys[key]; duplicate {
			return fmt.Errorf("Pacman repository repeats signing key %q", key)
		}
		seenKeys[key] = struct{}{}
	}
	if repository.CredentialRef != "" {
		if err := secretref.Validate(repository.CredentialRef); err != nil {
			return fmt.Errorf("Pacman repository credentialRef: %w", err)
		}
	}
	return nil
}
