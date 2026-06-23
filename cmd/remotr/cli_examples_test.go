package main

import (
	"strings"
	"testing"
)

func TestWithExamples(t *testing.T) {
	got := withExamples("desc", "remotr endpoint list")
	if !strings.Contains(got, "desc") || !strings.Contains(got, "Examples:") || !strings.Contains(got, "remotr endpoint list") {
		t.Fatalf("withExamples = %q", got)
	}
}
