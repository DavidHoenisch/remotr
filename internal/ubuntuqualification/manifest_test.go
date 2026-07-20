package ubuntuqualification_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
	"gopkg.in/yaml.v3"
)

func TestValidateManifestCompleteness(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatalf("load repository qualification manifest: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ubuntuqualification.Manifest)
		want   string
	}{
		{
			name: "missing exact row",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				candidate.Rows = candidate.Rows[1:]
			},
			want: "missing exact qualification row",
		},
		{
			name: "duplicate exact row",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				candidate.Rows = append(candidate.Rows, candidate.Rows[0])
			},
			want: "duplicate exact qualification row",
		},
		{
			name: "broad family row",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				candidate.Rows[0].CapabilityID = "filesystem"
			},
			want: "broad family capability",
		},
		{
			name: "stale contract revision",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				candidate.Rows[0].ContractRevision = "file-v999"
			},
			want: "stale contract revision",
		},
		{
			name: "unknown backend row",
			mutate: func(candidate *ubuntuqualification.Manifest) {
				candidate.Rows[0].Backend = "future-filesystem"
			},
			want: "unknown qualification row",
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

func TestDecodeManifestBoundaries(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"empty":         nil,
		"unknown field": []byte("version: 1\nunknown: true\n"),
		"oversized":     bytes.Repeat([]byte{'x'}, 4<<20+1),
		"duplicate key": []byte("version: 1\nversion: 1\n"),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ubuntuqualification.Decode(input, registry); err == nil {
				t.Fatal("Decode() accepted malformed or unbounded manifest")
			}
		})
	}
}

func FuzzDecodeManifest(f *testing.F) {
	valid, err := os.ReadFile(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte("version: 1\nrows: []\n"))
	f.Add([]byte("not: [valid"))

	registry, err := resourceregistry.NewDefault()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		manifest, err := ubuntuqualification.Decode(input, registry)
		if err != nil {
			return
		}
		if manifest.Version != 1 || manifest.Platform.Distribution != "ubuntu" ||
			manifest.Platform.Release != "24.04" || manifest.Platform.Architecture != "amd64" {
			t.Fatalf("Decode() accepted an inexact qualification platform: %+v", manifest.Platform)
		}
		encoded, err := yaml.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ubuntuqualification.Decode(encoded, registry); err != nil {
			t.Fatalf("valid manifest did not round trip: %v", err)
		}
	})
}
