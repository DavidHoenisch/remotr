package customapps

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestInstallBinary(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "bin", "tool")
	extractDir := t.TempDir()
	if err := extractZip(buildTestZip(t, dest), extractDir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(extractDir, apppackages.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := apppackages.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	a := New(models.Package{Name: "demo/tool", Version: "1.0.0", Present: true, PM: types.Remotr}, facts.Facts{Arch: types.X86}, executil.OSRunner{}, nil)
	if err := a.installBinary(extractDir, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("dest missing: %v", err)
	}
}

func buildTestZip(t *testing.T, dest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifest := "schemaVersion: 1\nname: demo/tool\nversion: \"1.0.0\"\ninstall:\n  mode: binary\n  files:\n    - src: bin/tool\n      dest: " + dest + "\n      mode: \"0755\"\n"
	w, err := zw.Create(apppackages.ManifestName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	w, err = zw.Create("bin/tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("#!/bin/sh\necho ok\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
