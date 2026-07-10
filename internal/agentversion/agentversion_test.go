package agentversion_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
)

func TestNormalize(t *testing.T) {
	got, err := agentversion.Normalize("0.1.12")
	if err != nil || got != "v0.1.12" {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = agentversion.Normalize("v0.1.12")
	if err != nil || got != "v0.1.12" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestNormalizeRejectsNonSemver(t *testing.T) {
	for _, raw := range []string{"", "v", "not-semver", "release/../../weird?x=1", "1.2", "1.2.3/evil"} {
		if got, err := agentversion.Normalize(raw); err == nil {
			t.Fatalf("Normalize(%q) = %q, want error", raw, got)
		}
	}
}

func TestMatch(t *testing.T) {
	if !agentversion.Match("v0.1.12", "0.1.12") {
		t.Fatal("expected match")
	}
	if agentversion.Match("v0.1.11", "v0.1.12") {
		t.Fatal("expected no match")
	}
}

func TestCompare(t *testing.T) {
	less, err := agentversion.Compare("v0.2.0", "v0.2.1")
	if err != nil || less != -1 {
		t.Fatalf("compare = %d err = %v", less, err)
	}
	same, err := agentversion.Compare("0.2.1", "v0.2.1")
	if err != nil || same != 0 {
		t.Fatalf("compare = %d err = %v", same, err)
	}
	greater, err := agentversion.Compare("v0.3.0", "v0.2.9")
	if err != nil || greater != 1 {
		t.Fatalf("compare = %d err = %v", greater, err)
	}
}
