package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndPresent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "remotr")
	if Present(dir) {
		t.Fatal("expected empty dir to be absent")
	}

	const (
		id   = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
		cert = "-----BEGIN CERTIFICATE-----\ncert\n-----END CERTIFICATE-----\n"
		key  = "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n"
		ca   = "-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----\n"
	)

	if err := Save(dir, id, cert, key, ca); err != nil {
		t.Fatal(err)
	}
	if !Present(dir) {
		t.Fatal("expected credentials to be present")
	}

	st, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.EndpointID != id {
		t.Fatalf("endpoint id = %q", st.EndpointID)
	}

	p, err := Layout(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.Cert, p.Key, p.CA, p.Meta} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o, want 0600", path, info.Mode().Perm())
		}
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o, want 0700", dirInfo.Mode().Perm())
	}
}

func TestSave_rejectsIncompleteMaterial(t *testing.T) {
	err := Save(t.TempDir(), "", "cert", "key", "ca")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLayout_joinsUnderDir(t *testing.T) {
	dir := filepath.Clean("/var/lib/remotr")
	p, err := Layout(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Cert != filepath.Join(dir, certName) {
		t.Fatalf("cert path = %q", p.Cert)
	}
}
