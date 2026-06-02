package deploytoken

import (
	"testing"
)

func TestIssueParseRoundTrip(t *testing.T) {
	full, id, err := Issue()
	if err != nil {
		t.Fatal(err)
	}
	gotID, secret, err := Parse(full)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != id {
		t.Fatalf("id = %v, want %v", gotID, id)
	}
	hash, err := HashSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifySecret(hash, secret) {
		t.Fatal("expected secret to verify")
	}
	if VerifySecret(hash, "wrong") {
		t.Fatal("wrong secret should not verify")
	}
}

func TestValidateLabel(t *testing.T) {
	if err := ValidateLabel("prod-agents_01"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLabel(""); err == nil {
		t.Fatal("expected error for empty label")
	}
	if err := ValidateLabel("bad label"); err == nil {
		t.Fatal("expected error for space in label")
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := []string{"", "noseparator", "not-a-uuid.secret", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.tooshort"}
	for _, c := range cases {
		if _, _, err := Parse(c); err == nil {
			t.Fatalf("Parse(%q) expected error", c)
		}
	}
}
