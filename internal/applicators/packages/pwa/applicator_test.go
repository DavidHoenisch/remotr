package pwa_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pwa"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func testUser(base string) interactiveuser.Account {
	uid := os.Getuid()
	gid := os.Getgid()
	home := filepath.Join(base, "home")
	_ = os.MkdirAll(home, 0o755)
	name := os.Getenv("USER")
	if name == "" {
		name = "testuser"
	}
	return interactiveuser.Account{
		Username: name,
		UID:      uid,
		GID:      gid,
		HomeDir:  home,
	}
}

func testPackage() models.Package {
	return models.Package{
		Name:     "slack",
		Present:  true,
		PM:       types.Pwa,
		PWAURL:   "https://app.slack.com/client",
		PWATitle: "Slack",
	}
}

func TestApplicator_applyInstallDesktopEntry(t *testing.T) {
	dir := t.TempDir()
	user := testUser(dir)
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"which [chromium]": {Stdout: []byte("/usr/bin/chromium\n")},
		},
	}
	a := pwa.New(testPackage(), mock)
	a.ListUsers = func() ([]interactiveuser.Account, error) { return []interactiveuser.Account{user}, nil }

	ctx := context.Background()
	if _, met := a.State(ctx); met {
		t.Fatal("expected drift before apply")
	}
	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	desktop := filepath.Join(user.HomeDir, ".local", "share", "applications", "remotr-pwa-slack.desktop")
	data, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "Name=Slack") {
		t.Fatalf("desktop missing title: %q", body)
	}
	if !strings.Contains(body, "--app=https://app.slack.com/client") {
		t.Fatalf("desktop missing app URL: %q", body)
	}
	if !strings.Contains(body, "StartupWMClass=app.slack.com") {
		t.Fatalf("desktop missing wm class: %q", body)
	}
	st, err := os.Stat(desktop)
	if err != nil {
		t.Fatal(err)
	}
	if stat, ok := st.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != user.UID {
		t.Fatalf("desktop not owned by uid %d", user.UID)
	}
	if _, met := a.State(ctx); !met {
		t.Fatal("expected compliant after apply")
	}
}

func TestApplicator_applyRemove(t *testing.T) {
	dir := t.TempDir()
	user := testUser(dir)
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"which [chromium]": {Stdout: []byte("/usr/bin/chromium\n")},
		},
	}
	pkg := testPackage()
	pkg.Present = false
	a := pwa.New(pkg, mock)
	a.ListUsers = func() ([]interactiveuser.Account, error) { return []interactiveuser.Account{user}, nil }

	desktop := filepath.Join(user.HomeDir, ".local", "share", "applications", "remotr-pwa-slack.desktop")
	if err := os.MkdirAll(filepath.Dir(desktop), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktop, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(desktop); !os.IsNotExist(err) {
		t.Fatal("expected desktop removed")
	}
}

func TestApplicator_customBrowser(t *testing.T) {
	dir := t.TempDir()
	user := testUser(dir)
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"which [google-chrome-stable]": {Stdout: []byte("/usr/bin/google-chrome-stable\n")},
		},
	}
	pkg := testPackage()
	pkg.PWABrowser = "google-chrome-stable"
	a := pwa.New(pkg, mock)
	a.ListUsers = func() ([]interactiveuser.Account, error) { return []interactiveuser.Account{user}, nil }

	ctx := context.Background()
	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	desktop := filepath.Join(user.HomeDir, ".local", "share", "applications", "remotr-pwa-slack.desktop")
	data, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/usr/bin/google-chrome-stable --app=") {
		t.Fatalf("unexpected exec line: %q", data)
	}
}

func TestApplicator_applyInstallIcon(t *testing.T) {
	dir := t.TempDir()
	user := testUser(dir)
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"which [chromium]": {Stdout: []byte("/usr/bin/chromium\n")},
		},
	}
	pkg := testPackage()
	pkg.PWAIcon = "https://example.com/icon.png"
	a := pwa.New(pkg, mock)
	a.ListUsers = func() ([]interactiveuser.Account, error) { return []interactiveuser.Account{user}, nil }
	a.FetchURL = func(_ context.Context, rawURL string) ([]byte, error) {
		if rawURL != pkg.PWAIcon {
			return nil, fmt.Errorf("unexpected url %q", rawURL)
		}
		return []byte("png-bytes"), nil
	}

	ctx := context.Background()
	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	icon := filepath.Join(user.HomeDir, ".local", "share", "icons", "hicolor", "256x256", "apps", "remotr-pwa-slack.png")
	data, err := os.ReadFile(icon)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "png-bytes" {
		t.Fatalf("icon content = %q", data)
	}
	iconDirs := []string{
		filepath.Join(user.HomeDir, ".local", "share", "icons"),
		filepath.Join(user.HomeDir, ".local", "share", "icons", "hicolor"),
		filepath.Join(user.HomeDir, ".local", "share", "icons", "hicolor", "256x256"),
		filepath.Join(user.HomeDir, ".local", "share", "icons", "hicolor", "256x256", "apps"),
	}
	for _, dir := range iconDirs {
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		stat, ok := st.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != user.UID {
			t.Fatalf("%s owned by uid %d, want %d", dir, stat.Uid, user.UID)
		}
	}
	st, err := os.Stat(icon)
	if err != nil {
		t.Fatal(err)
	}
	if stat, ok := st.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != user.UID {
		t.Fatalf("icon not owned by uid %d", user.UID)
	}
}

func TestApplicator_noBrowserFound(t *testing.T) {
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	a := pwa.New(testPackage(), mock)
	a.ListUsers = func() ([]interactiveuser.Account, error) {
		return []interactiveuser.Account{testUser(t.TempDir())}, nil
	}
	err := a.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no supported browser found") {
		t.Fatalf("Apply() = %v, want browser error", err)
	}
}

func TestApplicator_rejectsSymlinkedUserTreeDir(t *testing.T) {
	dir := t.TempDir()
	user := testUser(dir)
	share := filepath.Join(user.HomeDir, ".local", "share")
	if err := os.MkdirAll(share, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(share, "applications")); err != nil {
		t.Fatal(err)
	}
	mock := &executil.MockRunner{
		Next: map[string]executil.MockResult{
			"which [chromium]": {Stdout: []byte("/usr/bin/chromium\n")},
		},
	}
	a := pwa.New(testPackage(), mock)
	a.ListUsers = func() ([]interactiveuser.Account, error) { return []interactiveuser.Account{user}, nil }

	err := a.Apply(context.Background())
	if err == nil {
		t.Fatal("expected symlinked applications directory to be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "remotr-pwa-slack.desktop")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no desktop entry through symlink, stat err = %v", statErr)
	}
}
