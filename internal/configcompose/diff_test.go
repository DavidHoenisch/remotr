package configcompose

import (
	"strings"
	"testing"
)

func TestUnifiedLineDiff_singleHunk(t *testing.T) {
	current := []byte(`configurations:
  - name: base
    packages:
      - name: curl
        present: true
  - name: other
    packages:
      - name: jq
        present: true
`)
	composed := []byte(`configurations:
  - name: base
    packages:
      - name: curl
        present: true
      - name: vim
        present: true
  - name: other
    packages:
      - name: jq
        present: true
`)
	diff := unifiedLineDiff("fleets/lab/desired.yaml", current, composed)
	if diff == "" {
		t.Fatal("expected diff")
	}
	if strings.Contains(diff, "-  - name: other") {
		t.Fatalf("diff should not remove unchanged tail: %s", diff)
	}
	if !strings.Contains(diff, "+      - name: vim") {
		t.Fatalf("diff missing added line: %s", diff)
	}
	if strings.Count(diff, "\n-") > 3 {
		t.Fatalf("expected small hunk, got full-file diff:\n%s", diff)
	}
}

func TestLineDiff_semanticallyEqualFormatting(t *testing.T) {
	current := []byte("configurations:\n  - name: base\n")
	composed := []byte("configurations:\n  - name: base\n\n")
	if diff := lineDiff("fleets/lab/desired.yaml", current, composed); diff != "" {
		t.Fatalf("expected no diff after normalization, got: %s", diff)
	}
}
