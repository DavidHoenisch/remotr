package aisetup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFromEmbed(t *testing.T) {
	dir := t.TempDir()
	target := Target{
		Agent:      AgentClaude,
		Scope:      ScopeProject,
		InstallDir: filepath.Join(dir, "skills", "remotr-agent"),
	}

	// Use on-disk fixture mirroring repo layout.
	srcRoot := filepath.Join("..", "..", "ai", "remotr-agent")
	if _, err := os.Stat(srcRoot); err != nil {
		// test-exception: EXC-005
		t.Skip("ai/remotr-agent fixture not available from test cwd")
	}

	manifest, err := Install(InstallOptions{
		Target:      target,
		Source:      os.DirFS(srcRoot),
		SourceRoot:  ".",
		SourceLabel: "test",
		Force:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BundleVersion == "" {
		t.Fatal("expected bundle version")
	}
	if _, err := os.Stat(filepath.Join(target.InstallDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target.InstallDir, manifestName)); err != nil {
		t.Fatal(err)
	}
}

func TestExtractBundleToTemp(t *testing.T) {
	archive, err := buildFixtureArchive()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := extractBundleToTemp(archive, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "reference", "commands.md")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractBundleToTempRejectsTraversal(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}

	archive, err := buildArchive(map[string]string{
		"remotr-1.2.3/ai/remotr-agent/../victim": "owned",
		"remotr-1.2.3/ai/remotr-agent/SKILL.md":  "# skill",
	})
	if err != nil {
		t.Fatal(err)
	}

	dir, err := extractBundleToTemp(archive, "v1.2.3")
	if err == nil {
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		t.Fatal("expected traversal path to be rejected")
	}
	content, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "safe" {
		t.Fatalf("victim file was overwritten: %q", content)
	}
}

func TestResolveTargetPiUser(t *testing.T) {
	t.Setenv("HOME", "/tmp/example-home")
	target, err := ResolveTarget(AgentPi, ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/example-home", ".pi", "agent", "skills", "remotr-agent")
	if target.InstallDir != want {
		t.Fatalf("got %q want %q", target.InstallDir, want)
	}
}

func TestResolveTargetPaths(t *testing.T) {
	t.Setenv("HOME", "/tmp/example-home")
	target, err := ResolveTarget(AgentClaude, ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/example-home", ".claude", "skills", "remotr-agent")
	if target.InstallDir != want {
		t.Fatalf("got %q want %q", target.InstallDir, want)
	}
}

func buildFixtureArchive() ([]byte, error) {
	return buildArchive(map[string]string{
		"remotr-1.2.3/ai/remotr-agent/SKILL.md":              "# skill",
		"remotr-1.2.3/ai/remotr-agent/VERSION":               "1.2.3",
		"remotr-1.2.3/ai/remotr-agent/reference/commands.md": "# commands",
	})
}

func buildArchive(files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
