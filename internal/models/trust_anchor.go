package models

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/secretref"
)

var trustAnchorNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)

type TrustAnchorResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	AnchorRef    string `yaml:"anchorRef,omitempty"`
	Fingerprint  string `yaml:"fingerprint,omitempty"`
}

func (r TrustAnchorResource) Validate() error {
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if !trustAnchorNamePattern.MatchString(r.Name) {
		return fmt.Errorf("trust anchor name must be a safe named-fragment identifier")
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("trust anchor lifecycle must be present or absent")
	}
	if lifecycle == LifecycleAbsent {
		if r.AnchorRef != "" || r.Fingerprint != "" {
			return fmt.Errorf("absent trust anchor must omit anchorRef and fingerprint")
		}
		return nil
	}
	if _, err := secretref.ParseSelected(r.AnchorRef); err != nil {
		return fmt.Errorf("anchorRef: %w", err)
	}
	fingerprint := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(r.Fingerprint)), "sha256:")
	if len(fingerprint) != 64 {
		return fmt.Errorf("trust anchor fingerprint must be a SHA-256 digest")
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return fmt.Errorf("trust anchor fingerprint must be a SHA-256 digest")
	}
	return nil
}
