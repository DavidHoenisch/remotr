package userutil

import "testing"

func TestValidateLinuxUsername(t *testing.T) {
	valid := []string{"root", "deploy", "app_user", "user-1"}
	for _, u := range valid {
		if err := ValidateLinuxUsername(u); err != nil {
			t.Fatalf("%q: %v", u, err)
		}
	}
	invalid := []string{"", "-bad", "..", "a/b", "user;id", stringsLong(33)}
	for _, u := range invalid {
		if err := ValidateLinuxUsername(u); err == nil {
			t.Fatalf("%q should be invalid", u)
		}
	}
}

func stringsLong(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
