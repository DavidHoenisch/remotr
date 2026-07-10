package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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
	if err := writeTarGz(tarPath, map[string][]byte{
		"../escape":    []byte("bad"),
		"remotr-agent": []byte("binary"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := extractAgentBinary(tarPath, dir); err == nil {
		t.Fatal("expected unsafe archive path error")
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
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
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
