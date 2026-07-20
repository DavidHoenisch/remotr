package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestKnownHostFingerprintMustMatchDeclaredOpenSSHKey(t *testing.T) {
	const (
		key         = "AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W"
		fingerprint = "SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4"
	)
	base := models.KnownHostResource{
		Name: "git", Scope: models.KnownHostScopeSystem, Hosts: []string{"git.example"},
		Type: "ssh-ed25519", Key: key, Fingerprint: fingerprint, Hashing: models.KnownHostHashPlain,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipNamed},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid known host: %v", err)
	}

	base.Fingerprint = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if err := base.Validate(); err == nil || !strings.Contains(err.Error(), "does not match key") {
		t.Fatalf("mismatched fingerprint error = %v", err)
	}
}
