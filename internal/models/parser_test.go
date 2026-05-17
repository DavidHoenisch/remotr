package models

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestParseState(t *testing.T) {
	lastUpdated := time.Date(2026, 5, 9, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		want    State
		wantErr bool
	}{
		{
			name:  "valid state",
			input: `{"Configurations":[{"Name":"base","Description":"base packages","LastUpdated":"2026-05-09T12:30:00Z","TargetDistros":["Ubuntu","Arch"]}]}`,
			want: State{Configurations: []Configuration{
				{
					Name:          "base",
					Description:   "base packages",
					LastUpdated:   lastUpdated,
					TargetDistros: []types.Distro{types.Ubuntu, types.Arch},
				},
			}},
		},
		{
			name:    "invalid json",
			input:   `{"Configurations":[`,
			wantErr: true,
		},
		{
			name:    "empty reader",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseState(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseState() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseState() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
