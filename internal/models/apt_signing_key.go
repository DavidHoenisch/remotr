package models

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var aptResourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var aptKeyFingerprint = regexp.MustCompile(`^[A-F0-9]{40}([A-F0-9]{24})?$`)
var aptRepositoryToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Validate rejects ambiguous keyring names, non-HTTPS key sources, and
// fingerprints that cannot be compared reliably. Apt keys support only an
// explicit present/absent lifecycle in this first provider slice.
func (k APTSigningKey) Validate() error {
	if !aptResourceName.MatchString(k.Name) {
		return fmt.Errorf("APT signing key name %q must contain only lowercase letters, digits, '.', '_' or '-'", k.Name)
	}
	if k.Lifecycle == "" {
		k.Lifecycle = LifecyclePresent
	}
	if k.Lifecycle != LifecyclePresent && k.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("APT signing key lifecycle %q is unsupported", k.Lifecycle)
	}
	if k.Lifecycle == LifecycleAbsent {
		return nil
	}
	u, err := url.Parse(k.Source)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("APT signing key source must be an unauthenticated HTTPS URL")
	}
	if !aptKeyFingerprint.MatchString(strings.ToUpper(strings.ReplaceAll(k.Fingerprint, " ", ""))) {
		return fmt.Errorf("APT signing key fingerprint must be a 40- or 64-character hexadecimal OpenPGP fingerprint")
	}
	return nil
}

// NormalizedFingerprint returns the comparable canonical fingerprint form.
func (k APTSigningKey) NormalizedFingerprint() string {
	return strings.ToUpper(strings.ReplaceAll(k.Fingerprint, " ", ""))
}

// Validate accepts only canonical, independently manageable APT source fields.
func (r APTRepository) Validate() error {
	if !aptResourceName.MatchString(r.Name) {
		return fmt.Errorf("APT repository name %q must contain only lowercase letters, digits, '.', '_' or '-'", r.Name)
	}
	if r.Lifecycle == "" {
		r.Lifecycle = LifecyclePresent
	}
	if r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleDisabled && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("APT repository lifecycle %q is unsupported", r.Lifecycle)
	}
	if r.Lifecycle == LifecycleAbsent {
		return nil
	}
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("APT repository URL must be an unauthenticated HTTP(S) URL without query or fragment")
	}
	if len(r.Suites) == 0 || len(r.Components) == 0 {
		return fmt.Errorf("APT repository requires at least one suite and component")
	}
	for _, group := range [][]string{r.Suites, r.Components, r.Architectures} {
		seen := make(map[string]struct{}, len(group))
		for _, value := range group {
			if !aptRepositoryToken.MatchString(value) {
				return fmt.Errorf("APT repository has invalid token %q", value)
			}
			if _, duplicate := seen[value]; duplicate {
				return fmt.Errorf("APT repository repeats token %q", value)
			}
			seen[value] = struct{}{}
		}
	}
	if !aptResourceName.MatchString(r.SigningKey) {
		return fmt.Errorf("APT repository signingKey %q is invalid", r.SigningKey)
	}
	if r.Priority < -10000 || r.Priority > 10000 {
		return fmt.Errorf("APT repository priority %d is outside the supported range", r.Priority)
	}
	if r.CredentialRef != "" && !strings.HasPrefix(r.CredentialRef, "file:/") {
		return fmt.Errorf("APT repository credentialRef must be a file:/ secret reference")
	}
	return nil
}
