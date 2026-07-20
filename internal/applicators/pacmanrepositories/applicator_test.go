package pacmanrepositories_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/pacmanrepositories"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const canonicalVendorRepository = `# Managed by Remotr. Do not edit.
[options]
Architecture = x86_64

[vendor]
SigLevel = Required DatabaseRequired
Server = https://mirror-1.example.test/$repo/os/$arch
Server = https://mirror-2.example.test/$repo/os/$arch
`

func vendorRepository(lifecycle models.Lifecycle) models.PacmanRepository {
	return models.PacmanRepository{
		ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle},
		Name:         "vendor", Servers: []string{
			"https://mirror-1.example.test/$repo/os/$arch",
			"https://mirror-2.example.test/$repo/os/$arch",
		},
		Architecture: "x86_64", SignatureLevel: models.PacmanSignatureRequired,
		SigningKeys: []string{"vendor-key"},
	}
}

// OS-PRM-020: the provider stages and natively validates the complete
// canonical fragment before replacing an existing owned fragment.
func TestApplicator_presentRepositoryConvergesThroughStagedNativeValidation(t *testing.T) {
	directory := t.TempDir()
	livePath := filepath.Join(directory, "vendor.conf")
	if err := os.WriteFile(livePath, []byte("[vendor\nmalformed = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &validationRunner{t: t, livePath: livePath, liveBefore: "[vendor\nmalformed = true\n", wantStaged: canonicalVendorRepository}
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), runner)
	provider.FragmentsDir = directory
	provider.ConfigPath = writeTestPacmanConfig(t, "")
	runner.configPath = provider.ConfigPath

	if check := provider.Check(t.Context()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v, want safe drift for malformed owned content", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want changed", result)
	}
	if got, err := os.ReadFile(livePath); err != nil || string(got) != canonicalVendorRepository {
		t.Fatalf("activated fragment = %q, %v; want canonical content", got, err)
	}
	if info, err := os.Lstat(livePath); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o644 {
		t.Fatalf("activated fragment mode = %v, %v; want regular 0644", info, err)
	}
	runner.liveBefore = canonicalVendorRepository
	if check := provider.Check(t.Context()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v, want compliant", check)
	}
	if runner.stageValidations != 1 || runner.liveValidations != 1 {
		t.Fatalf("native validations = staged:%d live:%d, want one each", runner.stageValidations, runner.liveValidations)
	}
	for _, call := range runner.calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("repository validation used a shell: %+v", call)
		}
	}
}

func TestApplicator_compliantRepositoryRequiresNativeParse(t *testing.T) {
	directory := t.TempDir()
	livePath := filepath.Join(directory, "vendor.conf")
	if err := os.WriteFile(livePath, []byte(canonicalVendorRepository), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &validationRunner{t: t, livePath: livePath, liveBefore: canonicalVendorRepository, wantStaged: canonicalVendorRepository}
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), runner)
	provider.FragmentsDir = directory
	provider.ConfigPath = writeTestPacmanConfig(t, providerBoundary(directory))
	runner.configPath = provider.ConfigPath

	if check := provider.Check(t.Context()); check.Status != executor.Compliant {
		t.Fatalf("Check() = %+v, want compliant native configuration", check)
	}
	if runner.liveValidations != 1 {
		t.Fatalf("live native validations = %d, want 1", runner.liveValidations)
	}
}

func TestApplicator_disabledRepositoryConvergesToCanonicalComments(t *testing.T) {
	directory := t.TempDir()
	livePath := filepath.Join(directory, "vendor.conf")
	want := `# Disabled by Remotr.
# [options]
# Architecture = x86_64
#
# [vendor]
# SigLevel = Required DatabaseRequired
# Server = https://mirror-1.example.test/$repo/os/$arch
# Server = https://mirror-2.example.test/$repo/os/$arch
`
	runner := &validationRunner{t: t, livePath: livePath, wantStaged: want, disabled: true}
	provider := pacmanrepositories.New(vendorRepository(models.LifecycleDisabled), runner)
	provider.FragmentsDir = directory
	provider.ConfigPath = writeTestPacmanConfig(t, "")
	runner.configPath = provider.ConfigPath

	if check := provider.Check(t.Context()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v, want drift", check)
	}
	if result := provider.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want changed", result)
	}
	if got, err := os.ReadFile(livePath); err != nil || string(got) != want {
		t.Fatalf("disabled fragment = %q, %v; want canonical comments", got, err)
	}
	runner.liveBefore = want
	if check := provider.Check(t.Context()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v, want compliant", check)
	}
}

func TestApplicator_absentRepositoryRemovesOnlyItsOwnedFragment(t *testing.T) {
	directory := t.TempDir()
	ownedPath := filepath.Join(directory, "vendor.conf")
	if err := os.WriteFile(ownedPath, []byte(canonicalVendorRepository), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := pacmanrepositories.New(models.PacmanRepository{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "vendor",
	}, &executil.MockRunner{})
	provider.FragmentsDir = directory
	const unrelated = "[core]\nInclude = /etc/pacman.d/mirrorlist\n"
	provider.ConfigPath = writeTestPacmanConfig(t, unrelated)

	if check := provider.Check(t.Context()); check.Status != executor.Drifted {
		t.Fatalf("absent Check() = %+v, want drift", check)
	}
	if result := provider.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want changed", result)
	}
	if _, err := os.Stat(ownedPath); !os.IsNotExist(err) {
		t.Fatalf("owned fragment remains: %v", err)
	}
	if got, err := os.ReadFile(provider.ConfigPath); err != nil || string(got) != unrelated {
		t.Fatalf("unrelated Pacman content = %q, %v; want preserved", got, err)
	}
	if check := provider.Check(t.Context()); check.Status != executor.Compliant {
		t.Fatalf("second absent Check() = %+v, want compliant", check)
	}
}

func TestApplicator_nativeValidationFailurePreservesPreviousFragment(t *testing.T) {
	const canary = "pacman-native-validation-secret-canary"
	directory := t.TempDir()
	livePath := filepath.Join(directory, "vendor.conf")
	const previous = "[vendor\nmalformed = true\n"
	if err := os.WriteFile(livePath, []byte(previous), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &validationRunner{
		t: t, livePath: livePath, liveBefore: previous, wantStaged: canonicalVendorRepository,
		validationErr: errors.New("invalid repository fragment " + canary),
	}
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), runner)
	provider.FragmentsDir = directory
	provider.ConfigPath = writeTestPacmanConfig(t, "")
	runner.configPath = provider.ConfigPath

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "native validation failed") {
		t.Fatalf("ApplyResult() = %+v, want native validation failure", result)
	}
	if strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("native validation failure leaked provider output canary: %v", result.Err)
	}
	if got, err := os.ReadFile(livePath); err != nil || string(got) != previous {
		t.Fatalf("live fragment after failed validation = %q, %v; want previous content", got, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "vendor.conf" {
		t.Fatalf("staged files remain after validation failure: %+v", entries)
	}
}

func TestApplicator_presentRepositoryAddsOneReversibleIncludeBoundary(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "remotr-repositories")
	configPath := filepath.Join(root, "pacman.conf")
	const base = "# distribution-owned bytes\n[options]\nArchitecture = auto"
	if err := os.WriteFile(configPath, []byte(base), 0o640); err != nil {
		t.Fatal(err)
	}
	boundary := "\n# BEGIN Remotr managed Pacman repositories\nInclude = " + directory + "/*.conf\n# END Remotr managed Pacman repositories\n"
	runner := &includeValidationRunner{
		t: t, liveConfigPath: configPath, liveConfigBefore: base,
		wantStagedConfig: base + boundary, repositoryName: "vendor",
	}
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), runner)
	provider.FragmentsDir, provider.ConfigPath = directory, configPath

	if check := provider.Check(t.Context()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v, want drift", check)
	}
	if result := provider.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want changed", result)
	}
	got, err := os.ReadFile(configPath)
	if err != nil || string(got) != base+boundary {
		t.Fatalf("pacman.conf = %q, %v; want preserved base plus boundary", got, err)
	}
	if strings.Count(string(got), "BEGIN Remotr managed Pacman repositories") != 1 {
		t.Fatalf("managed include boundary count in %q is not one", got)
	}
	if info, err := os.Stat(configPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("pacman.conf mode = %v, %v; want preserved 0640", info, err)
	}
	runner.liveConfigBefore = base + boundary
	runner.wantStagedConfig = ""
	if result := provider.ApplyResult(t.Context()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("second ApplyResult() = %+v, want no change", result)
	}
	if runner.configValidations != 1 {
		t.Fatalf("staged pacman.conf validations = %d, want exactly one", runner.configValidations)
	}
}

func TestApplicator_absentRepositoryRemovesBoundaryOnlyAfterLastActiveFragment(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "remotr-repositories")
	configPath := filepath.Join(root, "pacman.conf")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	const base = "# distribution-owned bytes\n[options]\nArchitecture = auto"
	boundary := "\n# BEGIN Remotr managed Pacman repositories\nInclude = " + directory + "/*.conf\n# END Remotr managed Pacman repositories\n"
	if err := os.WriteFile(configPath, []byte(base+boundary), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vendor", "other"} {
		content := strings.ReplaceAll(canonicalVendorRepository, "[vendor]", "["+name+"]")
		if err := os.WriteFile(filepath.Join(directory, name+".conf"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runner := &includeValidationRunner{t: t, liveConfigPath: configPath}
	removeVendor := pacmanrepositories.New(models.PacmanRepository{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "vendor",
	}, runner)
	removeVendor.FragmentsDir, removeVendor.ConfigPath = directory, configPath
	if result := removeVendor.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("remove first repository = %+v", result)
	}
	if got, err := os.ReadFile(configPath); err != nil || string(got) != base+boundary {
		t.Fatalf("boundary after first removal = %q, %v; want retained", got, err)
	}

	runner.liveConfigBefore, runner.wantStagedConfig = base+boundary, base
	removeOther := pacmanrepositories.New(models.PacmanRepository{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "other",
	}, runner)
	removeOther.FragmentsDir, removeOther.ConfigPath = directory, configPath
	if result := removeOther.ApplyResult(t.Context()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("remove last repository = %+v", result)
	}
	if got, err := os.ReadFile(configPath); err != nil || string(got) != base {
		t.Fatalf("pacman.conf after last removal = %q, %v; want exact original bytes", got, err)
	}
	if runner.configValidations != 1 {
		t.Fatalf("pacman.conf validations = %d, want one removal validation", runner.configValidations)
	}
}

func TestApplicator_malformedOwnedBoundaryFailsClosedWithoutEditingPacmanConf(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "remotr-repositories")
	configPath := filepath.Join(root, "pacman.conf")
	const malformed = "[options]\nArchitecture = auto\n# BEGIN Remotr managed Pacman repositories\nInclude = /wrong/path/*.conf\n"
	if err := os.WriteFile(configPath, []byte(malformed), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), &includeValidationRunner{t: t, liveConfigPath: configPath})
	provider.FragmentsDir, provider.ConfigPath = directory, configPath

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "managed include boundary is malformed") {
		t.Fatalf("ApplyResult() = %+v, want fail-closed malformed boundary", result)
	}
	if got, err := os.ReadFile(configPath); err != nil || string(got) != malformed {
		t.Fatalf("malformed pacman.conf changed = %q, %v", got, err)
	}
}

func TestApplicator_configValidationFailureRestoresPreviousFragment(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "remotr-repositories")
	configPath := filepath.Join(root, "pacman.conf")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	const previousFragment = "[vendor\nprevious malformed bytes\n"
	const previousConfig = "# distribution-owned bytes\n[options]\nArchitecture = auto"
	fragmentPath := filepath.Join(directory, "vendor.conf")
	if err := os.WriteFile(fragmentPath, []byte(previousFragment), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(previousConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &includeValidationRunner{
		t: t, liveConfigPath: configPath, liveConfigBefore: previousConfig,
		wantStagedConfig: previousConfig + providerBoundary(directory), repositoryName: "vendor",
		configValidationErr: errors.New("native parser rejected staged configuration"),
	}
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), runner)
	provider.FragmentsDir, provider.ConfigPath = directory, configPath

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "native validation failed") {
		t.Fatalf("ApplyResult() = %+v, want staged config validation failure", result)
	}
	if got, err := os.ReadFile(fragmentPath); err != nil || string(got) != previousFragment {
		t.Fatalf("fragment after partial failure = %q, %v; want exact previous bytes", got, err)
	}
	if info, err := os.Stat(fragmentPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("restored fragment mode = %v, %v; want previous 0640", info, err)
	}
	if got, err := os.ReadFile(configPath); err != nil || string(got) != previousConfig {
		t.Fatalf("pacman.conf after partial failure = %q, %v; want exact previous bytes", got, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "vendor.conf" {
		t.Fatalf("partial failure left staging residue: %+v", entries)
	}
}

func TestApplicator_cancellationAfterFragmentValidationRestoresSafeState(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "remotr-repositories")
	configPath := filepath.Join(root, "pacman.conf")
	const previousConfig = "# distribution-owned bytes\n[options]\nArchitecture = auto"
	if err := os.WriteFile(configPath, []byte(previousConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runner := &includeValidationRunner{
		t: t, liveConfigPath: configPath, liveConfigBefore: previousConfig,
		wantStagedConfig: previousConfig + providerBoundary(directory), repositoryName: "vendor",
		cancelAfterFragmentValidation: cancel,
	}
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), runner)
	provider.FragmentsDir, provider.ConfigPath = directory, configPath

	result := provider.ApplyResult(ctx)
	if result.Status != executor.Failed || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("ApplyResult() = %+v, want cancellation", result)
	}
	if _, err := os.Stat(filepath.Join(directory, "vendor.conf")); !os.IsNotExist(err) {
		t.Fatalf("canceled transaction left a repository fragment: %v", err)
	}
	if got, err := os.ReadFile(configPath); err != nil || string(got) != previousConfig {
		t.Fatalf("pacman.conf after cancellation = %q, %v; want previous bytes", got, err)
	}
	if runner.fragmentValidations != 1 || runner.configValidations != 0 {
		t.Fatalf("canceled validation calls = fragment:%d config:%d", runner.fragmentValidations, runner.configValidations)
	}
}

func TestApplicator_credentialReferenceFailsClosedWithoutLeakOrMutation(t *testing.T) {
	const canary = "pacman-repository-secret-canary"
	root := t.TempDir()
	configPath := filepath.Join(root, "pacman.conf")
	const previousConfig = "[options]\nArchitecture = auto\n"
	if err := os.WriteFile(configPath, []byte(previousConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	repository := vendorRepository(models.LifecyclePresent)
	repository.CredentialRef = "file:/run/remotr/" + canary
	provider := pacmanrepositories.New(repository, runner)
	provider.FragmentsDir, provider.ConfigPath = filepath.Join(root, "remotr-repositories"), configPath

	check := provider.Check(t.Context())
	if check.Status != executor.CheckFailed || check.Err == nil || !strings.Contains(check.Err.Error(), "credentials are not supported") || strings.Contains(fmt.Sprintf("%+v", check), canary) {
		t.Fatalf("Check() = %+v, want redacted fail-closed unsupported credentials", check)
	}
	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "credentials are not supported") {
		t.Fatalf("ApplyResult() = %+v, want fail-closed unsupported credentials", result)
	}
	if strings.Contains(fmt.Sprintf("%+v", result), canary) {
		t.Fatalf("credential reference canary leaked into result: %+v", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("unsupported credential crossed native process boundary: %+v", runner.Calls)
	}
	if _, err := os.Stat(provider.FragmentsDir); !os.IsNotExist(err) {
		t.Fatalf("unsupported credential created repository state: %v", err)
	}
	if got, err := os.ReadFile(configPath); err != nil || string(got) != previousConfig {
		t.Fatalf("unsupported credential changed pacman.conf: %q, %v", got, err)
	}
}

func TestApplicator_defaultsToSanitizedShellFreeProcessRunner(t *testing.T) {
	provider := pacmanrepositories.New(vendorRepository(models.LifecyclePresent), nil)
	if _, ok := provider.Runner.(executil.SanitizedOSRunner); !ok {
		t.Fatalf("default runner = %T, want SanitizedOSRunner", provider.Runner)
	}
}

type validationRunner struct {
	t                *testing.T
	livePath         string
	configPath       string
	liveBefore       string
	wantStaged       string
	disabled         bool
	validationErr    error
	stageValidations int
	liveValidations  int
	calls            []executil.MockCall
}

func (r *validationRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.t.Helper()
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: slices.Clone(args)})
	if name != "pacman-conf" || len(args) < 2 || args[0] != "--config" {
		r.t.Fatalf("native validation = %s %v, want pacman-conf --config <path>", name, args)
	}
	if r.disabled {
		if len(args) != 2 {
			r.t.Fatalf("disabled validation argv = %v, want only --config <path>", args)
		}
	} else if !slices.Equal(args[2:], []string{"--repo", "vendor"}) {
		r.t.Fatalf("present validation argv = %v, want --repo vendor", args)
	}
	path := args[1]
	if path == r.configPath {
		r.liveValidations++
		return []byte("valid\n"), nil, nil
	}
	if r.configPath != "" && filepath.Dir(path) == filepath.Dir(r.configPath) {
		return []byte("valid\n"), nil, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		r.t.Fatal(err)
	}
	r.stageValidations++
	if string(content) != r.wantStaged {
		r.t.Fatalf("staged validation content = %q, want %q", content, r.wantStaged)
	}
	if r.liveBefore == "" {
		if _, err := os.Stat(r.livePath); !os.IsNotExist(err) {
			r.t.Fatalf("live fragment existed before staged validation: %v", err)
		}
	} else if live, err := os.ReadFile(r.livePath); err != nil || string(live) != r.liveBefore {
		r.t.Fatalf("live fragment during staged validation = %q, %v; want previous content", live, err)
	}
	return []byte("valid\n"), nil, r.validationErr
}

type includeValidationRunner struct {
	t                             *testing.T
	liveConfigPath                string
	liveConfigBefore              string
	wantStagedConfig              string
	repositoryName                string
	configValidationErr           error
	cancelAfterFragmentValidation context.CancelFunc
	fragmentValidations           int
	configValidations             int
	liveValidations               int
}

func (r *includeValidationRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.t.Helper()
	if name != "pacman-conf" || len(args) < 2 || args[0] != "--config" {
		r.t.Fatalf("native validation = %s %v, want pacman-conf --config <path>", name, args)
	}
	path := args[1]
	if path == r.liveConfigPath {
		r.liveValidations++
		wantTail := []string(nil)
		if r.repositoryName != "" {
			wantTail = []string{"--repo", r.repositoryName}
		}
		if !slices.Equal(args[2:], wantTail) {
			r.t.Fatalf("effective pacman.conf validation argv = %v, want tail %v", args, wantTail)
		}
		return []byte("valid\n"), nil, nil
	}
	if filepath.Dir(path) == filepath.Dir(r.liveConfigPath) {
		r.configValidations++
		if r.wantStagedConfig == "" {
			r.t.Fatalf("unexpected staged pacman.conf validation: %v", args)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != r.wantStagedConfig {
			r.t.Fatalf("staged pacman.conf = %q, %v; want %q", got, err, r.wantStagedConfig)
		}
		if got, err := os.ReadFile(r.liveConfigPath); err != nil || string(got) != r.liveConfigBefore {
			r.t.Fatalf("live pacman.conf during validation = %q, %v; want previous bytes", got, err)
		}
		wantTail := []string(nil)
		if r.repositoryName != "" {
			wantTail = []string{"--repo", r.repositoryName}
		}
		if !slices.Equal(args[2:], wantTail) {
			r.t.Fatalf("pacman.conf validation argv = %v, want tail %v", args, wantTail)
		}
		return []byte("valid\n"), nil, r.configValidationErr
	}
	r.fragmentValidations++
	if r.cancelAfterFragmentValidation != nil {
		r.cancelAfterFragmentValidation()
		r.cancelAfterFragmentValidation = nil
	}
	return []byte("valid\n"), nil, nil
}

func writeTestPacmanConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pacman.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func providerBoundary(directory string) string {
	return "\n# BEGIN Remotr managed Pacman repositories\nInclude = " + directory + "/*.conf\n# END Remotr managed Pacman repositories\n"
}
