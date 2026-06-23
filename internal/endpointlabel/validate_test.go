package endpointlabel

import "testing"

func TestValidateKey(t *testing.T) {
	if err := ValidateKey("site"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateKey(""); err == nil {
		t.Fatal("expected error for empty key")
	}
	if err := ValidateKey("bad key"); err == nil {
		t.Fatal("expected error for space in key")
	}
}

func TestValidateValue(t *testing.T) {
	if err := ValidateValue("berlin"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateValue(""); err != nil {
		t.Fatal("empty value should be allowed")
	}
}
