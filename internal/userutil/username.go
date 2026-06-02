package userutil

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateLinuxUsername checks a POSIX-style login name safe for useradd/userdel.
func ValidateLinuxUsername(name string) error {
	if name == "" {
		return fmt.Errorf("empty username")
	}
	if len(name) > 32 {
		return fmt.Errorf("username too long")
	}
	if name == "." || name == ".." || strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid username")
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		default:
			if i == 0 {
				return fmt.Errorf("invalid username")
			}
			if !unicode.IsPrint(r) {
				return fmt.Errorf("invalid username")
			}
			return fmt.Errorf("invalid username")
		}
	}
	return nil
}
