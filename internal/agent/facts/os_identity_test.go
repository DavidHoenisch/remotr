package facts_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestReadIdentityPreservesExactUbuntuAndFamilyFacts(t *testing.T) {
	t.Parallel()

	source := &identitySource{
		files: map[string][]byte{
			"/etc/os-release":     []byte("ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n"),
			"/usr/lib/os-release": []byte("ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n"),
		},
		vendor: []byte("Ubuntu\n"),
	}

	got, err := facts.ReadIdentity(source)
	if err != nil {
		t.Fatalf("ReadIdentity() error = %v", err)
	}
	if got.Distro != types.Ubuntu || got.DistroFamily != facts.DistroFamilyDebian || got.DistroVersion != "24.04" {
		t.Fatalf("portable identity = %#v", got)
	}
	if got.OSID != "ubuntu" || !slices.Equal(got.OSIDLike, []string{"debian"}) || !got.OSReleaseConsistent || got.DistroVendor != "Ubuntu" {
		t.Fatalf("exact identity = %#v", got)
	}
	if !slices.Equal(source.argv, []string{"/usr/bin/dpkg-vendor", "--query", "Vendor"}) {
		t.Fatalf("vendor argv = %q", source.argv)
	}
}

func TestReadIdentityKeepsPopOSInDebianFamilyWithoutExactUbuntuIdentity(t *testing.T) {
	t.Parallel()

	source := &identitySource{
		files: map[string][]byte{
			"/etc/os-release":     []byte("ID=pop\nVERSION_ID=\"24.04\"\nID_LIKE=\"ubuntu debian\"\n"),
			"/usr/lib/os-release": []byte("ID=pop\nVERSION_ID=\"24.04\"\nID_LIKE=\"ubuntu debian\"\n"),
		},
		vendor: []byte("Ubuntu\n"),
	}

	got, err := facts.ReadIdentity(source)
	if err != nil {
		t.Fatalf("ReadIdentity() error = %v", err)
	}
	if got.Distro != types.Debian || got.DistroFamily != facts.DistroFamilyDebian || got.OSID != "pop" {
		t.Fatalf("derivative family identity = %#v", got)
	}
	if got.ExactUbuntu() {
		t.Fatalf("Pop!_OS facts were accepted as exact Ubuntu: %#v", got)
	}
}

func TestReadIdentityRejectsSecondUbuntuDerivativeWithBoundedReason(t *testing.T) {
	t.Parallel()

	source := &identitySource{
		files: map[string][]byte{
			"/etc/os-release":     []byte("ID=linuxmint\nVERSION_ID=22\nID_LIKE=\"ubuntu debian\"\n"),
			"/usr/lib/os-release": []byte("ID=linuxmint\nVERSION_ID=22\nID_LIKE=\"ubuntu debian\"\n"),
		},
		vendor: []byte("Ubuntu\n"),
	}

	got, err := facts.ReadIdentity(source)
	if err != nil {
		t.Fatalf("ReadIdentity() error = %v", err)
	}
	if got.Distro != types.Debian || got.DistroFamily != facts.DistroFamilyDebian {
		t.Fatalf("Linux Mint family identity = %#v", got)
	}
	if reason := got.ExactUbuntuReason(); reason != `exact distribution ID "linuxmint" is not ubuntu` {
		t.Fatalf("ExactUbuntuReason() = %q", reason)
	}
}

type identitySource struct {
	files  map[string][]byte
	vendor []byte
	argv   []string
}

func (source *identitySource) ReadFile(path string) ([]byte, error) {
	data, ok := source.files[path]
	if !ok {
		return nil, fmt.Errorf("missing fixture %s", path)
	}
	return data, nil
}

func (source *identitySource) Run(path string, args ...string) ([]byte, error) {
	source.argv = append([]string{path}, args...)
	return source.vendor, nil
}
