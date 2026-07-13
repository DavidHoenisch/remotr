package models

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var aptKeyResourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var aptKeyFingerprint = regexp.MustCompile(`^[A-F0-9]{40}([A-F0-9]{24})?$`)

// Validate rejects ambiguous keyring names, non-HTTPS key sources, and
// fingerprints that cannot be compared reliably. Apt keys support only an
// explicit present/absent lifecycle in this first provider slice.
func (k APTSigningKey) Validate() error {
	if !aptKeyResourceName.MatchString(k.Name) {
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
