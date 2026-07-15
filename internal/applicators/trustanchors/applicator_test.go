package trustanchors_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/applicators/trustanchors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-LSM-027: absence removes only the named owned anchor and multiple anchor
// mutations coalesce to one distro-native trust-store refresh.
func TestApplicatorRemovesOnlyNamedAnchorAndCoalescesRefresh(t *testing.T) {
	dir := t.TempDir()
	managedPath := filepath.Join(dir, "remotr-corporate.crt")
	otherPath := filepath.Join(dir, "unrelated.crt")
	if err := os.WriteFile(managedPath, []byte("managed-anchor"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherPath, []byte("unrelated-anchor"), 0o644); err != nil {
		t.Fatal(err)
	}
	applicator, err := trustanchors.New(models.TrustAnchorResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "corporate",
	}, types.Debian)
	if err != nil {
		t.Fatal(err)
	}
	applicator.AnchorsDir = dir

	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if _, err := os.Stat(managedPath); !os.IsNotExist(err) {
		t.Fatalf("managed anchor still exists: %v", err)
	}
	if got, err := os.ReadFile(otherPath); err != nil || string(got) != "unrelated-anchor" {
		t.Fatalf("unrelated anchor = %q err=%v", got, err)
	}
	plan := executor.CollectActivations([]executor.ApplyResult{result, result})
	want := []executor.ActivationSignal{{Kind: executor.ActivationTrustStoreRefresh, Target: "debian"}}
	if !slices.Equal(plan, want) {
		t.Fatalf("coalesced refresh plan = %+v, want %+v", plan, want)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
}

func TestApplicatorVerifiesFingerprintBeforeInstallingNamedAnchor(t *testing.T) {
	certificatePEM, fingerprint := testAnchor(t, "Corporate Root")
	dir := t.TempDir()
	path := filepath.Join(dir, "remotr-corporate.crt")
	if err := os.WriteFile(path, []byte("previous-anchor"), 0o644); err != nil {
		t.Fatal(err)
	}
	resource := models.TrustAnchorResource{
		Name: "corporate", AnchorRef: "remotr:trust-anchors/corporate@active", Fingerprint: fingerprint,
	}
	applicator, err := trustanchors.New(resource, types.Ubuntu)
	if err != nil {
		t.Fatal(err)
	}
	applicator.AnchorsDir = dir
	applicator.Resolve = func(context.Context, string) ([]byte, error) { return append([]byte(nil), certificatePEM...), nil }

	applicator.Resource.Fingerprint = "sha256:" + strings.Repeat("0", 64)
	if err := applicator.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("mismatched Apply() error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "previous-anchor" {
		t.Fatalf("active anchor changed after mismatch: %q err=%v", got, err)
	}

	applicator.Resource.Fingerprint = fingerprint
	if result := applicator.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant || !strings.Contains(string(check.ObservedSummary), fingerprint) {
		t.Fatalf("second Check() = %+v", check)
	}
}

func testAnchor(t *testing.T, commonName string) ([]byte, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: commonName}, IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(der)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), "sha256:" + hex.EncodeToString(digest[:])
}
