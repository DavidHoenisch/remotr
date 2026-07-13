package models

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// AuthorizedKeyEntry is one OpenSSH public-key authorization with the
// restrictions that materially affect its access grant.
type AuthorizedKeyEntry struct {
	Type         string   `yaml:"type"`
	Key          string   `yaml:"key"`
	Fingerprint  string   `yaml:"fingerprint"`
	Comment      string   `yaml:"comment,omitempty"`
	Restrictions []string `yaml:"restrictions,omitempty"`
	Principals   []string `yaml:"principals,omitempty"`
	ExpiresAt    string   `yaml:"expiresAt,omitempty"`
}

// AuthorizedKeyResource owns one marked set of authorized_keys entries for a
// local account. Entries outside the resource marker are never modified.
type AuthorizedKeyResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string               `yaml:"name"`
	User         string               `yaml:"user"`
	Entries      []AuthorizedKeyEntry `yaml:"entries,omitempty"`
}

// Validate rejects key declarations that cannot be rendered as safe OpenSSH
// authorization entries. The SHA-256 fingerprint is checked against the key
// blob so copied or truncated key data cannot silently grant different access.
func (r AuthorizedKeyResource) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("authorizedKey resource requires name")
	}
	if !validLocalAccountName(r.User) {
		return fmt.Errorf("authorizedKey %q has invalid user %q", r.Name, r.User)
	}
	if r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("authorizedKey %q lifecycle must be present or absent", r.Name)
	}
	if r.Ownership != OwnershipMerge && r.Ownership != OwnershipAuthoritative {
		return fmt.Errorf("authorizedKey %q ownership must be merge or authoritative", r.Name)
	}
	if r.Lifecycle == LifecycleAbsent && len(r.Entries) != 0 {
		return fmt.Errorf("authorizedKey %q absent lifecycle must not declare entries", r.Name)
	}
	seen := make(map[string]struct{}, len(r.Entries))
	for i, entry := range r.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("authorizedKey %q entry %d: %w", r.Name, i+1, err)
		}
		if _, exists := seen[entry.Fingerprint]; exists {
			return fmt.Errorf("authorizedKey %q has duplicate fingerprint %q", r.Name, entry.Fingerprint)
		}
		seen[entry.Fingerprint] = struct{}{}
	}
	return nil
}

// Validate confirms the public key, fingerprint, and rendering-safe metadata.
func (e AuthorizedKeyEntry) Validate() error {
	typ := strings.TrimSpace(e.Type)
	if typ == "" || strings.ContainsAny(typ, " \t\r\n\x00") {
		return fmt.Errorf("key type is required")
	}
	key := strings.TrimSpace(e.Key)
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) < 8 {
		return fmt.Errorf("key must be base64 OpenSSH public-key data")
	}
	length := int(binary.BigEndian.Uint32(decoded[:4]))
	if length <= 0 || 4+length > len(decoded) || string(decoded[4:4+length]) != typ {
		return fmt.Errorf("key type does not match OpenSSH public-key data")
	}
	sum := sha256.Sum256(decoded)
	want := "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(sum[:]), "=")
	if e.Fingerprint != want {
		return fmt.Errorf("fingerprint %q does not match key (want %q)", e.Fingerprint, want)
	}
	if strings.ContainsAny(e.Comment, "\r\n\x00") {
		return fmt.Errorf("comment must not contain control characters")
	}
	seen := make(map[string]struct{}, len(e.Restrictions))
	for _, restriction := range e.Restrictions {
		restriction = strings.TrimSpace(restriction)
		if restriction == "" || strings.ContainsAny(restriction, "\r\n\x00") || strings.Contains(restriction, ",,") {
			return fmt.Errorf("invalid key restriction")
		}
		if _, exists := seen[restriction]; exists {
			return fmt.Errorf("duplicate key restriction %q", restriction)
		}
		seen[restriction] = struct{}{}
	}
	seen = make(map[string]struct{}, len(e.Principals))
	for _, principal := range e.Principals {
		principal = strings.TrimSpace(principal)
		if principal == "" || strings.ContainsAny(principal, ",\r\n\x00") {
			return fmt.Errorf("invalid key principal")
		}
		if _, exists := seen[principal]; exists {
			return fmt.Errorf("duplicate key principal %q", principal)
		}
		seen[principal] = struct{}{}
	}
	if e.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, e.ExpiresAt); err != nil {
			return fmt.Errorf("expiresAt must be RFC3339: %w", err)
		}
	}
	return nil
}
