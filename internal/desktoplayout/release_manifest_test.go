package desktoplayout_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopReleaseManifestRejectsUnevidencedAndNonLinuxArtifacts(t *testing.T) {
	root := repositoryRoot(t)
	tool := filepath.Join(root, "scripts", "desktop-release-manifest.py")
	targets := filepath.Join(root, "desktop", "build", "linux", "package-targets.json")

	makefileData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read root Makefile: %v", err)
	}
	for _, fragment := range []string{
		"DESKTOP_RELEASE_MANIFEST := $(DESKTOP_PACKAGE_DIR)/release-manifest.json",
		"desktop-release-manifest: desktop-package-smoke",
		"desktop-release-check: desktop-release-manifest",
		"python3 ./scripts/desktop-release-manifest.py generate",
		"python3 ./scripts/desktop-release-manifest.py check",
	} {
		if !strings.Contains(string(makefileData), fragment) {
			t.Errorf("root Makefile does not contain release-manifest gate %q", fragment)
		}
	}

	workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "desktop.yml"))
	if err != nil {
		t.Fatalf("read desktop workflow: %v", err)
	}
	workflow := string(workflowData)
	checkIndex := strings.Index(workflow, "run: make desktop-release-check")
	uploadIndex := strings.Index(workflow, "uses: actions/upload-artifact@v4")
	if checkIndex < 0 || uploadIndex < 0 || uploadIndex < checkIndex {
		t.Errorf("desktop workflow must release-check before upload; check=%d upload=%d", checkIndex, uploadIndex)
	}
	if !strings.Contains(workflow, "desktop/build/package/release-manifest.json") {
		t.Error("desktop workflow does not upload the checked release manifest with the DEB")
	}

	if _, err := os.Stat(tool); err != nil {
		t.Errorf("locate release-manifest tool: %v", err)
		return
	}

	artifactDir := t.TempDir()
	artifactName := "remotr-desktop_0.0.0-test.1_amd64.deb"
	artifactPath := filepath.Join(artifactDir, artifactName)
	artifactData := []byte("controlled linux amd64 deb fixture\n")
	if err := os.WriteFile(artifactPath, artifactData, 0o600); err != nil {
		t.Fatalf("write controlled artifact: %v", err)
	}
	manifestPath := filepath.Join(artifactDir, "release-manifest.json")

	generate := exec.Command("python3", tool,
		"generate",
		"--targets", targets,
		"--artifact", artifactPath,
		"--version", "0.0.0-test.1",
		"--os", "linux",
		"--architecture", "amd64",
		"--format", "deb",
		"--output", manifestPath,
	)
	generate.Dir = root
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate focused release manifest: %v\n%s", err, output)
	}

	checkManifest := func(path string) ([]byte, error) {
		command := exec.Command("python3", tool,
			"check",
			"--targets", targets,
			"--manifest", path,
			"--artifact-dir", artifactDir,
		)
		command.Dir = root
		return command.CombinedOutput()
	}
	if output, err := checkManifest(manifestPath); err != nil {
		t.Fatalf("check focused release manifest: %v\n%s", err, output)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read generated release manifest: %v", err)
	}
	var baseline map[string]any
	if err := json.Unmarshal(manifestData, &baseline); err != nil {
		t.Fatalf("parse generated release manifest: %v", err)
	}
	artifacts, ok := baseline["artifacts"].([]any)
	if !ok || len(artifacts) != 1 {
		t.Fatalf("generated artifacts = %#v, want one", baseline["artifacts"])
	}
	artifact, ok := artifacts[0].(map[string]any)
	if !ok {
		t.Fatalf("generated artifact = %#v, want object", artifacts[0])
	}
	wantDigest := sha256.Sum256(artifactData)
	if artifact["sha256"] != hex.EncodeToString(wantDigest[:]) || artifact["size"] != float64(len(artifactData)) {
		t.Errorf("generated artifact integrity = %v/%v, want controlled digest and size", artifact["sha256"], artifact["size"])
	}

	tests := []struct {
		name    string
		mutate  func(map[string]any, map[string]any)
		extra   string
		wantErr string
	}{
		{
			name: "Windows artifact",
			mutate: func(_ map[string]any, artifact map[string]any) {
				artifact["os"] = "windows"
			},
			wantErr: "Linux-only",
		},
		{
			name: "macOS artifact",
			mutate: func(_ map[string]any, artifact map[string]any) {
				artifact["os"] = "darwin"
			},
			wantErr: "Linux-only",
		},
		{
			name: "unevidenced architecture",
			mutate: func(_ map[string]any, artifact map[string]any) {
				artifact["architecture"] = "arm64"
			},
			wantErr: "not advertised with native evidence",
		},
		{
			name: "unevidenced format",
			mutate: func(_ map[string]any, artifact map[string]any) {
				artifact["packageFormat"] = "rpm"
			},
			wantErr: "not advertised with native evidence",
		},
		{
			name: "missing launch evidence",
			mutate: func(_ map[string]any, artifact map[string]any) {
				evidence := artifact["evidence"].(map[string]any)
				evidence["launch"] = "failed"
			},
			wantErr: "launch evidence is not passed",
		},
		{
			name: "digest mismatch",
			mutate: func(_ map[string]any, artifact map[string]any) {
				artifact["sha256"] = strings.Repeat("0", 64)
			},
			wantErr: "SHA-256 mismatch",
		},
		{
			name:    "undeclared macOS file",
			mutate:  func(map[string]any, map[string]any) {},
			extra:   "remotr-desktop.dmg",
			wantErr: "undeclared artifact",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(baseline)
			if err != nil {
				t.Fatalf("clone baseline manifest: %v", err)
			}
			var candidate map[string]any
			if err := json.Unmarshal(encoded, &candidate); err != nil {
				t.Fatalf("decode candidate manifest: %v", err)
			}
			candidateArtifact := candidate["artifacts"].([]any)[0].(map[string]any)
			test.mutate(candidate, candidateArtifact)
			candidateData, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("encode candidate manifest: %v", err)
			}
			if err := os.WriteFile(manifestPath, candidateData, 0o600); err != nil {
				t.Fatalf("write candidate manifest: %v", err)
			}
			t.Cleanup(func() { _ = os.WriteFile(manifestPath, manifestData, 0o600) })
			if test.extra != "" {
				extraPath := filepath.Join(artifactDir, test.extra)
				if err := os.WriteFile(extraPath, []byte("forbidden"), 0o600); err != nil {
					t.Fatalf("write undeclared artifact: %v", err)
				}
				t.Cleanup(func() { _ = os.Remove(extraPath) })
			}
			output, err := checkManifest(manifestPath)
			if err == nil || !strings.Contains(string(output), test.wantErr) {
				t.Errorf("manifest rejection = %v/%q, want %q", err, output, test.wantErr)
			}
		})
	}
}

func TestDesktopReleaseManifestAcceptsEvidencedFlatpakReleaseAsset(t *testing.T) {
	root := repositoryRoot(t)
	tool := filepath.Join(root, "scripts", "desktop-release-manifest.py")
	targets := filepath.Join(root, "desktop", "build", "linux", "package-targets.json")
	artifactDir := t.TempDir()
	artifactName := "remotr-desktop_1.2.3_amd64.flatpak"
	artifactPath := filepath.Join(artifactDir, artifactName)
	artifactData := []byte("controlled linux amd64 Flatpak fixture\n")
	if err := os.WriteFile(artifactPath, artifactData, 0o600); err != nil {
		t.Fatalf("write controlled Flatpak artifact: %v", err)
	}
	manifestPath := filepath.Join(artifactDir, "release-manifest.json")

	generate := exec.Command("python3", tool,
		"generate",
		"--targets", targets,
		"--artifact", artifactPath,
		"--version", "1.2.3",
		"--os", "linux",
		"--architecture", "amd64",
		"--format", "flatpak",
		"--output", manifestPath,
	)
	generate.Dir = root
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate focused Flatpak release manifest: %v\n%s", err, output)
	}

	check := exec.Command("python3", tool,
		"check",
		"--targets", targets,
		"--manifest", manifestPath,
		"--artifact-dir", artifactDir,
	)
	check.Dir = root
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("check focused Flatpak release manifest: %v\n%s", err, output)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read generated Flatpak release manifest: %v", err)
	}
	var manifest struct {
		Artifacts []struct {
			PackageFormat   string `json:"packageFormat"`
			Publication     string `json:"publication"`
			SigningStatus   string `json:"signingStatus"`
			ReleaseEligible bool   `json:"releaseEligible"`
			File            string `json:"file"`
			SHA256          string `json:"sha256"`
			Size            int    `json:"size"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("parse generated Flatpak release manifest: %v", err)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("generated Flatpak artifacts = %d, want one", len(manifest.Artifacts))
	}
	artifact := manifest.Artifacts[0]
	wantDigest := sha256.Sum256(artifactData)
	if artifact.PackageFormat != "flatpak" || artifact.Publication != "github-release-asset" || artifact.SigningStatus != "unsigned" || !artifact.ReleaseEligible {
		t.Errorf("generated Flatpak classification = %#v", artifact)
	}
	if artifact.File != artifactName || artifact.SHA256 != hex.EncodeToString(wantDigest[:]) || artifact.Size != len(artifactData) {
		t.Errorf("generated Flatpak integrity = %q/%q/%d", artifact.File, artifact.SHA256, artifact.Size)
	}
}
