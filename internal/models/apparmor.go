package models

import (
	"fmt"
	"regexp"
	"strings"
)

type AppArmorMode string

const (
	AppArmorEnforce  AppArmorMode = "enforce"
	AppArmorComplain AppArmorMode = "complain"
	AppArmorDisabled AppArmorMode = "disabled"
)

var appArmorFragmentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)

type AppArmorProfileResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string       `yaml:"name"`
	Profile      string       `yaml:"profile"`
	Content      string       `yaml:"content"`
	Mode         AppArmorMode `yaml:"mode"`
}

func (r AppArmorProfileResource) Validate() error {
	if !appArmorFragmentName.MatchString(r.Name) {
		return fmt.Errorf("AppArmor name must be a safe named-fragment identifier")
	}
	if strings.TrimSpace(r.Profile) == "" || r.Profile != strings.TrimSpace(r.Profile) || strings.ContainsAny(r.Profile, "\x00\r\n") {
		return fmt.Errorf("AppArmor profile identity is invalid")
	}
	if strings.TrimSpace(r.Content) == "" {
		return fmt.Errorf("AppArmor profile content is required")
	}
	switch r.Mode {
	case AppArmorEnforce, AppArmorComplain, AppArmorDisabled:
		return nil
	default:
		return fmt.Errorf("AppArmor mode must be enforce, complain, or disabled")
	}
}
