package facts_test

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
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

// OS-UPM-003: minimal Ubuntu images establish the same exact vendor identity
// from dpkg's canonical origins record when dpkg-vendor is not installed.
func TestReadIdentityUsesDpkgOriginWhenVendorCommandIsUnavailable(t *testing.T) {
	t.Parallel()

	source := &identitySource{
		files: map[string][]byte{
			"/etc/os-release":           []byte("ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n"),
			"/usr/lib/os-release":       []byte("ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n"),
			"/etc/dpkg/origins/default": []byte("Vendor: Ubuntu\nVendor-URL: https://www.ubuntu.com/\n"),
		},
		runErr: errors.New("dpkg-vendor unavailable"),
	}

	got, err := facts.ReadIdentity(source)
	if err != nil {
		t.Fatalf("ReadIdentity() error = %v", err)
	}
	if !got.ExactUbuntu() || got.DistroVendor != "Ubuntu" {
		t.Fatalf("minimal Ubuntu identity = %#v", got)
	}
}

func TestReadIdentityRejectsAmbiguousDpkgOriginFallback(t *testing.T) {
	t.Parallel()

	tests := map[string][]byte{
		"missing vendor":   []byte("Vendor-URL: https://www.ubuntu.com/\n"),
		"duplicate vendor": []byte("Vendor: Ubuntu\nVendor: Debian\n"),
		"oversized origin": bytes.Repeat([]byte("x"), 4097),
	}
	for name, origin := range tests {
		origin := origin
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := &identitySource{
				files: map[string][]byte{
					"/etc/os-release":           []byte("ID=ubuntu\nVERSION_ID=24.04\n"),
					"/etc/dpkg/origins/default": origin,
				},
				runErr: errors.New("dpkg-vendor unavailable"),
			}
			if _, err := facts.ReadIdentity(source); err == nil || len(err.Error()) > 256 {
				t.Fatalf("ReadIdentity() error = %v, want bounded rejection", err)
			}
		})
	}
}

// OS-LPC-011 and OS-LPC-028. Public seam: production operating-system fact
// discovery used to generate the authenticated capability document.
func TestReadIdentityPreservesExactPopOSWithDebianFamilyWithoutExactUbuntuIdentity(t *testing.T) {
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
	if got.Distro != types.PopOS || got.DistroFamily != facts.DistroFamilyDebian || got.OSID != "pop" {
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

func TestReadIdentityFailsClosedWhenOSReleaseSourcesConflict(t *testing.T) {
	t.Parallel()

	source := &identitySource{
		files: map[string][]byte{
			"/etc/os-release":     []byte("ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n"),
			"/usr/lib/os-release": []byte("ID=ubuntu\nVERSION_ID=\"22.04\"\nID_LIKE=debian\n"),
		},
		vendor: []byte("Ubuntu\n"),
	}

	got, err := facts.ReadIdentity(source)
	if err != nil {
		t.Fatalf("ReadIdentity() error = %v", err)
	}
	if got.OSReleaseSourceCount != 2 || got.OSReleaseConsistent || got.ExactUbuntu() {
		t.Fatalf("conflicting exact identity = %#v", got)
	}
	if reason := got.ExactUbuntuReason(); reason != "operating-system release sources disagree" {
		t.Fatalf("ExactUbuntuReason() = %q", reason)
	}
}

func TestReadIdentityClassifiesMalformedAndDuplicateOSRelease(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"duplicate exact ID": "ID=ubuntu\nID=debian\nVERSION_ID=24.04\n",
		"malformed release":  "ID=ubuntu\nVERSION_ID=\"24.04\n",
	}
	for name, content := range tests {
		content := content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := &identitySource{files: map[string][]byte{
				"/etc/os-release":     []byte(content),
				"/usr/lib/os-release": []byte("ID=ubuntu\nVERSION_ID=24.04\n"),
			}}

			_, err := facts.ReadIdentity(source)
			if !errors.Is(err, facts.ErrAmbiguousOSRelease) {
				t.Fatalf("ReadIdentity() error = %v, want ErrAmbiguousOSRelease", err)
			}
		})
	}
}

func TestReadIdentityExactUbuntuBoundaries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		etc       string
		usr       string
		vendor    string
		wantExact bool
		wantCount int
		wantErr   bool
	}{
		"single source is sufficient":  {etc: "ID=ubuntu\nVERSION_ID=24.04\n", vendor: "Ubuntu\n", wantExact: true, wantCount: 1},
		"vendor comparison is exact":   {etc: "ID=ubuntu\nVERSION_ID=24.04\n", usr: "ID=ubuntu\nVERSION_ID=24.04\n", vendor: "ubuntu\n", wantCount: 2},
		"duplicate ID_LIKE is invalid": {etc: "ID=ubuntu\nVERSION_ID=24.04\nID_LIKE=debian\nID_LIKE=ubuntu\n", wantErr: true},
		"missing release is invalid":   {etc: "ID=ubuntu\n", wantErr: true},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := map[string][]byte{"/etc/os-release": []byte(test.etc)}
			if test.usr != "" {
				files["/usr/lib/os-release"] = []byte(test.usr)
			}
			got, err := facts.ReadIdentity(&identitySource{files: files, vendor: []byte(test.vendor)})
			if test.wantErr {
				if !errors.Is(err, facts.ErrAmbiguousOSRelease) {
					t.Fatalf("ReadIdentity() error = %v, want ambiguity", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadIdentity() error = %v", err)
			}
			if got.ExactUbuntu() != test.wantExact || got.OSReleaseSourceCount != test.wantCount {
				t.Fatalf("identity = %#v, want exact=%t sources=%d", got, test.wantExact, test.wantCount)
			}
		})
	}
}

func FuzzReadIdentityBoundedOSRelease(f *testing.F) {
	f.Add([]byte("ID=ubuntu\nVERSION_ID=24.04\nID_LIKE=debian\n"))
	f.Add([]byte("ID=pop\nVERSION_ID=22.04\nID_LIKE=\"ubuntu debian\"\n"))
	f.Add([]byte("ID=" + strings.Repeat("x", 600) + "\nVERSION_ID=24.04\n"))
	f.Add([]byte("ID=ubuntu\nID=debian\nVERSION_ID=24.04\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			data = data[:4096]
		}
		source := &identitySource{files: map[string][]byte{
			"/etc/os-release":     data,
			"/usr/lib/os-release": data,
		}, vendor: []byte("Ubuntu\n")}
		got, err := facts.ReadIdentity(source)
		if err != nil {
			if len(err.Error()) > 512 {
				t.Fatalf("ReadIdentity() error is unbounded: %d bytes", len(err.Error()))
			}
			return
		}
		if got.OSReleaseSourceCount != 2 || !got.OSReleaseConsistent {
			t.Fatalf("same-source fixture produced inconsistent facts: %#v", got)
		}
	})
}

type identitySource struct {
	files  map[string][]byte
	vendor []byte
	argv   []string
	runErr error
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
	return source.vendor, source.runErr
}
