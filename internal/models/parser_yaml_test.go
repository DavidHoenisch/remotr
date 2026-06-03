package models

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestParseStateYAML(t *testing.T) {
	lastUpdated := time.Date(2026, 5, 9, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		want    State
		wantErr bool
	}{
		{
			name: "valid fleet desired yaml",
			input: `configurations:
  - name: base
    description: base packages
    lastUpdated: "2026-05-09T12:30:00Z"
    targetDistros:
      - Ubuntu
      - Arch
    packages:
      - name: nmap
        present: true
        packageManager: apt
`,
			want: State{Configurations: []Configuration{
				{
					Name:          "base",
					Description:   "base packages",
					LastUpdated:   lastUpdated,
					TargetDistros: []types.Distro{types.Ubuntu, types.Arch},
					Packages: []Package{
						{
							Name:    "nmap",
							Present: true,
							PM:      types.Apt,
						},
					},
				},
			}},
		},
		{
			name:    "invalid yaml",
			input:   "configurations:\n  - name: [\n",
			wantErr: true,
		},
		{
			name: "downloads resource",
			input: `configurations:
  - name: base
    downloads:
      - name: agent-bin
        url: https://example.com/remotr-agent
        dest: /usr/local/bin/remotr-agent
        mode: [493]
        checksum: sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789
`,
			want: State{Configurations: []Configuration{{
				Name: "base",
				Downloads: []DownloadResource{{
					Name:     "agent-bin",
					URL:      "https://example.com/remotr-agent",
					Dest:     "/usr/local/bin/remotr-agent",
					Mode:     []int{493},
					Checksum: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				}},
			}}},
		},
		{
			name:    "empty reader",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseState(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseState() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseState() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
