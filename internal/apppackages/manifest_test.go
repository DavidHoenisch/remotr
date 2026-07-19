package apppackages

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestValidateManifest_binary(t *testing.T) {
	m := Manifest{
		SchemaVersion: 1,
		Name:          "internal/mycli",
		Version:       "1.0.0",
		Install: InstallSpec{
			Mode: "binary",
			Files: []InstallFile{{
				Src:  "bin/mycli",
				Dest: "/usr/local/bin/mycli",
				Mode: "0755",
				Arch: types.X86,
			}},
		},
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
	if got := m.VersionFile(); got != "/var/lib/remotr/apps/internal-mycli/version" {
		t.Fatalf("VersionFile = %q", got)
	}
}

func TestParseManifestRejectsUnknownInstallMode(t *testing.T) {
	_, err := ParseManifest([]byte(`schemaVersion: 1
name: demo/tool
version: "1.0.0"
install:
  mode: package-manager
`))
	if err == nil || !strings.Contains(err.Error(), "invalid install.mode") {
		t.Fatalf("ParseManifest() error = %v, want invalid install mode", err)
	}
}

func TestValidateZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifest := `schemaVersion: 1
name: demo/tool
version: "0.1.0"
install:
  mode: binary
  files:
    - src: bin/tool
      dest: /usr/local/bin/tool
      mode: "0755"
`
	w, err := zw.Create(ManifestName)
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
	if _, err := w.Write([]byte("#!/bin/sh\necho tool\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	sum, err := ValidateZip(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if sum.Manifest.Name != "demo/tool" {
		t.Fatalf("name = %q", sum.Manifest.Name)
	}
	if sum.SHA256 == "" {
		t.Fatal("expected sha256")
	}
}
