package models

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/secretref"
)

type CertificateRenewalPolicy string

const (
	CertificateRenewalProvider CertificateRenewalPolicy = "provider"
	CertificateRenewalManual   CertificateRenewalPolicy = "manual"
)

// CertificateResource manages a certificate and its matching private key as
// one lifecycle unit. Material is always supplied through provider-neutral
// references; it is never embedded in desired state.
type CertificateResource struct {
	ResourceMeta    `yaml:",inline"`
	Name            string                   `yaml:"name"`
	CertificatePath string                   `yaml:"certificatePath"`
	PrivateKeyPath  string                   `yaml:"privateKeyPath"`
	CertificateRef  string                   `yaml:"certificateRef,omitempty"`
	PrivateKeyRef   string                   `yaml:"privateKeyRef,omitempty"`
	ChainRefs       []string                 `yaml:"chainRefs,omitempty"`
	Subject         string                   `yaml:"subject,omitempty"`
	SANs            []string                 `yaml:"sans,omitempty"`
	Fingerprint     string                   `yaml:"fingerprint,omitempty"`
	RenewBefore     string                   `yaml:"renewBefore,omitempty"`
	RenewalPolicy   CertificateRenewalPolicy `yaml:"renewalPolicy,omitempty"`
	Owner           string                   `yaml:"owner,omitempty"`
	Group           string                   `yaml:"group,omitempty"`
	CertificateMode []int                    `yaml:"certificateMode,omitempty"`
	PrivateKeyMode  []int                    `yaml:"privateKeyMode,omitempty"`
}

func (r CertificateResource) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("certificate name is required")
	}
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("certificate lifecycle must be present or absent")
	}
	if !filepath.IsAbs(r.CertificatePath) || !filepath.IsAbs(r.PrivateKeyPath) || filepath.Clean(r.CertificatePath) == filepath.Clean(r.PrivateKeyPath) {
		return fmt.Errorf("certificatePath and privateKeyPath must be distinct absolute paths")
	}
	if lifecycle == LifecyclePresent {
		if _, err := secretref.ParseSelected(r.CertificateRef); err != nil {
			return fmt.Errorf("certificateRef: %w", err)
		}
		if _, err := secretref.ParseSelected(r.PrivateKeyRef); err != nil {
			return fmt.Errorf("privateKeyRef: %w", err)
		}
		for i, reference := range r.ChainRefs {
			if _, err := secretref.ParseSelected(reference); err != nil {
				return fmt.Errorf("chainRefs[%d]: %w", i, err)
			}
		}
	} else if r.CertificateRef != "" || r.PrivateKeyRef != "" || len(r.ChainRefs) > 0 {
		return fmt.Errorf("absent certificate must not declare provider material")
	}
	if r.RenewBefore != "" {
		threshold, err := time.ParseDuration(r.RenewBefore)
		if err != nil || threshold <= 0 {
			return fmt.Errorf("renewBefore must be a positive duration")
		}
	}
	switch r.RenewalPolicy {
	case "", CertificateRenewalProvider, CertificateRenewalManual:
	default:
		return fmt.Errorf("certificate renewalPolicy must be provider or manual")
	}
	if r.Fingerprint != "" {
		fingerprint := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(r.Fingerprint)), "sha256:")
		if len(fingerprint) != 64 {
			return fmt.Errorf("certificate fingerprint must be a SHA-256 digest")
		}
		if _, err := hex.DecodeString(fingerprint); err != nil {
			return fmt.Errorf("certificate fingerprint must be a SHA-256 digest")
		}
	}
	if err := validateMode("certificateMode", r.CertificateMode, false); err != nil {
		return err
	}
	if err := validateMode("privateKeyMode", r.PrivateKeyMode, true); err != nil {
		return err
	}
	for _, value := range append([]string{r.Owner, r.Group}, r.SANs...) {
		if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("certificate identity fields must be trimmed and contain no control characters")
		}
	}
	return nil
}

func validateMode(field string, values []int, private bool) error {
	if len(values) > 1 || (len(values) == 1 && (values[0] < 0 || values[0] > 0o777)) {
		return fmt.Errorf("%s must contain one POSIX mode", field)
	}
	if private && len(values) == 1 && values[0]&0o077 != 0 {
		return fmt.Errorf("privateKeyMode must not grant group or other access")
	}
	return nil
}
