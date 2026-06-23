package main

import (
	"testing"
)

func TestReadFlagOrStdin(t *testing.T) {
	got, err := readFlagOrStdin(" literal ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "literal" {
		t.Fatalf("got %q", got)
	}
}

func TestCLIErrorExitCode(t *testing.T) {
	e := &cliError{Title: "test", exitCode: 4, Code: codeDrift}
	if e.ExitCode() != 4 {
		t.Fatalf("exit code = %d", e.ExitCode())
	}
	if !containsAll(e.Error(), "error:", "test", "E_DRIFT") {
		t.Fatalf("format = %q", e.Error())
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !stringsContains(s, p) {
			return false
		}
	}
	return true
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexSubstring(s, sub) >= 0)
}

func indexSubstring(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
