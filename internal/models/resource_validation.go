package models

import (
	"fmt"
	"strings"
	"time"
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
