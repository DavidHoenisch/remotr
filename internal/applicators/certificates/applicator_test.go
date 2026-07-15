package certificates_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/applicators/certificates"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-LSM-026: staged certificate and private-key material is validated as a
// pair before either active path is replaced.
func TestApplicatorRejectsMismatchedPairBeforeReplacingActiveFiles(t *testing.T) {
	dir := t.TempDir()
	certificatePath := filepath.Join(dir, "service.crt")
	keyPath := filepath.Join(dir, "service.key")
	if err := os.WriteFile(certificatePath, []byte("previous-certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("previous-private-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	certificatePEM, _ := testCertificatePair(t, "service.example.test", time.Now().Add(90*24*time.Hour))
	_, mismatchedKeyPEM := testCertificatePair(t, "other.example.test", time.Now().Add(90*24*time.Hour))

	applicator := certificates.New(models.CertificateResource{
		Name: "service", CertificatePath: certificatePath, PrivateKeyPath: keyPath,
		CertificateRef: "remotr:certificates/service@active", PrivateKeyRef: "remotr:private-keys/service@active",
	})
	applicator.Resolve = func(_ context.Context, reference string) ([]byte, error) {
		if strings.Contains(reference, "private-keys") {
			return mismatchedKeyPEM, nil
		}
		return certificatePEM, nil
	}

	if err := applicator.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("Apply() error = %v, want pair mismatch", err)
	}
	assertFileBody(t, certificatePath, "previous-certificate")
	assertFileBody(t, keyPath, "previous-private-key")
}

// OS-LSM-025: expiry drift is observable through safe metadata; Apply obtains
// renewed material through the provider and the second Check is compliant.
func TestApplicatorRenewsExpiringCertificateReportsSafeStateAndRollsBack(t *testing.T) {
	dir := t.TempDir()
	certificatePath := filepath.Join(dir, "service.crt")
	keyPath := filepath.Join(dir, "service.key")
	expiringCertificate, expiringKey := testCertificatePair(t, "service.example.test", time.Now().Add(12*time.Hour))
	renewedCertificate, renewedKey := testCertificatePair(t, "service.example.test", time.Now().Add(90*24*time.Hour))
	if err := os.WriteFile(certificatePath, expiringCertificate, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, expiringKey, 0o600); err != nil {
		t.Fatal(err)
	}

	applicator := certificates.New(models.CertificateResource{
		ResourceMeta: models.ResourceMeta{Notifications: []models.Notification{{Type: models.NotificationReload, Target: "service.service"}}},
		Name:         "service", CertificatePath: certificatePath, PrivateKeyPath: keyPath,
		CertificateRef: "remotr:certificates/service@active", PrivateKeyRef: "remotr:private-keys/service@active",
		Subject: "CN=service.example.test", SANs: []string{"service.example.test"}, RenewBefore: "72h",
		CertificateMode: []int{0o640}, PrivateKeyMode: []int{0o600},
	})
	applicator.Now = func() time.Time { return time.Now() }
	applicator.Resolve = func(_ context.Context, reference string) ([]byte, error) {
		if strings.Contains(reference, "private-keys") {
			return append([]byte(nil), renewedKey...), nil
		}
		return append([]byte(nil), renewedCertificate...), nil
	}

	check := applicator.Check(context.Background())
	if check.Status != executor.Drifted || !strings.Contains(string(check.ObservedSummary), "sha256:") || !strings.Contains(string(check.ObservedSummary), "expires=") || strings.Contains(string(check.ObservedSummary), string(expiringKey)) {
		t.Fatalf("expiring Check() = %+v", check)
	}
	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || !slices.Equal(result.Activation, []executor.ActivationSignal{{Kind: executor.ActivationReload, Target: "service.service"}}) {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	if info, err := os.Stat(certificatePath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("certificate mode = %v err=%v", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(keyPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private-key mode = %v err=%v", info.Mode().Perm(), err)
	}
	if err := applicator.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(certificatePath); err != nil || !slices.Equal(got, expiringCertificate) {
		t.Fatalf("rollback certificate differs: err=%v", err)
	}
	if got, err := os.ReadFile(keyPath); err != nil || !slices.Equal(got, expiringKey) {
		t.Fatalf("rollback private key differs: err=%v", err)
	}
}

func testCertificatePair(t *testing.T, commonName string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: commonName}, DNSNames: []string{commonName},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func assertFileBody(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
