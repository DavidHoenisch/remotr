package aptkeys_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/aptkeys"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-PRM-013: a key whose declared fingerprint does not match the decoded
// signing material must not replace the scoped keyring.
func TestApplicator_rejectsFingerprintMismatchWithoutReplacingKeyring(t *testing.T) {
	keyring := filepath.Join(t.TempDir(), "vendor.gpg")
	if err := os.WriteFile(keyring, []byte("previous-keyring"), 0o644); err != nil {
		t.Fatal(err)
	}

	applicator := aptkeys.New(models.APTSigningKey{
		Name:        "vendor",
		Source:      "https://keys.example.test/vendor.asc",
		Fingerprint: "0123456789ABCDEF0123456789ABCDEF01234567",
	}, nil)
	applicator.KeyringsDir = filepath.Dir(keyring)
	applicator.Fetch = func(context.Context, string) ([]byte, error) { return []byte("untrusted-key"), nil }
	applicator.Fingerprint = func(context.Context, []byte) (string, error) { return "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", nil }

	if err := applicator.Apply(context.Background()); err == nil {
		t.Fatal("Apply() succeeded for mismatched signing key fingerprint")
	}
	got, err := os.ReadFile(keyring)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous-keyring" {
		t.Fatalf("keyring was replaced after fingerprint mismatch: %q", got)
	}
}

func TestApplicator_installsScopedKeyringUsingProtectedGPGInput(t *testing.T) {
	const fingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"
	keyringDir := t.TempDir()
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"gpg [--batch --with-colons --import-options show-only --dry-run --import]": {
			Stdout: []byte("fpr:::::::::" + fingerprint + ":\n"),
		},
		"gpg [--batch --dearmor --output -]": {Stdout: []byte("binary-keyring")},
	}}
	applicator := aptkeys.New(models.APTSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: fingerprint,
	}, runner)
	applicator.KeyringsDir = keyringDir
	applicator.Fetch = func(context.Context, string) ([]byte, error) { return []byte("armored-key"), nil }

	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(keyringDir, "vendor.gpg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary-keyring" {
		t.Fatalf("keyring = %q, want dearmored material", got)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() after Apply = %+v, want compliant", check)
	}
	if len(runner.Inputs) != 3 {
		t.Fatalf("GPG input calls = %#v, want Apply fingerprint/dearmor plus post-Apply check", runner.Inputs)
	}
	for _, input := range runner.Inputs[:2] {
		if string(input.Input) != "armored-key" {
			t.Fatalf("GPG input = %q, want key material passed on stdin", input.Input)
		}
	}
	if !slices.Equal(runner.Inputs[0].Args, []string{"--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import"}) {
		t.Fatalf("fingerprint argv = %#v", runner.Inputs[0].Args)
	}
	if !slices.Equal(runner.Inputs[1].Args, []string{"--batch", "--dearmor", "--output", "-"}) {
		t.Fatalf("dearmor argv = %#v", runner.Inputs[1].Args)
	}
	for _, call := range runner.Calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("unexpected shell invocation: %#v", call)
		}
	}
}

func TestApplicator_removesOnlyItsNamedKeyring(t *testing.T) {
	keyringDir := t.TempDir()
	for name, body := range map[string]string{"vendor.gpg": "managed", "other.gpg": "unmanaged"} {
		if err := os.WriteFile(filepath.Join(keyringDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	applicator := aptkeys.New(models.APTSigningKey{
		Name: "vendor", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	}, nil)
	applicator.KeyringsDir = keyringDir
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if _, err := os.Stat(filepath.Join(keyringDir, "vendor.gpg")); !os.IsNotExist(err) {
		t.Fatalf("managed keyring remains after removal: %v", err)
	}
	other, err := os.ReadFile(filepath.Join(keyringDir, "other.gpg"))
	if err != nil || string(other) != "unmanaged" {
		t.Fatalf("unrelated keyring changed: %q, %v", other, err)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() = %+v, want compliant absent key", check)
	}
}

func TestApplicator_reportsMissingPresentKeyringAsDrift(t *testing.T) {
	applicator := aptkeys.New(models.APTSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: "0123456789ABCDEF0123456789ABCDEF01234567",
	}, nil)
	applicator.KeyringsDir = t.TempDir()
	if check := applicator.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("Check() = %+v, want drift for absent desired keyring", check)
	}
}
