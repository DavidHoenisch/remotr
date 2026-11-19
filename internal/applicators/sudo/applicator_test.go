package sudo_test

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/sudo"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func testSudoResource() models.SudoResource {
	return models.SudoResource{
		Name:               "developer-admin",
		Subjects:           []string{"developer"},
		RunAs:              []string{"ALL"},
		Commands:           []string{"/usr/bin/id"},
		Tags:               []string{"NOPASSWD"},
		RecoveryPrincipals: []string{"recovery"},
		ResourceMeta:       models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipFragment},
	}
}

// OS-LIA-010: failed effective sudoers validation leaves the active named
// fragment untouched; successful validation activates only that fragment.
func TestSudoApplicatorValidatesStagedEffectiveConfigurationBeforeActivation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "developer-admin")
	if err := os.WriteFile(path, []byte("old sudo policy\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	sudoers := filepath.Join(dir, "sudoers")
	if err := os.WriteFile(sudoers, []byte("@includedir "+dir+"\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	provider := sudo.New(testSudoResource())
	configureTestSudoOwnership(t, provider)
	provider.SudoersDir, provider.SudoersPath = dir, sudoers
	provider.LookupRecovery = func(string) error { return nil }
	provider.VerifyRecoveryAccess = func(context.Context, string, string, string) error { return nil }
	rollbackRoot := filepath.Join(dir, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.ConfigureRollback(store, "base/developer-admin", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	provider.ValidateEffective = func(_ context.Context, stagedSudoers, stagedDir string) error {
		if stagedSudoers == sudoers || stagedDir == dir {
			t.Fatal("effective validation must use an isolated staged tree")
		}
		content, err := os.ReadFile(filepath.Join(stagedDir, "developer-admin"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "developer ALL=(ALL) NOPASSWD: /usr/bin/id") {
			t.Fatalf("staged fragment = %q", content)
		}
		return errors.New("visudo rejected staged policy")
	}

	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("invalid staged sudoers policy must not activate")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "old sudo policy\n" {
		t.Fatalf("active fragment changed after failed validation: %q, %v", content, err)
	}

	provider.ValidateEffective = func(context.Context, string, string) error { return nil }
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	content, err = os.ReadFile(path)
	if err != nil || string(content) != "developer ALL=(ALL) NOPASSWD: /usr/bin/id\n" {
		t.Fatalf("active fragment = %q, %v", content, err)
	}
	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := sudo.New(testSudoResource())
	restarted.SudoersDir, restarted.SudoersPath = dir, sudoers
	if err := restarted.ConfigureRollback(restartedStore, "base/developer-admin", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil || string(content) != "old sudo policy\n" {
		t.Fatalf("protected rollback = %q, %v", content, err)
	}
}

func configureTestSudoOwnership(t *testing.T, provider *sudo.Applicator) {
	t.Helper()
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	group, err := user.LookupGroupId(account.Gid)
	if err != nil {
		t.Fatal(err)
	}
	provider.Owner, provider.Group = account.Username, group.Name
}

// OS-LIA-011: an access-risk sudo resource cannot begin enforcement until a
// declared recovery principal has passed its resource-specific preflight.
func TestSudoApplicatorPreflightRequiresRecoveryPrincipal(t *testing.T) {
	provider := sudo.New(testSudoResource())
	provider.LookupRecovery = func(string) error { return errors.New("recovery principal is unavailable") }
	if err := provider.Preflight(context.Background()); err == nil {
		t.Fatal("missing recovery principal must block sudo enforcement")
	}
}

// OS-AEC-098: sudoers fragments have an implicit root:root ownership
// contract; correct text and mode under an unprivileged owner are not safe.
func TestSudoProviderRejectsNonRootOwnedCompliantFragment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "developer-admin")
	if err := os.WriteFile(path, []byte("developer ALL=(ALL) NOPASSWD: /usr/bin/id\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, 65534, 65534); err != nil {
			t.Fatal(err)
		}
	}
	applicator := sudo.New(testSudoResource())
	applicator.SudoersDir = dir
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	if result := provider.Check(context.Background()); result.Status != contract.Drifted {
		t.Fatalf("unprivileged-owned sudo fragment Check = %+v, want drifted", result)
	}
}

// OS-AEC-098: effective-policy validation is an external diagnostic boundary;
// provider errors must not reproduce staged policy or other canary material.
func TestSudoProviderRedactsEffectiveValidationFailure(t *testing.T) {
	const canary = "REMOTR-SUDO-SECRET-CANARY"
	dir := t.TempDir()
	sudoers := filepath.Join(dir, "sudoers")
	if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	applicator := sudo.New(testSudoResource())
	configureTestSudoOwnership(t, applicator)
	applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
	applicator.LookupRecovery = func(string) error { return nil }
	applicator.ValidateEffective = func(context.Context, string, string) error {
		return errors.New("validator copied staged policy: " + canary)
	}
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("Apply = %+v, want failed validation", result)
	}
	if strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("validation failure leaked policy canary: %v", result.Err)
	}
}

// OS-LIA-014 & OS-LIA-015: priority-prefixed fragments render typed Defaults,
// multi-rule lists, user:group runAs targets, and argument-scoped commandSpecs.
func TestSudoApplicatorRendersHardeningDefaultsMultiRulesAndPriorityPath(t *testing.T) {
	dir := t.TempDir()
	sudoers := filepath.Join(dir, "sudoers")
	if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\nrecovery ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	prio := 20
	resource := models.SudoResource{
		ResourceMeta:       models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipFragment},
		Name:               "platform-ops",
		Priority:           &prio,
		RecoveryPrincipals: []string{"recovery"},
		Defaults: []models.SudoDefault{
			{Option: "use_pty"},
			{Option: "requiretty", Negate: true},
			{Option: "timestamp_timeout", Value: "5"},
			{Option: "secure_path", Value: "/usr/sbin:/usr/bin"},
			{Scope: "user", Target: "%ops", Option: "env_keep", Values: []string{"SSH_AUTH_SOCK", "LANG"}},
			{Scope: "command", Target: "/usr/bin/systemctl", Option: "log_output"},
		},
		Rules: []models.SudoRule{
			{
				Subjects: []string{"%ops"},
				RunAs:    []string{"root:root"},
				Tags:     []string{"NOPASSWD"},
				CommandSpecs: []models.SudoCommandSpec{
					{Path: "/usr/bin/id", NoArgs: true},
					{Path: "/usr/bin/systemctl", Args: []string{"status", "nginx"}},
					{Path: "/usr/bin/su", Negate: true},
				},
			},
			{
				Subjects: []string{"developer"},
				RunAs:    []string{"ALL:ALL", ":adm"},
				Tags:     []string{"PASSWD", "NOEXEC"},
				Commands: []string{"/usr/bin/journalctl"},
			},
		},
	}

	applicator := sudo.New(resource)
	configureTestSudoOwnership(t, applicator)
	applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
	applicator.LookupRecovery = func(string) error { return nil }
	applicator.ValidateEffective = func(context.Context, string, string) error { return nil }
	applicator.VerifyRecoveryAccess = func(context.Context, string, string, string) error { return nil }

	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	if check := provider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", check)
	}
	if res := provider.Apply(context.Background()); res.Status != contract.Changed || res.Err != nil {
		t.Fatalf("Apply = %+v, want changed", res)
	}
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}

	gotBytes, err := os.ReadFile(filepath.Join(dir, "20-platform-ops"))
	if err != nil {
		t.Fatalf("expected priority fragment 20-platform-ops: %v", err)
	}
	want := strings.Join([]string{
		`Defaults use_pty`,
		`Defaults !requiretty`,
		`Defaults timestamp_timeout=5`,
		`Defaults secure_path="/usr/sbin:/usr/bin"`,
		`Defaults:%ops env_keep += "SSH_AUTH_SOCK LANG"`,
		`Defaults!/usr/bin/systemctl log_output`,
		`%ops ALL=(root:root) NOPASSWD: /usr/bin/id "", /usr/bin/systemctl status nginx, !/usr/bin/su`,
		`developer ALL=(ALL:ALL,:adm) PASSWD: NOEXEC: /usr/bin/journalctl`,
	}, "\n") + "\n"
	if string(gotBytes) != want {
		t.Fatalf("rendered fragment =\n%s\nwant:\n%s", string(gotBytes), want)
	}
}

// OS-LIA-010 & OS-LIA-017: non-enforcing Preflight checks sudoers include
// prerequisites, rewrites all includedir directives in the staged copy, runs
// visudo -cf at the process boundary, and redacts validator failure output.
func TestSudoApplicatorPreflightValidatesStagedVisudoAndPrerequisites(t *testing.T) {
	t.Run("missing includedir fails preflight", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "sudoers.d")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		sudoers := filepath.Join(root, "sudoers")
		if err := os.WriteFile(sudoers, []byte("root ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
			t.Fatal(err)
		}
		applicator := sudo.New(testSudoResource())
		applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
		applicator.LookupRecovery = func(string) error { return nil }
		if err := applicator.Preflight(context.Background()); err == nil {
			t.Fatal("Preflight() succeeded despite missing includedir directive")
		}
	})

	t.Run("visudo failure fails preflight with redacted error and exact argv", func(t *testing.T) {
		const canary = "REMOTR-PREFLIGHT-VISUDO-CANARY"
		root := t.TempDir()
		dir := filepath.Join(root, "sudoers.d")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		sudoers := filepath.Join(root, "sudoers")
		if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\n@includedir "+dir+"\nrecovery ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
			t.Fatal(err)
		}
		var sawCalls []recordedCall
		runner := &funcRunner{fn: func(name string, args ...string) ([]byte, []byte, error) {
			sawCalls = append(sawCalls, recordedCall{name: name, args: append([]string(nil), args...)})
			if name == "visudo" {
				stagedMain, err := os.ReadFile(args[1])
				if err != nil {
					t.Fatalf("read staged sudoers: %v", err)
				}
				if strings.Contains(string(stagedMain), dir) {
					t.Fatalf("staged sudoers still references live dir %q:\n%s", dir, string(stagedMain))
				}
				return nil, []byte(canary), errors.New("syntax error: " + canary)
			}
			return nil, nil, nil
		}}
		applicator := sudo.New(testSudoResource(), runner)
		applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
		applicator.LookupRecovery = func(string) error { return nil }
		err := applicator.Preflight(context.Background())
		if err == nil {
			t.Fatal("Preflight() succeeded despite visudo rejection")
		}
		if strings.Contains(err.Error(), canary) {
			t.Fatalf("Preflight() leaked validator canary: %v", err)
		}
		if len(sawCalls) == 0 || sawCalls[0].name != "visudo" || len(sawCalls[0].args) != 2 || sawCalls[0].args[0] != "-cf" {
			t.Fatalf("visudo argv = %+v, want [visudo -cf <staged-sudoers>]", sawCalls)
		}
	})
}

type recordedCall struct {
	name string
	args []string
}

type funcRunner struct {
	fn func(name string, args ...string) ([]byte, []byte, error)
}

func (f *funcRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return f.fn(name, args...)
}

// OS-LIA-011 & OS-LIA-016: Preflight rejects unprivileged recovery accounts
// and blocks changes that would remove a recovery principal's last staged
// sudo grant, while allowing changes when another staged rule preserves access.
func TestSudoApplicatorPreflightVerifiesEffectiveRecoveryAccess(t *testing.T) {
	t.Run("unprivileged local account fails live sudo grant verification with redacted error", func(t *testing.T) {
		const canary = "REMOTR-SUDO-LIST-CANARY"
		root := t.TempDir()
		dir := filepath.Join(root, "sudoers.d")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		sudoers := filepath.Join(root, "sudoers")
		if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\nrecovery ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
			t.Fatal(err)
		}
		var sawSudoCall recordedCall
		runner := &funcRunner{fn: func(name string, args ...string) ([]byte, []byte, error) {
			if name == "sudo" {
				sawSudoCall = recordedCall{name: name, args: append([]string(nil), args...)}
				return []byte("User recovery is not allowed to run sudo: " + canary), nil, errors.New("exit status 1: " + canary)
			}
			return nil, nil, nil
		}}
		applicator := sudo.New(testSudoResource(), runner)
		applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
		applicator.LookupRecovery = func(string) error { return nil }
		err := applicator.Preflight(context.Background())
		if err == nil {
			t.Fatal("Preflight() succeeded for recovery principal without live sudo access")
		}
		if strings.Contains(err.Error(), canary) {
			t.Fatalf("Preflight() leaked sudo -l canary: %v", err)
		}
		if sawSudoCall.name != "sudo" || strings.Join(sawSudoCall.args, " ") != "-l -U recovery" {
			t.Fatalf("sudo argv = %+v, want sudo -l -U recovery", sawSudoCall)
		}
	})

	t.Run("revoking recovery principal last sudo fragment blocks preflight and apply", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "sudoers.d")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		recoveryFrag := filepath.Join(dir, "recovery-admin")
		const original = "recovery ALL=(ALL:ALL) NOPASSWD: ALL\n"
		if err := os.WriteFile(recoveryFrag, []byte(original), 0o440); err != nil {
			t.Fatal(err)
		}
		sudoers := filepath.Join(root, "sudoers")
		if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\nroot ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
			t.Fatal(err)
		}
		runner := &funcRunner{fn: func(name string, args ...string) ([]byte, []byte, error) {
			if name == "sudo" {
				return []byte("User recovery may run the following commands on host:\n    (ALL : ALL) NOPASSWD: ALL\n"), nil, nil
			}
			return nil, nil, nil
		}}
		res := models.SudoResource{
			ResourceMeta:       models.ResourceMeta{Lifecycle: models.LifecycleAbsent, Ownership: models.OwnershipFragment},
			Name:               "recovery-admin",
			RecoveryPrincipals: []string{"recovery"},
		}
		applicator := sudo.New(res, runner)
		configureTestSudoOwnership(t, applicator)
		applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
		applicator.LookupRecovery = func(string) error { return nil }
		applicator.LookupRecoveryGroups = func(string) ([]string, error) { return nil, nil }

		if err := applicator.Preflight(context.Background()); err == nil {
			t.Fatal("Preflight() allowed removing recovery principal's last sudo grant")
		}
		if err := applicator.Apply(context.Background()); err == nil {
			t.Fatal("Apply() allowed removing recovery principal's last sudo grant")
		}
		got, err := os.ReadFile(recoveryFrag)
		if err != nil || string(got) != original {
			t.Fatalf("active recovery fragment modified: %q, err=%v", string(got), err)
		}
	})

	t.Run("recovery principal preserved via supplementary group or User_Alias passes preflight", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "sudoers.d")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		sudoers := filepath.Join(root, "sudoers")
		if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\nUser_Alias BREAKGLASS = %wheel\nBREAKGLASS ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
			t.Fatal(err)
		}
		runner := &funcRunner{fn: func(name string, args ...string) ([]byte, []byte, error) {
			if name == "sudo" {
				return []byte("User recovery may run the following commands on host:\n    (ALL : ALL) ALL\n"), nil, nil
			}
			return nil, nil, nil
		}}
		applicator := sudo.New(testSudoResource(), runner)
		applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
		applicator.LookupRecovery = func(string) error { return nil }
		applicator.LookupRecoveryGroups = func(string) ([]string, error) { return []string{"wheel"}, nil }

		if err := applicator.Preflight(context.Background()); err != nil {
			t.Fatalf("Preflight() error = %v, want nil when recovery group alias remains", err)
		}
	})
}

// OS-LIA-018: when a compliant fragment is followed lexically by another
// active sudoers.d fragment matching a managed subject, Check reports a
// compliant resource with redacted, bounded sudo_fragment_shadowed subresults.
func TestSudoApplicatorCheckDetectsLexicallyLaterShadowingWithoutLeakingContent(t *testing.T) {
	const secretCmd = "/usr/local/bin/REMOTR-SHADOW-SECRET-CANARY"
	dir := t.TempDir()
	prio := 20
	res := testSudoResource()
	res.Priority = &prio

	managedPath := filepath.Join(dir, "20-developer-admin")
	if err := os.WriteFile(managedPath, []byte("developer ALL=(ALL) NOPASSWD: /usr/bin/id\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	// Earlier file targeting developer should NOT be reported as shadowing.
	if err := os.WriteFile(filepath.Join(dir, "10-earlier"), []byte("developer ALL=(ALL) /usr/bin/true\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	// Later file targeting unrelated user should NOT be reported as shadowing.
	if err := os.WriteFile(filepath.Join(dir, "80-other"), []byte("otheruser ALL=(ALL) /usr/bin/true\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	// Later file targeting developer SHOULD be reported as shadowing, without leaking secretCmd.
	if err := os.WriteFile(filepath.Join(dir, "90-cloud-init-users"), []byte("developer ALL=(ALL) NOPASSWD: "+secretCmd+"\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	// Inactive files (dotfile, dot in name, tilde backup) must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "99-ignored.bak"), []byte("developer ALL=(ALL) ALL\n"), 0o440); err != nil {
		t.Fatal(err)
	}

	applicator := sudo.New(res)
	configureTestSudoOwnership(t, applicator)
	applicator.SudoersDir = dir

	check := applicator.Check(context.Background())
	if err := check.Validate(); err != nil {
		t.Fatalf("CheckResult.Validate() error = %v", err)
	}
	if check.Status != executor.Compliant {
		t.Fatalf("Check().Status = %q, want compliant (non-looping)", check.Status)
	}
	if len(check.Subresults) != 1 {
		t.Fatalf("Subresults = %+v, want 1 shadowing subresult", check.Subresults)
	}
	sub := check.Subresults[0]
	if sub.Target != "90-cloud-init-users" || sub.Status != executor.Drifted || sub.ReasonCode != "sudo_fragment_shadowed" {
		t.Fatalf("shadow subresult = %+v, want target=90-cloud-init-users status=drifted reason=sudo_fragment_shadowed", sub)
	}
	if strings.Contains(string(check.ObservedSummary), secretCmd) || strings.Contains(string(sub.ObservedSummary), secretCmd) {
		t.Fatalf("shadow check leaked fragment content: summary=%q sub=%q", check.ObservedSummary, sub.ObservedSummary)
	}
}

// OS-LIA-019: opt-in auditUnmanaged reports unexpected active sudoers.d
// drop-ins as drifted with bounded, filename-only subresults, while Apply
// never deletes or mutates unmanaged files.
func TestSudoApplicatorAuditUnmanagedDetectsRogueFragmentsNonDestructively(t *testing.T) {
	const secretCanary = "REMOTR-UNMANAGED-SECRET-CANARY"
	root := t.TempDir()
	dir := filepath.Join(root, "sudoers.d")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	sudoers := filepath.Join(root, "sudoers")
	if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\nrecovery ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
		t.Fatal(err)
	}

	res := testSudoResource()
	res.AuditUnmanaged = true
	res.AllowedUnmanagedFragments = []string{"recovery"}

	managedPath := filepath.Join(dir, "developer-admin")
	if err := os.WriteFile(managedPath, []byte("developer ALL=(ALL) NOPASSWD: /usr/bin/id\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recovery"), []byte("recovery ALL=(ALL:ALL) ALL\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	// Ignored by sudoers (dot in name, dotfile, tilde suffix)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("doc\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backup~"), []byte("old\n"), 0o440); err != nil {
		t.Fatal(err)
	}

	applicator := sudo.New(res)
	configureTestSudoOwnership(t, applicator)
	applicator.SudoersDir, applicator.SudoersPath = dir, sudoers
	applicator.LookupRecovery = func(string) error { return nil }
	applicator.ValidateEffective = func(context.Context, string, string) error { return nil }
	applicator.VerifyRecoveryAccess = func(context.Context, string, string, string) error { return nil }

	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() with only allowed fragments = %+v, want compliant", check)
	}

	roguePath := filepath.Join(dir, "90-rogue")
	rogueContent := "rogue ALL=(ALL) NOPASSWD: /bin/" + secretCanary + "\n"
	if err := os.WriteFile(roguePath, []byte(rogueContent), 0o440); err != nil {
		t.Fatal(err)
	}

	check := applicator.Check(context.Background())
	if err := check.Validate(); err != nil {
		t.Fatalf("CheckResult.Validate() error = %v", err)
	}
	if check.Status != executor.Drifted || check.ReasonCode != "sudo_unmanaged_fragments" {
		t.Fatalf("Check() with rogue fragment = %+v, want drifted/sudo_unmanaged_fragments", check)
	}
	if len(check.Subresults) != 1 || check.Subresults[0].Target != "90-rogue" || check.Subresults[0].ReasonCode != "sudo_unmanaged_fragments" {
		t.Fatalf("Subresults = %+v, want single 90-rogue unmanaged subresult", check.Subresults)
	}
	if strings.Contains(string(check.ObservedSummary), secretCanary) || strings.Contains(string(check.Subresults[0].ObservedSummary), secretCanary) {
		t.Fatalf("audit leaked unmanaged fragment content: %+v", check)
	}

	// Apply must NOT delete or mutate the unmanaged fragment.
	if applyRes := applicator.ApplyResult(context.Background()); applyRes.Status != executor.NoChange || applyRes.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want no-change without touching unmanaged file", applyRes)
	}
	gotRogue, err := os.ReadFile(roguePath)
	if err != nil || string(gotRogue) != rogueContent {
		t.Fatalf("unmanaged fragment was mutated: %q, err=%v", string(gotRogue), err)
	}

	// Verify subresults are bounded at executor.MaxCheckSubresults.
	for i := 0; i < executor.MaxCheckSubresults+5; i++ {
		name := filepath.Join(dir, "unmanaged-"+string(rune('a'+(i/26)))+string(rune('a'+(i%26))))
		if err := os.WriteFile(name, []byte("other ALL=(ALL) /usr/bin/true\n"), 0o440); err != nil {
			t.Fatal(err)
		}
	}
	bounded := applicator.Check(context.Background())
	if err := bounded.Validate(); err != nil {
		t.Fatalf("bounded CheckResult.Validate() error = %v", err)
	}
	if len(bounded.Subresults) != executor.MaxCheckSubresults || !bounded.SubresultsTruncated {
		t.Fatalf("bounded subresults = %d truncated=%v, want %d/true", len(bounded.Subresults), bounded.SubresultsTruncated, executor.MaxCheckSubresults)
	}
}
