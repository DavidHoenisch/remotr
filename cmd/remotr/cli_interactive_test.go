package main

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func TestEndpointPickerOptions_sortsAndLabelsFleet(t *testing.T) {
	opts := endpointPickerOptions([]admin.Endpoint{
		{ID: "laptop-b", Fleet: "platform"},
		{ID: "laptop-a", Fleet: "engineering"},
	})
	if len(opts) != 2 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Value != "laptop-a" {
		t.Fatalf("first value = %q", opts[0].Value)
	}
	if opts[0].Key != "laptop-a  (engineering)" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
}
