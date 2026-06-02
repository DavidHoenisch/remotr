package models

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestParseState_rejectsInvalidYAML(t *testing.T) {
	_, err := ParseState(strings.NewReader("configurations:\n  - name: [\n"))
	if err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestParseState_rejectsEmpty(t *testing.T) {
	_, err := ParseState(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseState_parsesMinimalConfiguration(t *testing.T) {
	lastUpdated := time.Date(2026, 5, 9, 12, 30, 0, 0, time.UTC)
	input := `configurations:
  - name: base
    description: base packages
    lastUpdated: "2026-05-09T12:30:00Z"
    targetDistros:
      - Ubuntu
      - Arch
`
	want := State{Configurations: []Configuration{
		{
			Name:          "base",
			Description:   "base packages",
			LastUpdated:   lastUpdated,
			TargetDistros: []types.Distro{types.Ubuntu, types.Arch},
		},
	}}

	got, err := ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseState() = %#v, want %#v", got, want)
	}
}
