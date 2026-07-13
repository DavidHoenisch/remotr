package models

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var hostLocaleResourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var localeVariableName = regexp.MustCompile(`^(LANG|LANGUAGE|LC_[A-Z_]+)$`)
var consoleKeymap = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// Validate rejects unsafe values while preserving the independently optional
// ownership of timezone, locale, and keymap fields.
func (h HostLocaleResource) Validate() error {
	if !hostLocaleResourceName.MatchString(h.Name) {
		return fmt.Errorf("host locale resource name %q is invalid", h.Name)
	}
	if h.Timezone == nil && h.Locale == nil && h.Keymap == nil {
		return fmt.Errorf("host locale resource requires timezone, locale, or keymap state")
	}
	if h.Lifecycle != "" && h.Lifecycle != LifecyclePresent {
		return fmt.Errorf("host locale lifecycle %q is unsupported", h.Lifecycle)
	}
	if h.Timezone != nil {
		timezone := strings.TrimSpace(*h.Timezone)
		if timezone == "" || timezone != *h.Timezone {
			return fmt.Errorf("timezone %q is invalid", *h.Timezone)
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			return fmt.Errorf("timezone %q is invalid: %w", timezone, err)
		}
	}
	if h.Locale != nil && len(h.Locale) == 0 {
		return fmt.Errorf("system locale cannot be empty when managed")
	}
	for key, value := range h.Locale {
		if !localeVariableName.MatchString(key) {
			return fmt.Errorf("locale variable %q is invalid", key)
		}
		if value == "" || strings.ContainsAny(value, "\x00\n\r") {
			return fmt.Errorf("locale value for %q is invalid", key)
		}
	}
	if h.Keymap != nil && !consoleKeymap.MatchString(*h.Keymap) {
		return fmt.Errorf("console keymap %q is invalid", *h.Keymap)
	}
	return nil
}
