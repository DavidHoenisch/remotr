package main

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func TestEndpointPickerOptions_sortsAndLabelsFleet(t *testing.T) {
	opts := endpointPickerOptions([]admin.Endpoint{
		{ID: "laptop-b", Fleet: "platform", Usernames: []string{"bob"}},
		{ID: "laptop-a", Fleet: "engineering", Usernames: []string{"alice"}},
	})
	if len(opts) != 2 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Value != "laptop-a" {
		t.Fatalf("first value = %q", opts[0].Value)
	}
	if opts[0].Key != "laptop-a  (engineering · alice)" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
}

func TestEndpointLabelKeyOptions(t *testing.T) {
	opts := endpointLabelKeyOptions(map[string]string{
		"site": "berlin",
		"role": "web",
	})
	if len(opts) != 2 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Value != "role" {
		t.Fatalf("first value = %q", opts[0].Value)
	}
	if opts[0].Key != "role=web" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
}
