package models

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/secretref"
)

// GroupMembershipMode declares whether supplementary groups extend existing
// membership or define the complete owned supplementary set.
type GroupMembershipMode string

const (
	GroupMembershipMerge         GroupMembershipMode = "merge"
	GroupMembershipAuthoritative GroupMembershipMode = "authoritative"
)

// Validate enforces only user fields implemented by the current provider.
func (u UserResource) Validate() error {
	if strings.TrimSpace(u.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if u.UID < 0 {
		return fmt.Errorf("uid must not be negative")
	}
	if !u.Present && u.UID != 0 {
		return fmt.Errorf("uid is unsupported for an absent user")
	}
	if u.AllowUIDReassignment && u.UID == 0 {
		return fmt.Errorf("allowUIDReassignment requires uid")
	}
	if u.PrimaryGroup != "" && !validLocalAccountName(u.PrimaryGroup) {
		return fmt.Errorf("primaryGroup is invalid")
	}
	if u.SupplementaryGroupsMode != "" && u.SupplementaryGroupsMode != GroupMembershipMerge && u.SupplementaryGroupsMode != GroupMembershipAuthoritative {
		return fmt.Errorf("unknown supplementaryGroupsMode %q", u.SupplementaryGroupsMode)
	}
	if len(u.SupplementaryGroups) > 0 && u.SupplementaryGroupsMode == "" {
		return fmt.Errorf("supplementaryGroups requires supplementaryGroupsMode")
	}
	if u.SupplementaryGroupsMode == GroupMembershipAuthoritative && u.Ownership != OwnershipAuthoritative {
		return fmt.Errorf("authoritative supplementary groups require authoritative ownership")
	}
	seenGroups := map[string]struct{}{}
	for _, group := range u.SupplementaryGroups {
		if !validLocalAccountName(group) {
			return fmt.Errorf("supplementary group %q is invalid", group)
		}
		if _, exists := seenGroups[group]; exists {
			return fmt.Errorf("supplementary group %q is duplicated", group)
		}
		seenGroups[group] = struct{}{}
	}
	if u.Home != "" && !filepath.IsAbs(filepath.Clean(u.Home)) {
		return fmt.Errorf("home must be absolute")
	}
	if u.Shell != "" && !filepath.IsAbs(filepath.Clean(u.Shell)) {
		return fmt.Errorf("shell must be absolute")
	}
	if strings.ContainsAny(u.Comment, "\x00\r\n") {
		return fmt.Errorf("comment must not contain control characters")
	}
	if u.System != nil {
		if u.UID == 0 {
			return fmt.Errorf("system class requires uid")
		}
		if (*u.System && u.UID >= 1000) || (!*u.System && u.UID < 1000) {
			return fmt.Errorf("system class conflicts with uid range")
		}
	}
	if u.PasswordHashRef != "" {
		if err := secretref.Validate(u.PasswordHashRef); err != nil {
			return fmt.Errorf("passwordHashRef: %w", err)
		}
	}
	if u.Expiry != "" && u.Expiry != "never" {
		if _, err := time.Parse("2006-01-02", u.Expiry); err != nil {
			return fmt.Errorf("expiry must be YYYY-MM-DD or never")
		}
	}
	if !u.Present && (u.PasswordHashRef != "" || u.Locked != nil || u.Expiry != "") {
		return fmt.Errorf("password, lock, and expiry are unsupported for an absent user")
	}
	return nil
}

func (d DownloadResource) Validate() error {
	if strings.TrimSpace(d.Dest) == "" {
		return fmt.Errorf("download destination is required")
	}
	if d.Lifecycle != LifecycleAbsent && strings.TrimSpace(d.URL) == "" {
		return fmt.Errorf("download URL is required")
	}
	if (d.Signature == "") != (d.TrustedSigner == "") {
		return fmt.Errorf("signature and trustedSigner must be provided together")
	}
	switch d.RedirectPolicy {
	case "", "follow", "same-origin", "none":
	default:
		return fmt.Errorf("unknown redirect policy %q", d.RedirectPolicy)
	}
	if d.Timeout != "" {
		parsed, err := time.ParseDuration(d.Timeout)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("timeout must be a positive duration")
		}
	}
	return nil
}
