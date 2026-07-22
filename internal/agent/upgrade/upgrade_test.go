package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type releaseDownloadRunner struct {
	t              *testing.T
	archive        []byte
	downloadedURLs []string
}

func (r *releaseDownloadRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.t.Helper()
	if name == "systemctl" {
		return nil, nil, nil
	}
	if name != "curl" || len(args) != 4 || args[0] != "-fsSL" || args[1] != "-o" {
		return nil, nil, fmt.Errorf("unexpected argv: %s %v", name, args)
	}
	destination, source := args[2], args[3]
	r.downloadedURLs = append(r.downloadedURLs, source)
	if strings.HasSuffix(source, "/checksums.txt") {
		asset := "remotr-agent_0.6.8_linux_amd64.tar.gz"
		sum := sha256.Sum256(r.archive)
		return nil, nil, os.WriteFile(destination, []byte(fmt.Sprintf("%x  %s\n", sum, asset)), 0o600)
	}
	if strings.HasSuffix(source, "/"+filepath.Base(destination)) {
		return nil, nil, os.WriteFile(destination, r.archive, 0o600)
	}
	return nil, nil, fmt.Errorf("unexpected release URL %s", source)
}

func TestApplyUsesPublishedChecksumManifestAndExactArgv(t *testing.T) {
	temp := t.TempDir()
	archivePath := filepath.Join(temp, "fixture.tar.gz")
	if err := writeTarGz(archivePath, map[string][]byte{"remotr-agent": []byte("new-agent")}); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	runner := &releaseDownloadRunner{t: t, archive: archive}
	if err := Apply(Instruction{Version: "v0.6.8", GitHubRepo: "DavidHoenisch/remotr"}, Options{BinDir: temp, Exec: runner}); err != nil {
		t.Fatal(err)
	}
	if len(runner.downloadedURLs) != 2 || runner.downloadedURLs[1] != "https://github.com/DavidHoenisch/remotr/releases/download/v0.6.8/checksums.txt" {
		t.Fatalf("release downloads = %v", runner.downloadedURLs)
	}
	installed, err := os.ReadFile(filepath.Join(temp, "remotr-agent"))
	if err != nil || string(installed) != "new-agent" {
		t.Fatalf("installed agent = %q err=%v", installed, err)
	}
}

func TestInstallBinary_replacesDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dest := filepath.Join(dir, "remotr-agent")
	if err := os.WriteFile(src, []byte("new-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installBinary(src, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("dest = %q", got)
	}
	if _, err := os.Stat(dest + ".new"); !os.IsNotExist(err) {
		t.Fatalf("staging file left behind: %v", err)
	}
}

func TestReadExpectedSHA256AndVerify(t *testing.T) {
	dir := t.TempDir()
	asset := "remotr-agent_1.2.3_linux_amd64.tar.gz"
	tarPath := filepath.Join(dir, asset)
	data := []byte("archive")
	if err := os.WriteFile(tarPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	sumPath := tarPath + ".sha256"
	if err := os.WriteFile(sumPath, []byte(fmt.Sprintf("%x  %s\n", sum, asset)), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := readExpectedSHA256(sumPath, asset)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(tarPath, expected); err != nil {
		t.Fatal(err)
	}
}

func TestVerifySHA256RejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestExtractAgentBinaryRejectsUnsafePath(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "archive.tar.gz")
	if err := writeTarGzEntries(tarPath, []tarEntry{
		{name: "remotr-agent", data: []byte("binary")},
		{name: "../escape", data: []byte("bad")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := extractAgentBinary(tarPath, dir); err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("unsafe path after expected binary error = %v, want unsafe archive path rejection", err)
	}
}

func TestExtractAgentBinaryWritesOnlyExpectedBinary(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "archive.tar.gz")
	if err := writeTarGz(tarPath, map[string][]byte{"remotr-agent": []byte("binary")}); err != nil {
		t.Fatal(err)
	}
	if err := extractAgentBinary(tarPath, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "remotr-agent"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("binary = %q", got)
	}
}

func writeTarGz(path string, files map[string][]byte) error {
	entries := make([]tarEntry, 0, len(files))
	for name, data := range files {
		entries = append(entries, tarEntry{name: name, data: data})
	}
	return writeTarGzEntries(path, entries)
}

type tarEntry struct {
	name string
	data []byte
}

func writeTarGzEntries(path string, entries []tarEntry) error {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o755, Size: int64(len(entry.data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := tw.Write(entry.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
