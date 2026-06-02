package identity

import (
	"strings"
	"testing"
)

func TestSanitizeHostname(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"stllr-remotr", "stllr-remotr"},
		{"STLLR-REMOTR.example.com", "stllr-remotr-example-com"},
		{"  ", "host"},
		{"a..b", "a-b"},
	}
	for _, tc := range tests {
		if got := SanitizeHostname(tc.in); got != tc.want {
			t.Errorf("SanitizeHostname(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveEndpointID_explicit(t *testing.T) {
	got, err := ResolveEndpointID("my-laptop-a1b2c3d4")
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-laptop-a1b2c3d4" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveEndpointID_default(t *testing.T) {
	got, err := ResolveEndpointID("")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEndpointID(got); err != nil {
		t.Fatalf("invalid default id %q: %v", got, err)
	}
	parts := strings.Split(got, "-")
	if len(parts) < 2 {
		t.Fatalf("expected host-suffix form, got %q", got)
	}
	suffix := parts[len(parts)-1]
	if len(suffix) != endpointIDSuffixLen {
		t.Fatalf("suffix len = %d, want %d (%q)", len(suffix), endpointIDSuffixLen, suffix)
	}
}

func TestValidateEndpointID_rejectsBad(t *testing.T) {
	for _, id := range []string{"", "ab", "has space", "under_score", strings.Repeat("a", 64)} {
		if err := ValidateEndpointID(id); err == nil {
			t.Errorf("ValidateEndpointID(%q) = nil, want error", id)
		}
	}
}
