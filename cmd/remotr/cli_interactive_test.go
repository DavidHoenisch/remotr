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

func TestAppPackagePickerOptions_sortsByNameAndVersion(t *testing.T) {
	opts := appPackagePickerOptions([]admin.AppPackage{
		{Name: "demo/cli", Version: "0.2.0"},
		{Name: "demo/cli", Version: "0.1.0"},
		{Name: "internal/mycli", Version: "1.0.0"},
	})
	if len(opts) != 3 {
		t.Fatalf("len = %d", len(opts))
	}
	if opts[0].Key != "demo/cli@0.1.0" {
		t.Fatalf("first key = %q", opts[0].Key)
	}
	name, version := parseAppPackagePickerValue(opts[2].Value)
	if name != "internal/mycli" || version != "1.0.0" {
		t.Fatalf("third value = %q %q", name, version)
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
