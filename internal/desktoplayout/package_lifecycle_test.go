package desktoplayout_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdvertisedLinuxPackageHasNativeInstallLifecycleEvidence(t *testing.T) {
	root := repositoryRoot(t)

	targetData, err := os.ReadFile(filepath.Join(root, "desktop", "build", "linux", "package-targets.json"))
	if err != nil {
		t.Errorf("read advertised Linux package targets: %v", err)
	} else {
		var targets struct {
			SchemaVersion       int    `json:"schemaVersion"`
			Product             string `json:"product"`
			SignedReleaseOutput struct {
				Configured bool   `json:"configured"`
				Policy     string `json:"policy"`
			} `json:"signedReleaseOutput"`
			Artifacts []struct {
				OS               string   `json:"os"`
				Architecture     string   `json:"architecture"`
				PackageFormat    string   `json:"packageFormat"`
				Publication      string   `json:"publication"`
				SigningStatus    string   `json:"signingStatus"`
				ReleaseEligible  bool     `json:"releaseEligible"`
				LifecycleImage   string   `json:"lifecycleContainer"`
				EvidenceCommand  string   `json:"evidenceCommand"`
				RequiredEvidence []string `json:"requiredEvidence"`
			} `json:"artifacts"`
		}
		if err := json.Unmarshal(targetData, &targets); err != nil {
			t.Errorf("parse advertised Linux package targets: %v", err)
		} else {
			if targets.SchemaVersion != 1 || targets.Product != "Remotr Desktop" {
				t.Errorf("package target identity = version %d/product %q", targets.SchemaVersion, targets.Product)
			}
			if targets.SignedReleaseOutput.Configured || targets.SignedReleaseOutput.Policy != "not-configured" {
				t.Errorf("signed release output = %#v, want explicitly not configured", targets.SignedReleaseOutput)
			}
			if len(targets.Artifacts) != 1 {
				t.Errorf("advertised Linux artifacts = %d, want exactly one", len(targets.Artifacts))
			} else {
				artifact := targets.Artifacts[0]
				if artifact.OS != "linux" || artifact.Architecture != "amd64" || artifact.PackageFormat != "deb" {
					t.Errorf("advertised package target = %s/%s %s, want linux/amd64 deb", artifact.OS, artifact.Architecture, artifact.PackageFormat)
				}
				if artifact.Publication != "ci-development-artifact" || artifact.SigningStatus != "unsigned" || artifact.ReleaseEligible {
					t.Errorf("advertised package classification = %#v, want unsigned non-release CI development artifact", artifact)
				}
				if artifact.LifecycleImage != "debian:13-slim@sha256:020c0d20b9880058cbe785a9db107156c3c75c2ac944a6aa7ab59f2add76a7bd" {
					t.Errorf("package lifecycle container = %q, want the reviewed immutable amd64 fixture", artifact.LifecycleImage)
				}
				if artifact.EvidenceCommand != "make desktop-package-smoke" || strings.Join(artifact.RequiredEvidence, ",") != "build,install,launch,remove" {
					t.Errorf("advertised package evidence = %q/%v", artifact.EvidenceCommand, artifact.RequiredEvidence)
				}
			}
		}
	}

	makefileData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read root Makefile: %v", err)
	}
	for _, fragment := range []string{
		"DESKTOP_DEB := $(DESKTOP_PACKAGE_DIR)/remotr-desktop_$(DESKTOP_VERSION)_amd64.deb",
		"desktop-package: desktop-build",
		"desktop-package-smoke: desktop-package",
		"./scripts/desktop-package-deb.sh",
		"./scripts/desktop-package-smoke.sh",
	} {
		if !strings.Contains(string(makefileData), fragment) {
			t.Errorf("root Makefile does not contain DEB lifecycle contract %q", fragment)
		}
	}

	workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "desktop.yml"))
	if err != nil {
		t.Fatalf("read desktop workflow: %v", err)
	}
	workflow := string(workflowData)
	smokeIndex := strings.Index(workflow, "run: make desktop-package-smoke")
	uploadIndex := strings.Index(workflow, "uses: actions/upload-artifact@v4")
	if smokeIndex < 0 || uploadIndex < 0 || uploadIndex < smokeIndex {
		t.Errorf("desktop workflow must package-smoke before artifact upload; smoke=%d upload=%d", smokeIndex, uploadIndex)
	}
	for _, fragment := range []string{
		"name: Build, install, launch, and remove unsigned Linux/amd64 DEB snapshot",
		"name: Upload evidenced unsigned Linux/amd64 DEB development snapshot",
		"name: remotr-desktop-linux-amd64-deb-unsigned-development-snapshot",
		"retention-days: 7",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("desktop workflow does not classify DEB snapshot with %q", fragment)
		}
	}
	if strings.Contains(workflow, "softprops/action-gh-release") || strings.Contains(workflow, "gh release upload") {
		t.Error("unsigned desktop development snapshot is exposed as a signed release output")
	}

	builder := filepath.Join(root, "scripts", "desktop-package-deb.sh")
	smoke := filepath.Join(root, "scripts", "desktop-package-smoke.sh")
	if _, err := os.Stat(builder); err != nil {
		t.Errorf("locate DEB builder: %v", err)
		return
	}
	if _, err := os.Stat(smoke); err != nil {
		t.Errorf("locate DEB lifecycle smoke: %v", err)
		return
	}

	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf 'Linux\\n'\n")
	writeExecutable(t, filepath.Join(bin, "xvfb-run"), `#!/bin/sh
if [ "${1-}" = "-a" ]; then
  shift
fi
exec "$@"
`)
	writeExecutable(t, filepath.Join(bin, "xwininfo"), `#!/bin/sh
printf '%s\n' '0x0400003 "Remotr Desktop": ("remotr-desktop" "remotr-desktop")'
`)
	dockerLog := filepath.Join(t.TempDir(), "docker.log")
	writeExecutable(t, filepath.Join(bin, "docker"), `#!/bin/sh
printf '%s\n' "$*" >"$FAKE_DOCKER_LOG"
printf 'container package-manager install/remove passed\n'
`)
	binary := filepath.Join(bin, "remotr-desktop")
	writeExecutable(t, binary, `#!/bin/sh
if [ "${1-}" = "--version" ]; then
  printf 'Remotr Desktop 0.0.0-test.1\n'
  exit 0
fi
exec tail -f /dev/null
`)

	deb := filepath.Join(t.TempDir(), "remotr-desktop_0.0.0-test.1_amd64.deb")
	build := exec.Command(builder,
		"--binary", binary,
		"--version", "0.0.0-test.1",
		"--architecture", "amd64",
		"--output", deb,
	)
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build focused DEB fixture: %v\n%s", err, output)
	}

	fields := exec.Command("dpkg-deb", "--field", deb, "Package", "Version", "Architecture")
	fieldOutput, err := fields.CombinedOutput()
	if err != nil {
		t.Fatalf("read focused DEB fields: %v\n%s", err, fieldOutput)
	}
	for _, fragment := range []string{"Package: remotr-desktop", "Version: 0.0.0-test.1", "Architecture: amd64"} {
		if !strings.Contains(string(fieldOutput), fragment) {
			t.Errorf("DEB fields = %q, want %q", fieldOutput, fragment)
		}
	}

	contents := exec.Command("dpkg-deb", "--contents", deb)
	contentOutput, err := contents.CombinedOutput()
	if err != nil {
		t.Fatalf("read focused DEB payload: %v\n%s", err, contentOutput)
	}
	for _, path := range []string{
		"./usr/bin/remotr-desktop",
		"./usr/share/applications/remotr-desktop.desktop",
		"./usr/share/metainfo/io.github.davidhoenisch.remotr.desktop.metainfo.xml",
		"./usr/share/icons/hicolor/256x256/apps/remotr-desktop.png",
	} {
		if !strings.Contains(string(contentOutput), path) {
			t.Errorf("DEB payload does not contain %s", path)
		}
	}

	lifecycle := exec.Command(smoke,
		"--package", deb,
		"--version", "0.0.0-test.1",
		"--native-smoke", filepath.Join(root, "scripts", "desktop-native-smoke.sh"),
	)
	lifecycle.Dir = root
	lifecycle.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_DOCKER_LOG="+dockerLog,
		"REMOTR_DESKTOP_SMOKE_ATTEMPTS=2",
		"REMOTR_DESKTOP_SMOKE_INTERVAL=0",
	)
	lifecycleOutput, err := lifecycle.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke focused DEB lifecycle: %v\n%s", err, lifecycleOutput)
	}
	if !strings.Contains(string(lifecycleOutput), "unsigned development snapshot install/launch/remove smoke passed") {
		t.Errorf("DEB lifecycle output = %q, want complete unsigned snapshot evidence", lifecycleOutput)
	}
	dockerInvocation, err := os.ReadFile(dockerLog)
	if err != nil {
		t.Fatalf("read container package-manager invocation: %v", err)
	}
	for _, fragment := range []string{"run --rm --platform linux/amd64", "debian:13-slim@sha256:020c0d20b9880058cbe785a9db107156c3c75c2ac944a6aa7ab59f2add76a7bd", ":/tmp/remotr-desktop.deb:ro"} {
		if !strings.Contains(string(dockerInvocation), fragment) {
			t.Errorf("container package-manager invocation = %q, want %q", dockerInvocation, fragment)
		}
	}

	t.Run("unsupported architecture", func(t *testing.T) {
		command := exec.Command(builder,
			"--binary", binary,
			"--version", "0.0.0-test.1",
			"--architecture", "arm64",
			"--output", filepath.Join(t.TempDir(), "unsupported.deb"),
		)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "only linux/amd64") {
			t.Errorf("unsupported architecture result = %v/%q, want linux/amd64 rejection", err, output)
		}
	})

	t.Run("mismatched package version", func(t *testing.T) {
		command := exec.Command(smoke,
			"--package", deb,
			"--version", "0.0.0-other",
			"--native-smoke", filepath.Join(root, "scripts", "desktop-native-smoke.sh"),
		)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "package version") {
			t.Errorf("mismatched version result = %v/%q, want package-version rejection", err, output)
		}
	})
}
