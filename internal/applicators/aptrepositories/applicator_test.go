package aptrepositories_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/aptrepositories"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// OS-PRM-011: repository configuration is an owned, canonical APT fragment
// rather than an edit to distribution-managed source lists.
func TestApplicator_writesCanonicalNamedRepositoryAndPriority(t *testing.T) {
	root := t.TempDir()
	applicator := aptrepositories.New(models.APTRepository{
		Name:          "vendor",
		URL:           "https://packages.example.test/debian",
		Suites:        []string{"stable", "stable-updates"},
		Components:    []string{"main", "extras"},
		Architectures: []string{"amd64", "arm64"},
		SigningKey:    "vendor",
		Priority:      700,
	}, nil)
	applicator.SourcesDir = filepath.Join(root, "sources.list.d")
	applicator.PreferencesDir = filepath.Join(root, "preferences.d")
	applicator.AuthDir = filepath.Join(root, "auth.conf.d")

	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	source, err := os.ReadFile(filepath.Join(applicator.SourcesDir, "remotr-vendor.list"))
	if err != nil {
		t.Fatal(err)
	}
	wantSource := "deb [arch=amd64,arm64 signed-by=/etc/apt/keyrings/vendor.gpg] https://packages.example.test/debian stable main extras\n" +
		"deb [arch=amd64,arm64 signed-by=/etc/apt/keyrings/vendor.gpg] https://packages.example.test/debian stable-updates main extras\n"
	if string(source) != wantSource {
		t.Fatalf("source fragment = %q, want %q", source, wantSource)
	}
	preference, err := os.ReadFile(filepath.Join(applicator.PreferencesDir, "remotr-vendor.pref"))
	if err != nil {
		t.Fatal(err)
	}
	wantPreference := "Package: *\nPin: origin \"packages.example.test\"\nPin-Priority: 700\n"
	if string(preference) != wantPreference {
		t.Fatalf("preference fragment = %q, want %q", preference, wantPreference)
	}
}

// OS-PRM-012: absence removes exactly the resource's three owned fragments.
func TestApplicator_absentRemovesOnlyOwnedRepositoryFragments(t *testing.T) {
	root := t.TempDir()
	sources := filepath.Join(root, "sources.list.d")
	preferences := filepath.Join(root, "preferences.d")
	auth := filepath.Join(root, "auth.conf.d")
	for path, content := range map[string]string{
		filepath.Join(sources, "remotr-vendor.list"):        "owned source",
		filepath.Join(preferences, "remotr-vendor.pref"):    "owned preference",
		filepath.Join(auth, "remotr-vendor.conf"):           "owned auth",
		filepath.Join(sources, "distribution-managed.list"): "unrelated source",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	applicator := aptrepositories.New(models.APTRepository{
		Name: "vendor", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	}, nil)
	applicator.SourcesDir, applicator.PreferencesDir, applicator.AuthDir = sources, preferences, auth
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	for _, path := range []string{filepath.Join(sources, "remotr-vendor.list"), filepath.Join(preferences, "remotr-vendor.pref"), filepath.Join(auth, "remotr-vendor.conf")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("owned fragment %q remains: %v", path, err)
		}
	}
	unrelated, err := os.ReadFile(filepath.Join(sources, "distribution-managed.list"))
	if err != nil || string(unrelated) != "unrelated source" {
		t.Fatalf("unrelated source changed: %q, %v", unrelated, err)
	}
}

// OS-PRM-015: credentials are written only to APT's protected auth fragment,
// never included in source fragments or check diagnostics.
func TestApplicator_credentialReferenceDoesNotLeakIntoSourceOrCheck(t *testing.T) {
	const canary = "remotr-apt-secret-canary"
	root := t.TempDir()
	applicator := aptrepositories.New(models.APTRepository{
		Name: "private", URL: "https://packages.example.test/private", Suites: []string{"stable"}, Components: []string{"main"}, SigningKey: "vendor", CredentialRef: "file:/run/remotr/private-auth",
	}, nil)
	applicator.SourcesDir = filepath.Join(root, "sources.list.d")
	applicator.PreferencesDir = filepath.Join(root, "preferences.d")
	applicator.AuthDir = filepath.Join(root, "auth.conf.d")
	applicator.ResolveCredential = func(context.Context, string) (string, error) {
		return "machine packages.example.test login remotr password " + canary, nil
	}
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	source, err := os.ReadFile(filepath.Join(applicator.SourcesDir, "remotr-private.list"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), canary) {
		t.Fatalf("source fragment leaked credential canary: %q", source)
	}
	credentialPath := filepath.Join(applicator.AuthDir, "remotr-private.conf")
	info, err := os.Stat(credentialPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode = %v / %v, want 0600", info, err)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant || (check.Err != nil && strings.Contains(check.Err.Error(), canary)) || strings.Contains(string(check.DesiredSummary)+string(check.ObservedSummary), canary) {
		t.Fatalf("Check() leaked credential or is not compliant: %+v", check)
	}
}

func TestApplicatorProtectedMultiFileRollbackSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	const previousCredential = "machine old.example login old password apt-rollback-canary"
	root := t.TempDir()
	sources := filepath.Join(root, "sources.list.d")
	preferences := filepath.Join(root, "preferences.d")
	auth := filepath.Join(root, "auth.conf.d")
	paths := []string{
		filepath.Join(sources, "remotr-vendor.list"),
		filepath.Join(preferences, "remotr-vendor.pref"),
		filepath.Join(auth, "remotr-vendor.conf"),
	}
	for index, content := range []string{"previous source\n", "previous priority\n", previousCredential + "\n"} {
		if err := os.MkdirAll(filepath.Dir(paths[index]), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if index == 2 {
			mode = 0o600
		}
		if err := os.WriteFile(paths[index], []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	resource := models.APTRepository{
		Name: "vendor", URL: "https://packages.example.test/debian", Suites: []string{"stable"},
		Components: []string{"main"}, SigningKey: "vendor", Priority: 700, CredentialRef: "file:/run/remotr/vendor-auth",
	}
	rollbackRoot := filepath.Join(root, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	first := aptrepositories.New(resource, nil)
	first.SourcesDir, first.PreferencesDir, first.AuthDir = sources, preferences, auth
	first.ResolveCredential = func(context.Context, string) (string, error) {
		return "machine packages.example.test login remotr password replacement", nil
	}
	if err := first.ConfigureRollback(store, "base/vendor-repository", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if result := first.ApplyResult(ctx); result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if err := filepath.Walk(rollbackRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, []byte(previousCredential)) {
			t.Fatalf("protected repository rollback exposed credential canary in %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := aptrepositories.New(resource, nil)
	restarted.SourcesDir, restarted.PreferencesDir, restarted.AuthDir = sources, preferences, auth
	if err := restarted.ConfigureRollback(restartedStore, "base/vendor-repository", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	for index, want := range []string{"previous source\n", "previous priority\n", previousCredential + "\n"} {
		if got, err := os.ReadFile(paths[index]); err != nil || string(got) != want {
			t.Fatalf("restored fragment %d = %q, %v", index, got, err)
		}
	}
}
