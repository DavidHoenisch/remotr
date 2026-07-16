package desktoplayout_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopRootTargetsUsePinnedNestedModuleWorkflow(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read root Makefile: %v", err)
	}

	makefile := string(data)
	required := []string{
		"DESKTOP_DIR := $(CURDIR)/desktop",
		"DESKTOP_FRONTEND_DIR := $(DESKTOP_DIR)/frontend",
		"DESKTOP_PNPM ?= env COREPACK_HOME=$(DESKTOP_DIR)/.cache/corepack corepack pnpm@11.7.0",
		"DESKTOP_WAILS_VERSION := v2.12.0",
		"desktop-test: desktop-setup",
		"desktop-dev: desktop-linux-prerequisites desktop-setup",
		"desktop-build: desktop-linux-prerequisites desktop-setup",
		"cd $(DESKTOP_FRONTEND_DIR) && $(DESKTOP_PNPM) install --frozen-lockfile",
		"cd $(DESKTOP_DIR) && go test ./...",
		"cd $(DESKTOP_FRONTEND_DIR) && $(DESKTOP_PNPM) test",
		"go run github.com/wailsapp/wails/v2/cmd/wails@$(DESKTOP_WAILS_VERSION) dev",
		"go run github.com/wailsapp/wails/v2/cmd/wails@$(DESKTOP_WAILS_VERSION) build",
	}
	for _, fragment := range required {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("root Makefile does not contain required desktop command contract %q", fragment)
		}
	}

	for _, target := range []string{"desktop-test", "desktop-dev", "desktop-build"} {
		command := exec.Command("make", "-n", target)
		command.Dir = root
		if output, runErr := command.CombinedOutput(); runErr != nil {
			t.Errorf("make -n %s failed: %v\n%s", target, runErr, output)
		}
	}

	wailsConfig, err := os.ReadFile(filepath.Join(root, "desktop", "wails.json"))
	if err != nil {
		t.Fatalf("read Wails configuration: %v", err)
	}
	for _, fragment := range []string{
		`"frontend:install": "env COREPACK_HOME=../.cache/corepack corepack pnpm@11.7.0 install --frozen-lockfile"`,
		`"frontend:build": "env COREPACK_HOME=../.cache/corepack corepack pnpm@11.7.0 run build"`,
		`"frontend:dev:watcher": "env COREPACK_HOME=../.cache/corepack corepack pnpm@11.7.0 run dev"`,
		`"frontend:dev:serverUrl": "auto"`,
	} {
		if !strings.Contains(string(wailsConfig), fragment) {
			t.Errorf("desktop/wails.json does not contain required frontend command %q", fragment)
		}
	}
}

func TestDesktopLinuxPrerequisitesSelectSupportedWebKit(t *testing.T) {
	root := repositoryRoot(t)
	script := filepath.Join(root, "scripts", "desktop-linux-prerequisites.sh")

	tests := []struct {
		name      string
		operating string
		gtk       bool
		webkit41  bool
		webkit40  bool
		wantTags  string
		wantErr   string
	}{
		{name: "WebKitGTK 4.1", operating: "Linux", gtk: true, webkit41: true, wantTags: "webkit2_41"},
		{name: "WebKitGTK 4.0", operating: "Linux", gtk: true, webkit40: true},
		{name: "non-Linux", operating: "Darwin", gtk: true, webkit41: true, wantErr: "Linux"},
		{name: "missing GTK", operating: "Linux", webkit41: true, wantErr: "GTK 3"},
		{name: "missing WebKitGTK", operating: "Linux", gtk: true, wantErr: "WebKitGTK 4.1 or 4.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf '%s\\n' \"$FAKE_OS\"\n")
			writeExecutable(t, filepath.Join(bin, "pkg-config"), `#!/bin/sh
if [ "$1" != "--exists" ]; then
  exit 2
fi
case "$2" in
  gtk+-3.0) test "$FAKE_GTK" = "1" ;;
  webkit2gtk-4.1) test "$FAKE_WEBKIT_41" = "1" ;;
  webkit2gtk-4.0) test "$FAKE_WEBKIT_40" = "1" ;;
  *) exit 1 ;;
esac
`)

			command := exec.Command(script, "--wails-tags")
			command.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"FAKE_OS="+test.operating,
				boolEnvironment("FAKE_GTK", test.gtk),
				boolEnvironment("FAKE_WEBKIT_41", test.webkit41),
				boolEnvironment("FAKE_WEBKIT_40", test.webkit40),
			)
			output, runErr := command.CombinedOutput()
			if test.wantErr != "" {
				if runErr == nil {
					t.Fatalf("prerequisite check succeeded, want error containing %q", test.wantErr)
				}
				if !strings.Contains(string(output), test.wantErr) {
					t.Fatalf("prerequisite error = %q, want substring %q", output, test.wantErr)
				}
				return
			}
			if runErr != nil {
				t.Fatalf("prerequisite check failed: %v\n%s", runErr, output)
			}
			if got := strings.TrimSpace(string(output)); got != test.wantTags {
				t.Errorf("Wails tags = %q, want %q", got, test.wantTags)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate desktop root-target test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write fake executable %s: %v", path, err)
	}
}

func boolEnvironment(name string, value bool) string {
	if value {
		return name + "=1"
	}
	return name + "=0"
}
