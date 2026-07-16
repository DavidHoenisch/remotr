package desktopparity

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestValidateRejectsUnmappedCommand(t *testing.T) {
	inventory := Inventory{Entries: []Entry{{
		Command:              "remotr endpoint list",
		Status:               "planned",
		TargetFeatureRelease: "v1-fleet-operations",
		VerificationIDs:      []string{"OS-DFV-006"},
	}}}

	issues := Validate(
		[]string{"remotr endpoint list", "remotr fleet list"},
		inventory,
	)

	want := []string{"unmapped non-hidden CLI command: remotr fleet list"}
	if !slices.Equal(issues, want) {
		t.Fatalf("Validate() issues = %q, want %q", issues, want)
	}
}

func TestValidateRejectsImplementedCommandWithoutEvidence(t *testing.T) {
	inventory := Inventory{Entries: []Entry{{
		Command:              "remotr endpoint list",
		Status:               "implemented",
		TargetFeatureRelease: "v1-fleet-operations",
		VerificationIDs:      []string{"OS-DFV-006"},
	}}}

	issues := Validate([]string{"remotr endpoint list"}, inventory)

	want := []string{"implemented command has no passing selectors: remotr endpoint list"}
	if !slices.Equal(issues, want) {
		t.Fatalf("Validate() issues = %q, want %q", issues, want)
	}
}

func TestValidateRejectsPlannedCommandWithoutTarget(t *testing.T) {
	inventory := Inventory{Entries: []Entry{{
		Command:         "remotr endpoint list",
		Status:          "planned",
		VerificationIDs: []string{"OS-DFV-006"},
	}}}

	issues := Validate([]string{"remotr endpoint list"}, inventory)

	want := []string{"planned command has no target feature release: remotr endpoint list"}
	if !slices.Equal(issues, want) {
		t.Fatalf("Validate() issues = %q, want %q", issues, want)
	}
}

func TestValidateRejectsNotApplicableCommandWithoutReviewedReason(t *testing.T) {
	inventory := Inventory{Entries: []Entry{{
		Command:              "remotr",
		Status:               "not_applicable",
		TargetFeatureRelease: "interface-review",
		VerificationIDs:      []string{"OS-DOA-019"},
	}}}

	issues := Validate([]string{"remotr"}, inventory)

	want := []string{"not-applicable command has no reviewed reason: remotr"}
	if !slices.Equal(issues, want) {
		t.Fatalf("Validate() issues = %q, want %q", issues, want)
	}
}

func TestValidateRejectsCommandWithoutVerificationID(t *testing.T) {
	inventory := Inventory{Entries: []Entry{{
		Command:              "remotr endpoint list",
		Status:               "planned",
		TargetFeatureRelease: "v1-fleet-operations",
	}}}

	issues := Validate([]string{"remotr endpoint list"}, inventory)

	want := []string{"command has no OpenSpec verification IDs: remotr endpoint list"}
	if !slices.Equal(issues, want) {
		t.Fatalf("Validate() issues = %q, want %q", issues, want)
	}
}

func TestValidateRejectsUnknownDisposition(t *testing.T) {
	inventory := Inventory{Entries: []Entry{{
		Command:              "remotr endpoint list",
		Status:               "done-ish",
		TargetFeatureRelease: "v1-fleet-operations",
		VerificationIDs:      []string{"OS-DFV-006"},
	}}}

	issues := Validate([]string{"remotr endpoint list"}, inventory)

	want := []string{"command has invalid desktop disposition done-ish: remotr endpoint list"}
	if !slices.Equal(issues, want) {
		t.Fatalf("Validate() issues = %q, want %q", issues, want)
	}
}

func TestParseRejectsOversizedInventory(t *testing.T) {
	_, err := Parse(bytes.Repeat([]byte{' '}, MaxInventoryBytes+1))
	if err == nil || err.Error() != "inventory exceeds 1048576 bytes" {
		t.Fatalf("Parse() error = %v, want inventory size error", err)
	}
}

func FuzzParseInventoryRoundTrip(f *testing.F) {
	f.Add([]byte(`{"entries":[]}`))
	f.Add([]byte(`{"entries":[{"command":"remotr","status":"planned"}]}`))
	f.Add([]byte(`{"entries":`))

	f.Fuzz(func(t *testing.T, data []byte) {
		inventory, err := Parse(data)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(inventory)
		if err != nil {
			t.Fatalf("marshal accepted inventory: %v", err)
		}
		roundTrip, err := Parse(encoded)
		if err != nil {
			t.Fatalf("parse marshaled inventory: %v", err)
		}
		if !reflect.DeepEqual(roundTrip, inventory) {
			t.Fatalf("round-trip inventory changed: got %#v, want %#v", roundTrip, inventory)
		}
	})
}
