package endpointlabel

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxKeyLen   = 64
	maxValueLen = 512
)

// ValidateKey checks an endpoint label key from operator or agent inventory.
func ValidateKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("label key is required")
	}
	if utf8.RuneCountInString(key) > maxKeyLen {
		return fmt.Errorf("label key exceeds %d characters", maxKeyLen)
	}
	for i, r := range key {
		if r == ' ' || r == '=' || r == '\n' || r == '\r' || r == '\t' {
			return fmt.Errorf("label key contains invalid character")
		}
		if i == 0 && r == '.' {
			return fmt.Errorf("label key must not start with '.'")
		}
	}
	return nil
}

// ValidateValue checks an endpoint label value.
func ValidateValue(value string) error {
	if utf8.RuneCountInString(value) > maxValueLen {
		return fmt.Errorf("label value exceeds %d characters", maxValueLen)
	}
	return nil
}
