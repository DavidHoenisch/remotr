package models

import (
	"fmt"
	"strings"
)

// KnownHostScope identifies the known_hosts target owned by a resource.
type KnownHostScope string

const (
	KnownHostScopeSystem KnownHostScope = "system"
	KnownHostScopeUser   KnownHostScope = "user"
)

// KnownHostHashing controls whether host patterns are stored literally or in
// OpenSSH's |1| salted HMAC form.
type KnownHostHashing string

const (
	KnownHostHashPlain  KnownHostHashing = "plain"
	KnownHostHashHashed KnownHostHashing = "hash"
)

// KnownHostResource owns one marked known_hosts entry. The marker lets it
// merge safely with administrator-managed entries in the same file.
type KnownHostResource struct {
	ResourceMeta    `yaml:",inline"`
	Name            string           `yaml:"name"`
	Scope           KnownHostScope   `yaml:"scope"`
	User            string           `yaml:"user,omitempty"`
	Hosts           []string         `yaml:"hosts,omitempty"`
	Type            string           `yaml:"type,omitempty"`
	Key             string           `yaml:"key,omitempty"`
	Fingerprint     string           `yaml:"fingerprint,omitempty"`
	Comment         string           `yaml:"comment,omitempty"`
	Hashing         KnownHostHashing `yaml:"hashing,omitempty"`
	ReplaceExisting bool             `yaml:"replaceExisting,omitempty"`
}

// Validate rejects known-host declarations that could replace an ambiguous
// target or write invalid OpenSSH key material.
func (r KnownHostResource) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("knownHost resource requires name")
	}
	switch r.Scope {
	case KnownHostScopeSystem:
		if r.User != "" {
			return fmt.Errorf("system knownHost %q must not select a user", r.Name)
		}
	case KnownHostScopeUser:
		if !validLocalAccountName(r.User) {
			return fmt.Errorf("user knownHost %q has invalid user %q", r.Name, r.User)
		}
	default:
		return fmt.Errorf("knownHost %q scope must be system or user", r.Name)
	}
	if r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("knownHost %q lifecycle must be present or absent", r.Name)
	}
	if r.Ownership != OwnershipNamed && r.Ownership != OwnershipMerge {
		return fmt.Errorf("knownHost %q ownership must be named or merge", r.Name)
	}
	if r.Hashing != KnownHostHashPlain && r.Hashing != KnownHostHashHashed {
		return fmt.Errorf("knownHost %q hashing must be plain or hash", r.Name)
	}
	if r.Lifecycle == LifecycleAbsent {
		if len(r.Hosts) != 0 || r.Type != "" || r.Key != "" || r.Fingerprint != "" || r.Comment != "" || r.ReplaceExisting {
			return fmt.Errorf("knownHost %q absent lifecycle must not declare host material", r.Name)
		}
		return nil
	}
	if len(r.Hosts) == 0 {
		return fmt.Errorf("knownHost %q requires hosts", r.Name)
	}
	seen := make(map[string]struct{}, len(r.Hosts))
	for _, host := range r.Hosts {
		host = strings.TrimSpace(host)
		if host == "" || strings.ContainsAny(host, " \t,\r\n\x00") {
			return fmt.Errorf("knownHost %q has invalid host pattern", r.Name)
		}
		if _, exists := seen[host]; exists {
			return fmt.Errorf("knownHost %q has duplicate host pattern %q", r.Name, host)
		}
		seen[host] = struct{}{}
	}
	if err := (AuthorizedKeyEntry{Type: r.Type, Key: r.Key, Fingerprint: r.Fingerprint}).Validate(); err != nil {
		return fmt.Errorf("knownHost %q: %w", r.Name, err)
	}
	if strings.ContainsAny(r.Comment, "\r\n\x00") {
		return fmt.Errorf("knownHost %q comment must not contain control characters", r.Name)
	}
	return nil
}
