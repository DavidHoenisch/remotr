package models

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type AccountLimitType string

const (
	AccountLimitSoft AccountLimitType = "soft"
	AccountLimitHard AccountLimitType = "hard"
	AccountLimitBoth AccountLimitType = "-"
)

var (
	accountLimitName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)
	accountLimitDomain = regexp.MustCompile(`^(\*|%|[@%]?[A-Za-z_][A-Za-z0-9_.-]*)$`)
)

type AccountLimitEntry struct {
	Domain string           `yaml:"domain"`
	Type   AccountLimitType `yaml:"type"`
	Item   string           `yaml:"item"`
	Value  string           `yaml:"value"`
}

type AccountLimitResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string              `yaml:"name"`
	Entries      []AccountLimitEntry `yaml:"entries,omitempty"`
}

func (r AccountLimitResource) Validate() error {
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if !accountLimitName.MatchString(r.Name) {
		return fmt.Errorf("account limit name must be a safe named-fragment identifier")
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("account limit lifecycle must be present or absent")
	}
	if lifecycle == LifecycleAbsent {
		if len(r.Entries) != 0 {
			return fmt.Errorf("absent account limit fragment must omit entries")
		}
		return nil
	}
	if len(r.Entries) == 0 {
		return fmt.Errorf("account limit fragment requires at least one entry")
	}
	seen := make(map[string]struct{}, len(r.Entries))
	for i, entry := range r.Entries {
		if !accountLimitDomain.MatchString(entry.Domain) {
			return fmt.Errorf("account limit entry %d has invalid domain", i+1)
		}
		if entry.Type != AccountLimitSoft && entry.Type != AccountLimitHard && entry.Type != AccountLimitBoth {
			return fmt.Errorf("account limit entry %d has invalid type", i+1)
		}
		if !validAccountLimitItem(entry.Item) {
			return fmt.Errorf("account limit entry %d has unknown item %q", i+1, entry.Item)
		}
		if entry.Value != "unlimited" && entry.Value != "infinity" {
			if _, err := strconv.ParseInt(entry.Value, 10, 64); err != nil {
				return fmt.Errorf("account limit entry %d value must be an integer, unlimited, or infinity", i+1)
			}
		}
		key := strings.Join([]string{entry.Domain, string(entry.Type), entry.Item}, "\x00")
		if _, exists := seen[key]; exists {
			return fmt.Errorf("account limit entry %d duplicates domain/type/item", i+1)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validAccountLimitItem(item string) bool {
	switch item {
	case "core", "data", "fsize", "memlock", "nofile", "rss", "stack", "cpu", "nproc", "as", "maxlogins", "maxsyslogins", "priority", "locks", "sigpending", "msgqueue", "nice", "rtprio", "chroot":
		return true
	default:
		return false
	}
}
