package models_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-OSQ-003, OS-OSQ-004, OS-OSQ-005, OS-OSQ-006: canonical kind: osquery
// runtime resource validates managed (private/global), existing, and disabled modes.
func TestParseState_canonicalOsquery(t *testing.T) {
	input := `schemaVersion: 1
configurations:
  - name: telemetry
    resources:
      - kind: osquery
        name: private-osquery
        lifecycle: present
        mode: managed
        visibility: private
        version: "5.14.1"
        sourceUrl: https://downloads.example.internal/osquery-5.14.1-linux-amd64.tar.gz
        sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
      - kind: osquery
        name: elastic-reuse
        lifecycle: present
        mode: existing
        allowedOwner: root
        binaryPaths:
          - /opt/Elastic/Agent/data/elastic-agent-*/components/osqueryd
          - /usr/bin/osqueryi
      - kind: osquery
        name: disabled-osquery
        lifecycle: present
        mode: disabled
`
	state, err := models.ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	got := state.Configurations[0].Osquery
	if len(got) != 3 {
		t.Fatalf("len(Osquery) = %d, want 3", len(got))
	}
	if got[0].EffectiveVisibility() != models.OsqueryVisibilityPrivate || got[0].EffectiveInstallDir() != models.DefaultOsqueryPrivateInstallDir {
		t.Fatalf("managed private defaults = %+v", got[0])
	}
	if got[1].Mode != models.OsqueryModeExisting || len(got[1].BinaryPaths) != 2 {
		t.Fatalf("existing mode = %+v", got[1])
	}
	if got[2].Mode != models.OsqueryModeDisabled {
		t.Fatalf("disabled mode = %+v", got[2])
	}
}

func TestOsqueryResource_Validate(t *testing.T) {
	const validSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	present := models.ResourceMeta{Lifecycle: models.LifecyclePresent}
	absent := models.ResourceMeta{Lifecycle: models.LifecycleAbsent}

	cases := []struct {
		name    string
		res     models.OsqueryResource
		wantErr bool
	}{
		{
			name: "managed private defaults",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "priv",
				Mode:         models.OsqueryModeManaged,
				Version:      "5.14.1",
				SourceURL:    "https://packages.example.test/osquery.tar.gz",
				SHA256:       validSHA,
			},
		},
		{
			name: "managed global custom symlink",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "glob",
				Mode:         models.OsqueryModeManaged,
				Visibility:   models.OsqueryVisibilityGlobal,
				InstallDir:   "/opt/custom-osquery",
				SymlinkPath:  "/usr/local/bin/osqueryi",
				Version:      "5.14.1",
				SourceURL:    "https://packages.example.test/osquery.tar.gz",
				SHA256:       validSHA,
			},
		},
		{
			name: "existing with anchored Elastic Agent pattern",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "elastic",
				Mode:         models.OsqueryModeExisting,
				BinaryPaths:  []string{"/opt/Elastic/Agent/data/elastic-agent-*/components/osqueryd"},
			},
		},
		{
			name: "absent managed private",
			res: models.OsqueryResource{
				ResourceMeta: absent,
				Name:         "priv",
				Mode:         models.OsqueryModeManaged,
			},
		},
		{
			name: "managed rejects non-https URL",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "http-bad",
				Mode:         models.OsqueryModeManaged,
				Version:      "5.14.1",
				SourceURL:    "http://packages.example.test/osquery.tar.gz",
				SHA256:       validSHA,
			},
			wantErr: true,
		},
		{
			name: "managed rejects short sha256",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "bad-sha",
				Mode:         models.OsqueryModeManaged,
				Version:      "5.14.1",
				SourceURL:    "https://packages.example.test/osquery.tar.gz",
				SHA256:       "deadbeef",
			},
			wantErr: true,
		},
		{
			name: "managed private rejects symlinkPath",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "priv-sym",
				Mode:         models.OsqueryModeManaged,
				Visibility:   models.OsqueryVisibilityPrivate,
				SymlinkPath:  "/usr/local/bin/osqueryi",
				Version:      "5.14.1",
				SourceURL:    "https://packages.example.test/osquery.tar.gz",
				SHA256:       validSHA,
			},
			wantErr: true,
		},
		{
			name: "managed rejects installDir inside /usr/bin",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "bad-dir",
				Mode:         models.OsqueryModeManaged,
				Visibility:   models.OsqueryVisibilityPrivate,
				InstallDir:   "/usr/bin",
				Version:      "5.14.1",
				SourceURL:    "https://packages.example.test/osquery.tar.gz",
				SHA256:       validSHA,
			},
			wantErr: true,
		},
		{
			name: "existing requires binaryPaths",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "no-paths",
				Mode:         models.OsqueryModeExisting,
			},
			wantErr: true,
		},
		{
			name: "existing rejects relative path",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "rel-path",
				Mode:         models.OsqueryModeExisting,
				BinaryPaths:  []string{"bin/osqueryi"},
			},
			wantErr: true,
		},
		{
			name: "existing rejects traversal",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "dotdot",
				Mode:         models.OsqueryModeExisting,
				BinaryPaths:  []string{"/opt/../bin/osqueryi"},
			},
			wantErr: true,
		},
		{
			name: "existing rejects double wildcard or wildcard in basename",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "wild-base",
				Mode:         models.OsqueryModeExisting,
				BinaryPaths:  []string{"/opt/Elastic/Agent/data/elastic-agent-*/components/*"},
			},
			wantErr: true,
		},
		{
			name: "disabled rejects install fields",
			res: models.OsqueryResource{
				ResourceMeta: present,
				Name:         "disabled-bad",
				Mode:         models.OsqueryModeDisabled,
				BinaryPaths:  []string{"/usr/bin/osqueryi"},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.res.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func FuzzParseCanonicalOsqueryPolicy(f *testing.F) {
	f.Add("managed", "private", "/usr/lib/remotr/osquery", "https://example.test/osquery.tar.gz", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	f.Add("existing", "", "/opt/Elastic/Agent/data/elastic-agent-*/components/osqueryd", "", "")
	f.Add("disabled", "", "", "", "")

	f.Fuzz(func(t *testing.T, mode, visibility, pathVal, urlVal, sha string) {
		if len(mode) > 32 || len(visibility) > 32 || len(pathVal) > 128 || len(urlVal) > 128 || len(sha) > 128 {
			return
		}
		doc := fmt.Sprintf(`schemaVersion: 1
configurations:
  - name: fuzz
    resources:
      - kind: osquery
        name: fuzz-osq
        lifecycle: present
        mode: %q
        visibility: %q
        installDir: %q
        version: "5.14.1"
        sourceUrl: %q
        sha256: %q
`, mode, visibility, pathVal, urlVal, sha)
		state, err := models.ParseState(strings.NewReader(doc))
		if err != nil {
			return
		}
		if len(state.Configurations) != 1 || len(state.Configurations[0].Osquery) != 1 {
			t.Fatalf("unexpected parsed osquery count: %#v", state)
		}
		got := state.Configurations[0].Osquery[0]
		if got.Mode == models.OsqueryModeManaged && !strings.HasPrefix(got.SourceURL, "https://") {
			t.Fatalf("accepted non-https sourceUrl %q", got.SourceURL)
		}
	})
}
