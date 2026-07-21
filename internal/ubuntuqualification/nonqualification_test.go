package ubuntuqualification_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

func TestUnadvertisedContractsHaveExplicitReasons(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatalf("load repository qualification manifest: %v", err)
	}

	wantRows := map[string]bool{
		"command/argv": true, "bootstrap/one-shot": true, "agentInstall/binary-install": true,
		"systemd/systemd-legacy": true, "systemdUser/systemd-user-legacy": true,
		"service/openrc": true, "service/sysv": true,
		"networkProfile/netplan": true, "networkProfile/systemd-networkd": true,
		"firewall/firewalld-enforcement": true,
	}
	for _, row := range manifest.Rows {
		key := row.CapabilityID + "/" + row.Backend
		if !wantRows[key] {
			continue
		}
		if row.Disposition != "unadvertised" || strings.TrimSpace(row.Reason) == "" {
			t.Errorf("%s = disposition %q reason %q, want explicit unadvertised reason", key, row.Disposition, row.Reason)
		}
		delete(wantRows, key)
	}
	for key := range wantRows {
		t.Errorf("missing explicit non-qualification row %s", key)
	}

	wantRoadmap := []string{
		"UHF-000", "UHF-001", "UHF-002",
		"UHF-100", "UHF-101", "UHF-102", "UHF-103", "UHF-104", "UHF-105", "UHF-106", "UHF-107", "UHF-108",
		"UHF-200", "UHF-201", "UHF-202", "UHF-203", "UHF-204", "UHF-205", "UHF-206", "UHF-207", "UHF-208",
		"UHF-300", "UHF-301", "UHF-302", "UHF-303", "UHF-304",
	}
	if len(manifest.FutureRoadmap) != len(wantRoadmap) {
		t.Fatalf("future roadmap entries = %d, want %d", len(manifest.FutureRoadmap), len(wantRoadmap))
	}
	for index, want := range wantRoadmap {
		entry := manifest.FutureRoadmap[index]
		if entry.ID != want || strings.TrimSpace(entry.Title) == "" || strings.TrimSpace(entry.Reason) == "" {
			t.Errorf("future roadmap entry %d = %+v, want %s with title and reason", index, entry, want)
		}
	}
	roadmapReasons := make(map[string]string, len(manifest.FutureRoadmap))
	for _, entry := range manifest.FutureRoadmap {
		roadmapReasons[entry.ID] = strings.ToLower(entry.Reason)
	}
	for _, phrase := range []string{"edge", "other browsers", "user scope", "firefox recommended", "unknown policy names", "types", "levels"} {
		if !strings.Contains(roadmapReasons["UHF-104"], phrase) {
			t.Errorf("UHF-104 reason %q is missing %q", roadmapReasons["UHF-104"], phrase)
		}
	}
	for _, phrase := range []string{"authoritative", "default application", "cleanup", "not safely portable"} {
		if !strings.Contains(roadmapReasons["UHF-108"], phrase) {
			t.Errorf("UHF-108 reason %q is missing %q", roadmapReasons["UHF-108"], phrase)
		}
	}

	tests := []struct {
		name   string
		mutate func(*ubuntuqualification.Manifest)
		want   string
	}{
		{
			name: "missing explicit row reason",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				for index := range candidate.Rows {
					if candidate.Rows[index].CapabilityID == "command" {
						candidate.Rows[index].Reason = ""
					}
				}
			},
			want: "explicit non-qualification reason",
		},
		{
			name: "missing roadmap item",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				candidate.FutureRoadmap = candidate.FutureRoadmap[1:]
			},
			want: "missing future-roadmap item UHF-000",
		},
		{
			name: "missing roadmap reason",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				candidate.FutureRoadmap[0].Reason = ""
			},
			want: "future-roadmap item UHF-000 requires a title and reason",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := manifest.Clone()
			test.mutate(&candidate)
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}
