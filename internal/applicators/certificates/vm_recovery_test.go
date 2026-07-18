//go:build vmsafety

package certificates_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/applicators/certificates"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// TestCertificateSecretInterruptedRecoveryVM runs in two processes separated
// by the harness's controlled Ubuntu reboot. It proves encrypted private-key
// recovery and exact rollback after reconstruction, then exercises the only
// authorized path for abandoning a secret version retained by that recovery.
func TestCertificateSecretInterruptedRecoveryVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM certificate recovery test must run as root")
	}
	phase := os.Getenv("REMOTR_CERTIFICATE_VM_PHASE")
	stateDir := os.Getenv("REMOTR_CERTIFICATE_VM_STATE_DIR")
	if phase == "" || stateDir == "" {
		t.Fatal("REMOTR_CERTIFICATE_VM_PHASE and REMOTR_CERTIFICATE_VM_STATE_DIR are required")
	}

	ctx := context.Background()
	managedDir := filepath.Join(stateDir, "managed")
	materialDir := filepath.Join(stateDir, "material")
	rollbackRoot := filepath.Join(stateDir, "transactions")
	certificatePath := filepath.Join(managedDir, "service.crt")
	privateKeyPath := filepath.Join(managedDir, "service.key")
	resource := models.CertificateResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-secret-certificate", CertificatePath: certificatePath, PrivateKeyPath: privateKeyPath,
		CertificateRef: "remotr:certificates/vm-service@active", PrivateKeyRef: "remotr:private-keys/vm-service@active",
		Subject: "CN=vm-secret.example.test", SANs: []string{"vm-secret.example.test"}, RenewBefore: "72h",
		CertificateMode: []int{0o640}, PrivateKeyMode: []int{0o600}, Owner: "root", Group: "root",
	}
	canary := testsupport.SecretCanary("ubuntu-certificate-recovery")

	switch phase {
	case "prepare":
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(materialDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(managedDir, 0o700); err != nil {
			t.Fatal(err)
		}
		previousCertificate, previousKey := testCertificatePair(t, "vm-secret.example.test", time.Now().Add(time.Hour))
		previousKey = append(previousKey, []byte("# "+canary+"\n")...)
		desiredCertificate, desiredKey := testCertificatePair(t, "vm-secret.example.test", time.Now().Add(90*24*time.Hour))
		vmWriteSecretFile(t, certificatePath, previousCertificate, 0o640)
		vmWriteSecretFile(t, privateKeyPath, previousKey, 0o600)
		vmWriteSecretFile(t, filepath.Join(materialDir, "desired.crt"), desiredCertificate, 0o600)
		vmWriteSecretFile(t, filepath.Join(materialDir, "desired.key"), desiredKey, 0o600)

		store := vmCertificateRollbackStore(t, rollbackRoot)
		if report := store.Protection(); report.Class != rollbackstore.ProtectionRootFile || !report.ReducedProtection || report.KeyID == "" {
			t.Fatalf("Ubuntu certificate rollback protection = %+v", report)
		}
		provider := vmCertificateProvider(t, resource, store, materialDir)
		if err := provider.PreflightRollback(ctx); err != nil {
			t.Fatalf("certificate rollback preflight: %v", err)
		}
		result := provider.ApplyResult(ctx)
		if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
			t.Fatalf("ApplyResult = %+v, want changed transactional", result)
		}
		if check := provider.Check(ctx); check.Status != executor.Compliant {
			t.Fatalf("post-Apply Check = %+v, want compliant", check)
		}
		records, err := store.Records(ctx, "certificate.vm-secret-certificate")
		if err != nil || len(records) != 1 || !records[0].Armed || !records[0].Sensitive || !records[0].PayloadAvailable {
			t.Fatalf("protected certificate records = %+v, err=%v", records, err)
		}
		vmAssertCertificateTreeExcludes(t, rollbackRoot, []byte(canary), previousKey)
	case "verify":
		store := vmCertificateRollbackStore(t, rollbackRoot)
		provider := vmCertificateProvider(t, resource, store, materialDir)
		if err := provider.Revert(ctx); err != nil {
			t.Fatalf("restart certificate rollback: %v", err)
		}
		key, err := os.ReadFile(privateKeyPath)
		if err != nil || !bytes.Contains(key, []byte(canary)) {
			t.Fatalf("trusted recovery boundary did not restore the exact canary-bearing key: err=%v", err)
		}
		if check := provider.Check(ctx); check.Status != executor.Drifted {
			t.Fatalf("second Check after secret rollback = %+v, want drifted", check)
		}
		if err := provider.Revert(ctx); !errors.Is(err, appErr.ErrNoOp) {
			t.Fatalf("second certificate rollback = %v, want no replay", err)
		}
		vmAssertCertificateTreeExcludes(t, rollbackRoot, []byte(canary))
		vmProveAuthorizedRecoveryAbandonment(t, canary)
		clear(key)
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown VM certificate recovery phase %q", phase)
	}
}

func TestCertificateRecoveryAbandonmentContractVMFixture(t *testing.T) {
	vmProveAuthorizedRecoveryAbandonment(t, testsupport.SecretCanary("ubuntu-certificate-abandonment-contract"))
}

func vmCertificateRollbackStore(t *testing.T, root string) *rollbackstore.Store {
	t.Helper()
	store, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func vmCertificateProvider(t *testing.T, resource models.CertificateResource, store *rollbackstore.Store, materialDir string) *certificates.Applicator {
	t.Helper()
	provider := certificates.New(resource)
	provider.Resolve = func(_ context.Context, reference string) ([]byte, error) {
		if strings.Contains(reference, "private-keys") {
			return os.ReadFile(filepath.Join(materialDir, "desired.key"))
		}
		return os.ReadFile(filepath.Join(materialDir, "desired.crt"))
	}
	if err := provider.ConfigureRollback(store, "certificate.vm-secret-certificate", "sha256:vm-certificate"); err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmWriteSecretFile(t *testing.T, path string, body []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatal(err)
	}
}

func vmAssertCertificateTreeExcludes(t *testing.T, root string, secretsToExclude ...[]byte) {
	t.Helper()
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range secretsToExclude {
			if len(secret) > 0 && bytes.Contains(raw, secret) {
				t.Fatalf("protected certificate payload appeared in %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func vmProveAuthorizedRecoveryAbandonment(t *testing.T, canary string) {
	t.Helper()
	ctx := context.Background()
	repository := secrets.NewMemoryVersionRepository()
	keyring, err := secrets.NewKeyring("vm-kek", map[string][]byte{"vm-kek": bytes.Repeat([]byte{0x5a}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := secrets.NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	service, err := secrets.NewRegistryService(repository, envelope, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, material := range []string{canary + "-prior", canary + "-replacement"} {
		if _, err := service.Upload(ctx, secrets.UploadRequest{Name: "certificates/vm-service", Fleet: "vm", Material: []byte(material), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Activate(ctx, secrets.ActivationRequest{Name: "certificates/vm-service", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	reference, err := service.RetainRollbackReference(ctx, secrets.RollbackReferenceRequest{
		Name: "certificates/vm-service", Version: "1", ResourceAddress: "certificate/vm-secret-certificate",
		ArtifactDigest: "sha256:vm-certificate", Attempt: 1, ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(ctx, secrets.ActivationRequest{Name: "certificates/vm-service", Version: "2", ActorID: "operator-2"}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteVersion(ctx, secrets.DeleteVersionRequest{Name: "certificates/vm-service", Version: "1", ActorID: "operator-2"}); !errors.Is(err, secrets.ErrVersionReferenced) {
		t.Fatalf("ordinary deletion = %v, want retained recovery refusal", err)
	}
	if err := service.DeleteVersion(ctx, secrets.DeleteVersionRequest{Name: "certificates/vm-service", Version: "1", ActorID: "operator-2", AbandonRecovery: true}); !errors.Is(err, secrets.ErrRecoveryAbandonmentUnauthorized) {
		t.Fatalf("unauthorized abandonment = %v", err)
	}
	authorized, err := secrets.NewRegistryService(repository, envelope, nil, nil, secrets.WithRecoveryAbandonmentAuthorizer(vmAbandonAuthorizer{"operator-3": true}))
	if err != nil {
		t.Fatal(err)
	}
	if err := authorized.DeleteVersion(ctx, secrets.DeleteVersionRequest{Name: "certificates/vm-service", Version: "1", ActorID: "operator-3", AbandonRecovery: true}); err != nil {
		t.Fatalf("authorized abandonment: %v", err)
	}
	encoded, err := json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(canary)) {
		t.Fatalf("recovery metadata exposed secret canary: %s", encoded)
	}
}

type vmAbandonAuthorizer map[string]bool

func (a vmAbandonAuthorizer) AuthorizeRecoveryAbandonment(_ context.Context, request secrets.RecoveryAbandonmentRequest) bool {
	return a[request.ActorID]
}
