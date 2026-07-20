package aptkeys_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/aptkeys"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const testGPGHome = "/tmp/remotr-apt-key-test-home"

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
	dearmorCalls := 0
	applicator.Dearmor = func(context.Context, []byte) ([]byte, error) {
		dearmorCalls++
		return []byte("untrusted-keyring"), nil
	}

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
	if dearmorCalls != 0 {
		t.Fatalf("mismatched source key reached dearmor boundary %d times", dearmorCalls)
	}
}

func TestApplicator_acceptsCompletePrimaryFingerprintWithoutConfusingSubkey(t *testing.T) {
	const (
		primary = "0123456789ABCDEF0123456789ABCDEF01234567"
		subkey  = "89ABCDEF0123456789ABCDEF0123456789ABCDEF"
	)
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --with-colons --import-options show-only --dry-run --import]": {
			Stdout: []byte("pub:-:255:1:KEYID:0:0::::::\n" +
				"fpr:::::::::" + primary + ":\n" +
				"sub:-:255:1:SUBKEY:0:0::::::\n" +
				"fpr:::::::::" + subkey + ":\n"),
		},
		"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --dearmor --output -]": {Stdout: []byte("binary-keyring")},
	}}
	keyringDir := t.TempDir()
	applicator := aptkeys.New(models.APTSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: primary,
	}, runner)
	applicator.KeyringsDir = keyringDir
	applicator.GPGHomeDir = testGPGHome
	applicator.Fetch = func(context.Context, string) ([]byte, error) { return []byte("armored-key-with-subkey"), nil }

	if err := applicator.Apply(t.Context()); err != nil {
		t.Fatalf("Apply() = %v, want complete primary fingerprint accepted", err)
	}
	if got, err := os.ReadFile(filepath.Join(keyringDir, "vendor.gpg")); err != nil || string(got) != "binary-keyring" {
		t.Fatalf("installed keyring = %q, %v", got, err)
	}
}

func TestApplicator_rejectsDearmoredKeyringWhoseCompleteFingerprintChanged(t *testing.T) {
	const (
		declared = "0123456789ABCDEF0123456789ABCDEF01234567"
		changed  = "1123456789ABCDEF0123456789ABCDEF01234567"
	)
	keyring := filepath.Join(t.TempDir(), "vendor.gpg")
	if err := os.WriteFile(keyring, []byte("previous-keyring"), 0o644); err != nil {
		t.Fatal(err)
	}
	applicator := aptkeys.New(models.APTSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: declared,
	}, nil)
	applicator.KeyringsDir = filepath.Dir(keyring)
	applicator.Fetch = func(context.Context, string) ([]byte, error) { return []byte("armored-key"), nil }
	applicator.Dearmor = func(context.Context, []byte) ([]byte, error) { return []byte("changed-keyring"), nil }
	applicator.Fingerprint = func(_ context.Context, material []byte) (string, error) {
		if string(material) == "armored-key" {
			return declared, nil
		}
		return changed, nil
	}

	if err := applicator.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "dearmored fingerprint mismatch") {
		t.Fatalf("Apply() = %v, want complete dearmored fingerprint mismatch", err)
	}
	if got, err := os.ReadFile(keyring); err != nil || string(got) != "previous-keyring" {
		t.Fatalf("keyring after mismatch = %q, %v; want previous content", got, err)
	}
}

func TestApplicator_rejectsMalformedPrimaryFingerprintBeforeDearmor(t *testing.T) {
	const declared = "0123456789ABCDEF0123456789ABCDEF01234567"
	keyring := filepath.Join(t.TempDir(), "vendor.gpg")
	if err := os.WriteFile(keyring, []byte("previous-keyring"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --with-colons --import-options show-only --dry-run --import]": {
			Stdout: []byte("pub:-:255:1:KEYID:0:0::::::\nfpr:::::::::not-a-complete-fingerprint:\n"),
		},
	}}
	applicator := aptkeys.New(models.APTSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: declared,
	}, runner)
	applicator.KeyringsDir = filepath.Dir(keyring)
	applicator.GPGHomeDir = testGPGHome
	applicator.Fetch = func(context.Context, string) ([]byte, error) { return []byte("malformed-key"), nil }

	if err := applicator.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "primary OpenPGP fingerprint is malformed") {
		t.Fatalf("Apply() = %v, want malformed primary fingerprint failure", err)
	}
	if len(runner.Inputs) != 1 || !slices.Equal(runner.Inputs[0].Args, []string{"--homedir", testGPGHome, "--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import"}) {
		t.Fatalf("malformed-key GPG calls = %#v, want inspection only", runner.Inputs)
	}
	if got, err := os.ReadFile(keyring); err != nil || string(got) != "previous-keyring" {
		t.Fatalf("keyring after malformed input = %q, %v; want previous content", got, err)
	}
}

func TestApplicator_installsScopedKeyringUsingProtectedGPGInput(t *testing.T) {
	const fingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"
	keyringDir := t.TempDir()
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --with-colons --import-options show-only --dry-run --import]": {
			Stdout: []byte("pub:-:255:1:KEYID:0:0::::::\nfpr:::::::::" + fingerprint + ":\n"),
		},
		"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --dearmor --output -]": {Stdout: []byte("binary-keyring")},
	}}
	applicator := aptkeys.New(models.APTSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: fingerprint,
	}, runner)
	applicator.KeyringsDir = keyringDir
	applicator.GPGHomeDir = testGPGHome
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
	if len(runner.Inputs) != 4 {
		t.Fatalf("GPG input calls = %#v, want Apply fingerprint/dearmor/reinspection plus post-Apply check", runner.Inputs)
	}
	for _, input := range runner.Inputs[:2] {
		if string(input.Input) != "armored-key" {
			t.Fatalf("GPG input = %q, want key material passed on stdin", input.Input)
		}
	}
	if !slices.Equal(runner.Inputs[0].Args, []string{"--homedir", testGPGHome, "--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import"}) {
		t.Fatalf("fingerprint argv = %#v", runner.Inputs[0].Args)
	}
	if !slices.Equal(runner.Inputs[1].Args, []string{"--homedir", testGPGHome, "--batch", "--dearmor", "--output", "-"}) {
		t.Fatalf("dearmor argv = %#v", runner.Inputs[1].Args)
	}
	if string(runner.Inputs[2].Input) != "binary-keyring" || !slices.Equal(runner.Inputs[2].Args, runner.Inputs[0].Args) {
		t.Fatalf("dearmored fingerprint verification = %#v", runner.Inputs[2])
	}
	for _, call := range runner.Calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("unexpected shell invocation: %#v", call)
		}
	}
}

func TestApplicator_nativeGPGFailuresAreSanitizedAndDoNotPersistTrust(t *testing.T) {
	const (
		fingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"
		canary      = "apt-key-native-secret-canary"
	)
	tests := []struct {
		name string
		next map[string]executil.MockResult
	}{
		{"fingerprint inspection", map[string]executil.MockResult{
			"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --with-colons --import-options show-only --dry-run --import]": {Stderr: []byte(canary), Err: errors.New(canary)},
		}},
		{"dearmor", map[string]executil.MockResult{
			"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --with-colons --import-options show-only --dry-run --import]": {Stdout: []byte("pub:-:255:1:KEYID:0:0::::::\nfpr:::::::::" + fingerprint + ":\n")},
			"gpg [--homedir /tmp/remotr-apt-key-test-home --batch --dearmor --output -]":                                        {Stderr: []byte(canary), Err: errors.New(canary)},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keyringDir := t.TempDir()
			provider := aptkeys.New(models.APTSigningKey{
				Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: fingerprint,
			}, &executil.MockRunner{Next: test.next})
			provider.KeyringsDir = keyringDir
			provider.GPGHomeDir = testGPGHome
			provider.Fetch = func(context.Context, string) ([]byte, error) { return []byte("protected-key-material"), nil }
			result := provider.ApplyResult(t.Context())
			if result.Status != executor.Failed || result.Err == nil || strings.Contains(result.Err.Error(), canary) {
				t.Fatalf("ApplyResult() = %+v, want sanitized native GPG failure", result)
			}
			if _, err := os.Stat(filepath.Join(keyringDir, "vendor.gpg")); !os.IsNotExist(err) {
				t.Fatalf("failed native trust persisted keyring: %v", err)
			}
		})
	}
}

func TestApplicator_defaultsToSanitizedProcessRunner(t *testing.T) {
	provider := aptkeys.New(models.APTSigningKey{Name: "vendor"}, nil)
	if _, ok := provider.Runner.(executil.SanitizedOSRunner); !ok {
		t.Fatalf("default runner = %T, want SanitizedOSRunner", provider.Runner)
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

func TestApplicatorProtectedRollbackSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	const fingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"
	dir := t.TempDir()
	path := filepath.Join(dir, "vendor.gpg")
	if err := os.WriteFile(path, []byte("previous-keyring"), 0o600); err != nil {
		t.Fatal(err)
	}
	resource := models.APTSigningKey{Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: fingerprint}
	rollbackRoot := filepath.Join(dir, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	first := aptkeys.New(resource, nil)
	first.KeyringsDir = dir
	first.Fetch = func(context.Context, string) ([]byte, error) { return []byte("armored"), nil }
	first.Fingerprint = func(context.Context, []byte) (string, error) { return fingerprint, nil }
	first.Dearmor = func(context.Context, []byte) ([]byte, error) { return []byte("replacement-keyring"), nil }
	if err := first.ConfigureRollback(store, "base/vendor-key", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if result := first.ApplyResult(ctx); result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v", result)
	}

	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := aptkeys.New(resource, nil)
	restarted.KeyringsDir = dir
	if err := restarted.ConfigureRollback(restartedStore, "base/vendor-key", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "previous-keyring" {
		t.Fatalf("restored keyring = %q, %v", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("restored mode = %v, %v", info.Mode().Perm(), err)
	}
}
