package certificates_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/applicators/certificates"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider {
			return certificateContractProvider(t, models.LifecyclePresent, true)
		},
		Drifted: func(t *testing.T) contract.Provider {
			return certificateContractProvider(t, models.LifecyclePresent, false)
		},
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent: func(t *testing.T) contract.Provider {
			return certificateContractProvider(t, models.LifecycleAbsent, false)
		},
		Present: func(t *testing.T) contract.Provider {
			return certificateContractProvider(t, models.LifecycleAbsent, true)
		},
	})
}

func certificateContractProvider(t *testing.T, lifecycle models.Lifecycle, installed bool) contract.Provider {
	t.Helper()
	dir := t.TempDir()
	certificatePath, keyPath := filepath.Join(dir, "service.crt"), filepath.Join(dir, "service.key")
	certificatePEM, keyPEM := testCertificatePair(t, "service.example.test", time.Now().Add(90*24*time.Hour))
	if installed {
		if err := os.WriteFile(certificatePath, certificatePEM, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resource := models.CertificateResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle}, Name: "service",
		CertificatePath: certificatePath, PrivateKeyPath: keyPath,
	}
	if lifecycle == models.LifecyclePresent {
		resource.CertificateRef = "remotr:certificates/service@active"
		resource.PrivateKeyRef = "remotr:private-keys/service@active"
		resource.Subject = "CN=service.example.test"
		resource.SANs = []string{"service.example.test"}
		resource.RenewalPolicy = models.CertificateRenewalProvider
	}
	provider := certificates.New(resource)
	provider.Resolve = func(_ context.Context, reference string) ([]byte, error) {
		if strings.Contains(reference, "private-keys") {
			return append([]byte(nil), keyPEM...), nil
		}
		return append([]byte(nil), certificatePEM...), nil
	}
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
