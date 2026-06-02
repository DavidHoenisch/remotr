package main

import "testing"

func TestLabelFromFlagOrArg(t *testing.T) {
	if got, ok := labelFromFlagOrArg("from-flag", []string{"positional"}); !ok || got != "from-flag" {
		t.Fatalf("flag precedence: got %q ok=%v", got, ok)
	}
	if got, ok := labelFromFlagOrArg("", []string{"positional"}); !ok || got != "positional" {
		t.Fatalf("positional: got %q ok=%v", got, ok)
	}
	if _, ok := labelFromFlagOrArg("", nil); ok {
		t.Fatal("expected missing label")
	}
}
