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

func TestMatch(t *testing.T) {
	if !agentversion.Match("v0.1.12", "0.1.12") {
		t.Fatal("expected match")
	}
	if agentversion.Match("v0.1.11", "v0.1.12") {
		t.Fatal("expected no match")
	}
}
