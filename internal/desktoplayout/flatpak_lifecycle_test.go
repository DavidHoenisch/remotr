package desktoplayout_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdvertisedFlatpakHasNativeInstallLifecycleEvidence(t *testing.T) {
	root := repositoryRoot(t)

	type flatpakTargetContract struct {
		OS               string   `json:"os"`
		Architecture     string   `json:"architecture"`
		PackageFormat    string   `json:"packageFormat"`
		Publication      string   `json:"publication"`
		SigningStatus    string   `json:"signingStatus"`
		ReleaseEligible  bool     `json:"releaseEligible"`
		Runtime          string   `json:"runtime"`
		EvidenceCommand  string   `json:"evidenceCommand"`
		RequiredEvidence []string `json:"requiredEvidence"`
	}
	targetData, err := os.ReadFile(filepath.Join(root, "desktop", "build", "linux", "package-targets.json"))
	if err != nil {
		t.Fatalf("read advertised Linux package targets: %v", err)
	}
	var targets struct {
		Artifacts []flatpakTargetContract `json:"artifacts"`
	}
	if err := json.Unmarshal(targetData, &targets); err != nil {
		t.Fatalf("parse advertised Linux package targets: %v", err)
	}
	var flatpakTarget *flatpakTargetContract
	for index := range targets.Artifacts {
		if targets.Artifacts[index].PackageFormat == "flatpak" {
			flatpakTarget = &targets.Artifacts[index]
		}
	}
	if flatpakTarget == nil {
		t.Fatal("Linux/amd64 Flatpak target is not advertised")
	}
	if flatpakTarget.OS != "linux" || flatpakTarget.Architecture != "amd64" || flatpakTarget.Publication != "github-release-asset" || flatpakTarget.SigningStatus != "unsigned" || !flatpakTarget.ReleaseEligible {
		t.Errorf("Flatpak publication target = %#v", flatpakTarget)
	}
	if flatpakTarget.Runtime != "org.gnome.Platform//50" || flatpakTarget.EvidenceCommand != "make desktop-flatpak-smoke" || strings.Join(flatpakTarget.RequiredEvidence, ",") != "build,install,launch,remove" {
		t.Errorf("Flatpak runtime/evidence = %q %q %v", flatpakTarget.Runtime, flatpakTarget.EvidenceCommand, flatpakTarget.RequiredEvidence)
	}

	makefileData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read root Makefile: %v", err)
	}
	for _, fragment := range []string{
		"DESKTOP_FLATPAK := $(DESKTOP_FLATPAK_PACKAGE_DIR)/remotr-desktop_$(DESKTOP_VERSION)_amd64.flatpak",
		"desktop-flatpak: desktop-build",
		"desktop-flatpak-smoke: desktop-flatpak",
		"./scripts/desktop-package-flatpak.sh",
		"./scripts/desktop-flatpak-smoke.sh",
	} {
		if !strings.Contains(string(makefileData), fragment) {
			t.Errorf("root Makefile does not contain Flatpak lifecycle contract %q", fragment)
		}
	}

	builder := filepath.Join(root, "scripts", "desktop-package-flatpak.sh")
	smoke := filepath.Join(root, "scripts", "desktop-flatpak-smoke.sh")
	for _, script := range []string{builder, smoke} {
		info, err := os.Stat(script)
		if err != nil {
			t.Fatalf("locate Flatpak lifecycle script %s: %v", script, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("Flatpak lifecycle script is not executable: %s", script)
		}
	}

	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "uname"), `#!/bin/sh
if [ "${1-}" = "-m" ]; then
  printf 'x86_64\n'
else
  printf 'Linux\n'
fi
`)
	builderLog := filepath.Join(t.TempDir(), "flatpak-builder.log")
	writeExecutable(t, filepath.Join(bin, "flatpak-builder"), `#!/bin/sh
printf '%s\n' "$*" >"$FAKE_FLATPAK_BUILDER_LOG"
`)
	flatpakLog := filepath.Join(t.TempDir(), "flatpak.log")
	state := filepath.Join(t.TempDir(), "installed")
	writeExecutable(t, filepath.Join(bin, "flatpak"), `#!/bin/sh
printf '%s\n' "$*" >>"$FAKE_FLATPAK_LOG"
case "${1-}" in
  build-bundle)
    : >"$5"
    ;;
  install)
    : >"$FAKE_FLATPAK_STATE"
    ;;
  info)
    test -f "$FAKE_FLATPAK_STATE"
    ;;
  run)
    case "$*" in
      *'--version'*)
        case "$*" in
          *'--env=GSETTINGS_BACKEND=memory'*) ;;
          *) printf '%s\n' "dconf-CRITICAL: unable to create /run/user dconf state" >&2 ;;
        esac
        printf 'Remotr Desktop 0.0.0-test.1\n'
        ;;
      *'--command=sh'*) exit 0 ;;
      *)
        test -z "${WAYLAND_DISPLAY-}" || exit 44
        exec tail -f /dev/null
        ;;
    esac
    ;;
  uninstall)
    rm -f "$FAKE_FLATPAK_STATE"
    ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "xvfb-run"), `#!/bin/sh
if [ "${1-}" = "-a" ]; then
  shift
fi
exec "$@"
`)
	writeExecutable(t, filepath.Join(bin, "xwininfo"), `#!/bin/sh
printf '%s\n' '0x0400003 "Remotr Desktop": ("remotr-desktop" "remotr-desktop")'
`)
	binary := filepath.Join(bin, "remotr-desktop")
	writeExecutable(t, binary, `#!/bin/sh
if [ "${1-}" = "--version" ]; then
  printf 'Remotr Desktop 0.0.0-test.1\n'
  exit 0
fi
exit 1
`)
	bundle := filepath.Join(t.TempDir(), "remotr-desktop_0.0.0-test.1_amd64.flatpak")
	commonEnvironment := append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_FLATPAK_BUILDER_LOG="+builderLog,
		"FAKE_FLATPAK_LOG="+flatpakLog,
		"FAKE_FLATPAK_STATE="+state,
		"WAYLAND_DISPLAY=wayland-test",
	)

	build := exec.Command(builder,
		"--binary", binary,
		"--version", "0.0.0-test.1",
		"--architecture", "amd64",
		"--output", bundle,
	)
	build.Dir = root
	build.Env = commonEnvironment
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build focused Flatpak fixture: %v\n%s", err, output)
	}
	if info, err := os.Stat(bundle); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("Flatpak bundle was not produced: %v", err)
	}
	builderInvocation, err := os.ReadFile(builderLog)
	if err != nil {
		t.Fatalf("read flatpak-builder invocation: %v", err)
	}
	for _, fragment := range []string{"--state-dir=", "--arch=x86_64", "--default-branch=stable", "io.github.davidhoenisch.remotr.desktop.json"} {
		if !strings.Contains(string(builderInvocation), fragment) {
			t.Errorf("flatpak-builder invocation = %q, want %q", builderInvocation, fragment)
		}
	}

	lifecycle := exec.Command(smoke, "--package", bundle, "--version", "0.0.0-test.1")
	lifecycle.Dir = root
	lifecycle.Env = append(commonEnvironment,
		"REMOTR_DESKTOP_SMOKE_ATTEMPTS=2",
		"REMOTR_DESKTOP_SMOKE_INTERVAL=0",
	)
	lifecycleOutput, err := lifecycle.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke focused Flatpak lifecycle: %v\n%s", err, lifecycleOutput)
	}
	if !strings.Contains(string(lifecycleOutput), "unsigned GitHub release asset install/launch/remove smoke passed") {
		t.Errorf("Flatpak lifecycle output = %q, want complete release evidence", lifecycleOutput)
	}
	if strings.Contains(string(lifecycleOutput), "dconf-CRITICAL") {
		t.Errorf("Flatpak lifecycle output contains avoidable dconf warning: %q", lifecycleOutput)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("Flatpak fake installation remains after smoke: %v", err)
	}
	flatpakInvocation, err := os.ReadFile(flatpakLog)
	if err != nil {
		t.Fatalf("read Flatpak command log: %v", err)
	}
	for _, fragment := range []string{
		"build-bundle --arch=x86_64 --runtime-repo=https://dl.flathub.org/repo/flathub.flatpakrepo",
		"install --user --noninteractive",
		`test -r "$HOME/.config/remotr/flatpak-smoke-config"`,
		`test ! -r "$HOME/.ssh/remotr-flatpak-forbidden-canary"`,
		"run --user --env=GSETTINGS_BACKEND=memory io.github.davidhoenisch.remotr.desktop --version",
		"run --user --nosocket=wayland --socket=x11 --env=GDK_BACKEND=x11 --env=WEBKIT_DISABLE_COMPOSITING_MODE=1 io.github.davidhoenisch.remotr.desktop",
		"uninstall --user --noninteractive --delete-data io.github.davidhoenisch.remotr.desktop",
	} {
		if !strings.Contains(string(flatpakInvocation), fragment) {
			t.Errorf("Flatpak lifecycle commands = %q, want %q", flatpakInvocation, fragment)
		}
	}
}
