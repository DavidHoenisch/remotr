package cliupgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetFileName(t *testing.T) {
	if got := assetFileName("v0.2.1", "linux", "amd64"); got != "remotr_0.2.1_linux_amd64.tar.gz" {
		t.Fatalf("got %q", got)
	}
	if got := assetFileName("0.2.1", "windows", "amd64"); got != "remotr_0.2.1_windows_amd64.zip" {
		t.Fatalf("got %q", got)
	}
}

func TestInstallBinary_replacesDestination(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "remotr")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installBinary([]byte("new"), dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("dest = %q", got)
	}
}

func TestExtractTarGzBinary(t *testing.T) {
	goos, _, err := currentPlatform()
	if err != nil {
		t.Skip(err)
	}
	name := binaryName(goos)
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("bin")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractBinary(buf.Bytes(), goos, binaryName(goos))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bin" {
		t.Fatalf("got %q", got)
	}
}

func TestRun_upgradeFromGitHubRelease(t *testing.T) {
	const tag = "v9.9.9"
	payload, err := json.Marshal(releaseInfo{TagName: tag})
	if err != nil {
		t.Fatal(err)
	}

	archive, err := buildTarGz("remotr", []byte("release-binary"))
	if err != nil {
		t.Fatal(err)
	}

	goos, goarch, err := currentPlatform()
	if err != nil {
		t.Skip(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/test/repo/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	})
	asset := assetFileName(tag, goos, goarch)
	checksumPath := "/test/repo/releases/download/" + tag + "/remotr_checksums.txt"
	mux.HandleFunc(checksumPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(checksumLine(asset, archive)))
	})
	assetPath := "/test/repo/releases/download/" + tag + "/" + asset
	mux.HandleFunc(assetPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "remotr")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), Options{
		CurrentVersion: "0.1.0",
		TargetVersion:  "",
		GitHubRepo:     "test/repo",
		InstallPath:    dest,
		HTTPClient:     client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Installed {
		t.Fatal("expected installed")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "release-binary" {
		t.Fatalf("dest = %q", got)
	}
}

func TestRun_rejectsChecksumMismatch(t *testing.T) {
	const tag = "v9.9.9"
	payload, err := json.Marshal(releaseInfo{TagName: tag})
	if err != nil {
		t.Fatal(err)
	}

	archive, err := buildTarGz("remotr", []byte("release-binary"))
	if err != nil {
		t.Fatal(err)
	}

	goos, goarch, err := currentPlatform()
	if err != nil {
		t.Skip(err)
	}
	asset := assetFileName(tag, goos, goarch)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/test/repo/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/test/repo/releases/download/"+tag+"/remotr_checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("0", sha256.Size*2) + "  " + asset + "\n"))
	})
	mux.HandleFunc("/test/repo/releases/download/"+tag+"/"+asset, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "remotr")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = Run(context.Background(), Options{
		CurrentVersion: "0.1.0",
		GitHubRepo:     "test/repo",
		InstallPath:    dest,
		HTTPClient:     client,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("dest = %q", got)
	}
}

func TestRun_checkOnly(t *testing.T) {
	payload, err := json.Marshal(releaseInfo{TagName: "v2.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/test/repo/releases/latest" {
			_, _ = w.Write(payload)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	res, err := Run(context.Background(), Options{
		CurrentVersion: "0.1.0",
		GitHubRepo:     "test/repo",
		CheckOnly:      true,
		HTTPClient:     client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.UpToDate {
		t.Fatal("expected update available")
	}
	if res.Target != "v2.0.0" {
		t.Fatalf("target = %q", res.Target)
	}
}

func checksumLine(asset string, data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + "  " + asset + "\n"
}

func buildTarGz(name string, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(data); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
