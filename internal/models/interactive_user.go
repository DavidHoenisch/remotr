package models

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// InteractiveUserSelectionMode defines how a per-user resource chooses local
// interactive accounts.
type InteractiveUserSelectionMode string

const (
	InteractiveUserSelectionAll      InteractiveUserSelectionMode = "all-interactive"
	InteractiveUserSelectionExplicit InteractiveUserSelectionMode = "explicit"
)

// InteractiveUserSelector selects either every interactive local account or
// an explicit, closed list of usernames.
type InteractiveUserSelector struct {
	Mode      InteractiveUserSelectionMode `yaml:"mode"`
	Usernames []string                     `yaml:"usernames,omitempty"`
}

func (s InteractiveUserSelector) Validate() error {
	switch s.Mode {
	case InteractiveUserSelectionAll:
		if len(s.Usernames) != 0 {
			return fmt.Errorf("all-interactive selector must not declare usernames")
		}
	case InteractiveUserSelectionExplicit:
		if len(s.Usernames) == 0 {
			return fmt.Errorf("explicit selector requires at least one username")
		}
		seen := make(map[string]struct{}, len(s.Usernames))
		for _, username := range s.Usernames {
			if strings.TrimSpace(username) != username || !validLocalAccountName(username) {
				return fmt.Errorf("explicit selector username %q is invalid", username)
			}
			if _, exists := seen[username]; exists {
				return fmt.Errorf("explicit selector username %q is duplicated", username)
			}
			seen[username] = struct{}{}
		}
	default:
		return fmt.Errorf("interactive user selector mode %q is invalid", s.Mode)
	}
	return nil
}

// EffectiveSelector preserves the legacy users: interactive input while new
// canonical resources use the structured selector.
func (r UserFileResource) EffectiveSelector() InteractiveUserSelector {
	if r.Selector != nil {
		return *r.Selector
	}
	return InteractiveUserSelector{Mode: InteractiveUserSelectionAll}
}

// Validate checks the author-facing user-file contract at the configuration
// seam, including its structured user selector and home-relative path.
func (r UserFileResource) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("userFile resource requires name")
	}
	if r.Ownership != "" && r.Ownership != OwnershipMerge && r.Ownership != OwnershipAuthoritative {
		return fmt.Errorf("userFile ownership must be merge or authoritative")
	}
	if r.Selector != nil && strings.TrimSpace(r.Users) != "" {
		return fmt.Errorf("userFile must not declare both selector and legacy users")
	}
	if r.Selector != nil {
		if err := r.Selector.Validate(); err != nil {
			return fmt.Errorf("userFile selector: %w", err)
		}
	} else if strings.TrimSpace(r.Users) != "interactive" {
		return fmt.Errorf("userFile legacy users must be %q", "interactive")
	}
	rel := strings.TrimSpace(r.Path)
	if rel == "" {
		return fmt.Errorf("userFile path is required")
	}
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("userFile path must remain relative to the user home directory")
	}
	if r.Lifecycle == LifecycleAbsent {
		if r.UpdateExisting || r.WithRegx != "" || r.ReplaceRegx != "" || r.Content != "" || len(r.Mode) != 0 {
			return fmt.Errorf("absent userFile must not declare content or file metadata")
		}
		return nil
	}
	if r.UpdateExisting && strings.TrimSpace(r.WithRegx) != "" && strings.TrimSpace(r.Content) == "" {
		return fmt.Errorf("userFile line edit requires content")
	}
	if r.UpdateExisting && strings.TrimSpace(r.WithRegx) != "" {
		if _, err := regexp.Compile(strings.TrimSpace(r.WithRegx)); err != nil {
			return fmt.Errorf("userFile invalid withRegx: %w", err)
		}
	}
	if replacement := strings.TrimSpace(r.ReplaceRegx); replacement != "" {
		if _, err := regexp.Compile(replacement); err != nil {
			return fmt.Errorf("userFile invalid replaceRegx: %w", err)
		}
	}
	if !r.UpdateExisting && strings.TrimSpace(r.Content) == "" && strings.TrimSpace(r.WithRegx) == "" {
		return fmt.Errorf("userFile requires content")
	}
	return nil
}

func (r UserFileResource) EffectiveSelectorOwnership() OwnershipMode {
	if r.Ownership == "" {
		return OwnershipMerge
	}
	return r.Ownership
}
