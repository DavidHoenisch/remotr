package pacmankeys_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/pacmankeys"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const (
	pacmanDeclaredFingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"
	pacmanMismatchFingerprint = "1123456789ABCDEF0123456789ABCDEF01234567"
	testGPGHome               = "/tmp/remotr-pacman-key-test-home"
)

// OS-PRM-022: complete fingerprint verification happens before any mutation
// of the provider-native Pacman keyring.
func TestApplicator_rejectsFingerprintMismatchBeforePacmanKeyMutation(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"gpg [--homedir /tmp/remotr-pacman-key-test-home --batch --with-colons --import-options show-only --dry-run --import]": {
			Stdout: []byte("pub:-:255:1:KEYID:0:0::::::\nfpr:::::::::" + pacmanMismatchFingerprint + ":\n"),
		},
	}}
	provider := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: pacmanDeclaredFingerprint,
	}, runner)
	provider.StateDir = t.TempDir()
	provider.GPGHomeDir = testGPGHome
	provider.Fetch = func(context.Context, string) ([]byte, error) { return []byte("mismatched-key"), nil }

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "fingerprint mismatch") {
		t.Fatalf("ApplyResult() = %+v, want complete fingerprint mismatch", result)
	}
	for _, call := range runner.Calls {
		if call.Name == "pacman-key" {
			t.Fatalf("mismatched key reached Pacman mutation: %+v", call)
		}
	}
	if len(runner.Inputs) != 1 || string(runner.Inputs[0].Input) != "mismatched-key" ||
		!slices.Equal(runner.Inputs[0].Args, []string{"--homedir", testGPGHome, "--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import"}) {
		t.Fatalf("protected fingerprint inspection = %+v", runner.Inputs)
	}
	if !errors.Is(provider.Revert(t.Context()), appErr.ErrNoOp) {
		t.Fatalf("Revert() after rejected key = %v, want no-op", provider.Revert(t.Context()))
	}
}

func TestApplicator_rejectsLexicallyLowerFingerprintMismatchBeforePacmanKeyMutation(t *testing.T) {
	const (
		declared = "F123456789ABCDEF0123456789ABCDEF01234567"
		actual   = "0123456789ABCDEF0123456789ABCDEF01234567"
	)
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"gpg [--homedir /tmp/remotr-pacman-key-test-home --batch --with-colons --import-options show-only --dry-run --import]": {
			Stdout: []byte("pub:-:255:1:KEYID:0:0::::::\nfpr:::::::::" + actual + ":\n"),
		},
	}}
	provider := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: declared,
	}, runner)
	provider.StateDir = t.TempDir()
	provider.GPGHomeDir = testGPGHome
	provider.Fetch = func(context.Context, string) ([]byte, error) { return []byte("lower-mismatched-key"), nil }

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "fingerprint mismatch") {
		t.Fatalf("ApplyResult() = %+v, want lower complete fingerprint mismatch", result)
	}
	for _, call := range runner.Calls {
		if call.Name == "pacman-key" {
			t.Fatalf("lower mismatched key reached Pacman mutation: %+v", call)
		}
	}
}

// OS-PRM-021: an unknown declared key is imported and locally trusted through
// Pacman's native boundary, then becomes compliant without repeating mutation.
func TestApplicator_unknownKeyConvergesAndSecondCheckIsCompliant(t *testing.T) {
	runner := &nativeKeyRunner{}
	stateDir := t.TempDir()
	provider := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: pacmanDeclaredFingerprint,
	}, runner)
	provider.StateDir = stateDir
	provider.GPGHomeDir = testGPGHome
	provider.Fetch = func(context.Context, string) ([]byte, error) { return []byte("matching-key"), nil }

	if check := provider.Check(t.Context()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v, want drift", check)
	}
	if result := provider.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want changed", result)
	}
	if check := provider.Check(t.Context()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(t.Context()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("second ApplyResult() = %+v, want no change", result)
	}
	marker, err := os.ReadFile(filepath.Join(stateDir, "vendor.fingerprint"))
	if err != nil || strings.TrimSpace(string(marker)) != pacmanDeclaredFingerprint {
		t.Fatalf("ownership marker = %q, %v", marker, err)
	}
	if !runner.added || !runner.trusted || runner.addCalls != 1 || runner.trustCalls != 1 {
		t.Fatalf("native key state = added:%t trusted:%t addCalls:%d trustCalls:%d", runner.added, runner.trusted, runner.addCalls, runner.trustCalls)
	}
	for _, call := range runner.calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("unexpected shell process: %+v", call)
		}
	}
}

func TestApplicator_absentDeletesPersistedOwnedFingerprintAndPreservesUnrelatedTrust(t *testing.T) {
	runner := &nativeKeyRunner{}
	stateDir := t.TempDir()
	present := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: pacmanDeclaredFingerprint,
	}, runner)
	present.StateDir = stateDir
	present.GPGHomeDir = testGPGHome
	present.Fetch = func(context.Context, string) ([]byte, error) { return []byte("matching-key"), nil }
	if result := present.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("present ApplyResult() = %+v", result)
	}
	unrelatedPath := filepath.Join(stateDir, "unrelated.fingerprint")
	if err := os.WriteFile(unrelatedPath, []byte(pacmanMismatchFingerprint+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	absent := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	}, runner)
	absent.StateDir = stateDir
	if check := absent.Check(t.Context()); check.Status != executor.Drifted {
		t.Fatalf("absent Check() = %+v, want drift", check)
	}
	if result := absent.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("absent ApplyResult() = %+v, want changed", result)
	}
	if runner.added || runner.trusted {
		t.Fatal("owned Pacman key remains after absence")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "vendor.fingerprint")); !os.IsNotExist(err) {
		t.Fatalf("owned marker remains: %v", err)
	}
	if got, err := os.ReadFile(unrelatedPath); err != nil || string(got) != pacmanMismatchFingerprint+"\n" {
		t.Fatalf("unrelated trust marker = %q, %v; want preserved", got, err)
	}
	if check := absent.Check(t.Context()); check.Status != executor.Compliant {
		t.Fatalf("second absent Check() = %+v, want compliant", check)
	}
}

func TestApplicator_adoptsMatchingNativeKeyWithoutReimport(t *testing.T) {
	runner := &nativeKeyRunner{added: true}
	provider := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: pacmanDeclaredFingerprint,
	}, runner)
	provider.StateDir = t.TempDir()
	provider.GPGHomeDir = testGPGHome
	provider.Fetch = func(context.Context, string) ([]byte, error) { return []byte("matching-key"), nil }

	if result := provider.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want adopted trust", result)
	}
	if runner.addCalls != 0 || runner.trustCalls != 1 || !runner.trusted {
		t.Fatalf("matching native key calls = add:%d trust:%d trusted:%t", runner.addCalls, runner.trustCalls, runner.trusted)
	}
}

func TestApplicator_nativeTrustFailureIsSanitizedAndRemovesNewImport(t *testing.T) {
	const canary = "pacman-key-native-secret-canary"
	runner := &failingTrustRunner{canary: canary}
	stateDir := t.TempDir()
	provider := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: pacmanDeclaredFingerprint,
	}, runner)
	provider.StateDir = stateDir
	provider.GPGHomeDir = testGPGHome
	provider.Fetch = func(context.Context, string) ([]byte, error) { return []byte("matching-key"), nil }

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "locally trust") {
		t.Fatalf("ApplyResult() = %+v, want local trust failure", result)
	}
	if strings.Contains(fmt.Sprintf("%+v", result), canary) {
		t.Fatalf("trust failure leaked native canary: %+v", result)
	}
	if runner.added || runner.addCalls != 1 || runner.deleteCalls != 1 {
		t.Fatalf("failed trust cleanup = added:%t addCalls:%d deleteCalls:%d", runner.added, runner.addCalls, runner.deleteCalls)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "vendor.fingerprint")); !os.IsNotExist(err) {
		t.Fatalf("failed trust wrote ownership marker: %v", err)
	}
	for _, call := range runner.calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("trust provider used a shell: %+v", call)
		}
	}
}

func TestApplicator_canceledContextDoesNotCrossNativeTrustBoundary(t *testing.T) {
	runner := &nativeKeyRunner{}
	provider := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: pacmanDeclaredFingerprint,
	}, runner)
	provider.StateDir = t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := provider.ApplyResult(ctx)
	if result.Status != executor.Failed || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("ApplyResult() = %+v, want cancellation", result)
	}
	if len(runner.calls) != 0 || len(runner.inputs) != 0 {
		t.Fatalf("canceled trust crossed process boundary: calls=%+v inputs=%+v", runner.calls, runner.inputs)
	}
}

func TestApplicator_defaultsToSanitizedProcessRunner(t *testing.T) {
	provider := pacmankeys.New(models.PacmanSigningKey{}, nil)
	if _, ok := provider.Runner.(executil.SanitizedOSRunner); !ok {
		t.Fatalf("default runner = %T, want SanitizedOSRunner", provider.Runner)
	}
}

func TestApplicator_nativeProbeFailureIsNotMisreportedAsTrustDrift(t *testing.T) {
	const canary = "pacman-key-probe-secret-canary"
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "vendor.fingerprint"), []byte(pacmanDeclaredFingerprint+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"pacman-key [--nocolor --finger " + pacmanDeclaredFingerprint + "]": {
			Stderr: []byte(canary), Err: errors.New("native executable failure " + canary),
		},
	}}
	provider := pacmankeys.New(models.PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: pacmanDeclaredFingerprint,
	}, runner)
	provider.StateDir = stateDir

	check := provider.Check(t.Context())
	if check.Status != executor.CheckFailed || check.Err == nil || !strings.Contains(check.Err.Error(), "native trust probe failed") {
		t.Fatalf("Check() = %+v, want native probe failure", check)
	}
	if strings.Contains(fmt.Sprintf("%+v", check), canary) {
		t.Fatalf("native trust probe leaked canary: %+v", check)
	}
}

type nativeKeyRunner struct {
	added, trusted       bool
	addCalls, trustCalls int
	calls                []executil.MockCall
	inputs               []executil.MockInput
}

type failingTrustRunner struct {
	canary                string
	added                 bool
	addCalls, deleteCalls int
	calls                 []executil.MockCall
}

func (r *failingTrustRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: slices.Clone(args)})
	switch {
	case name == "pacman-key" && slices.Equal(args, []string{"--nocolor", "--finger", pacmanDeclaredFingerprint}):
		return nil, []byte("unknown key"), errors.New("exit status 1")
	case name == "pacman-key" && len(args) == 2 && args[0] == "--add":
		material, err := os.ReadFile(args[1])
		if err != nil || string(material) != "matching-key" {
			return nil, nil, fmt.Errorf("staged material = %q, %v", material, err)
		}
		r.added, r.addCalls = true, r.addCalls+1
		return nil, nil, nil
	case name == "pacman-key" && slices.Equal(args, []string{"--lsign-key", pacmanDeclaredFingerprint}):
		return nil, []byte(r.canary), errors.New("native failure " + r.canary)
	case name == "pacman-key" && slices.Equal(args, []string{"--delete", pacmanDeclaredFingerprint}):
		r.added, r.deleteCalls = false, r.deleteCalls+1
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected process %s %v", name, args)
	}
}

func (r *failingTrustRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	if name != "gpg" || string(input) != "matching-key" || !slices.Equal(args, []string{"--homedir", testGPGHome, "--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import"}) {
		return nil, nil, fmt.Errorf("unexpected protected process %s %v", name, args)
	}
	return []byte("pub:-:255:1:KEYID:0:0::::::\nfpr:::::::::" + pacmanDeclaredFingerprint + ":\n"), nil, nil
}

func (r *nativeKeyRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: slices.Clone(args)})
	switch {
	case name == "pacman-key" && slices.Equal(args, []string{"--nocolor", "--finger", pacmanDeclaredFingerprint}):
		if !r.added {
			return nil, []byte("unknown key"), errors.New("exit status 1")
		}
		return []byte("Key fingerprint = " + pacmanDeclaredFingerprint + "\n"), nil, nil
	case name == "pacman-key" && len(args) == 2 && args[0] == "--add":
		material, err := os.ReadFile(args[1])
		if err != nil || string(material) != "matching-key" {
			return nil, nil, fmt.Errorf("import material = %q, %v", material, err)
		}
		r.added, r.addCalls = true, r.addCalls+1
		return nil, nil, nil
	case name == "pacman-key" && slices.Equal(args, []string{"--lsign-key", pacmanDeclaredFingerprint}):
		if !r.added {
			return nil, nil, errors.New("key not imported")
		}
		r.trusted, r.trustCalls = true, r.trustCalls+1
		return nil, nil, nil
	case name == "pacman-key" && slices.Equal(args, []string{"--delete", pacmanDeclaredFingerprint}):
		r.added, r.trusted = false, false
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected process %s %v", name, args)
	}
}

func (r *nativeKeyRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	r.inputs = append(r.inputs, executil.MockInput{Name: name, Args: slices.Clone(args), Input: slices.Clone(input)})
	if name != "gpg" || !slices.Equal(args, []string{"--homedir", testGPGHome, "--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import"}) {
		return nil, nil, fmt.Errorf("unexpected protected process %s %v", name, args)
	}
	return []byte("pub:-:255:1:KEYID:0:0::::::\nfpr:::::::::" + pacmanDeclaredFingerprint + ":\n"), nil, nil
}
