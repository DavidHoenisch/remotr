package files

import (
	"regexp"
	"testing"
)

func Test_applyLineEdit_replacesMatchingLine(t *testing.T) {
	re := regexp.MustCompile(`^#?\s*PASS_MAX_DAYS[[:space:]].*`)
	got, replaced, err := applyLineEdit("PASS_MAX_DAYS 999\nPASS_MIN_DAYS 0\n", re, "PASS_MAX_DAYS 90")
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Fatal("expected replacement")
	}
	want := "PASS_MAX_DAYS 90\nPASS_MIN_DAYS 0\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func Test_applyLineEdit_appendsWhenNoMatch(t *testing.T) {
	re := regexp.MustCompile(`^PASS_WARN_AGE[[:space:]].*`)
	got, replaced, err := applyLineEdit("PASS_MAX_DAYS 90\n", re, "PASS_WARN_AGE 7")
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("expected append")
	}
	if got != "PASS_MAX_DAYS 90\nPASS_WARN_AGE 7\n" {
		t.Fatalf("got %q", got)
	}
}
