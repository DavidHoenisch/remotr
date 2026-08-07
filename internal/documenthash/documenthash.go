// Package documenthash defines bounded, acknowledged hashes for repeatable
// Sync documents. Hashes are domain-separated by protocol version and
// document type so equal bytes in different document domains cannot alias.
package documenthash

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
)

const (
	CurrentVersion      = 1
	MaxSummaryBytes     = 2 << 10
	MaxDocuments        = 8
	MaxNameBytes        = 64
	MaxDocumentBytes    = 64 << 10
	MaxTargetLabels     = 64
	MaxTargetUsers      = 1024
	MaxTargetKeyBytes   = 64
	MaxTargetValueBytes = 256

	Capability        = "capability"
	SystemInformation = "systemInformation"
	Delivery          = "delivery"
	Targeting         = "targeting"
)

var (
	hashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	knownNames  = map[string]bool{
		Capability: true, SystemInformation: true, Delivery: true, Targeting: true,
	}
)

// Known reports whether name is part of the closed Sync document vocabulary.
func Known(name string) bool { return knownNames[name] }

// Summary is the optional versioned document-hash section of a Sync message.
type Summary struct {
	Version   int               `json:"version"`
	Documents map[string]string `json:"documents"`
}

// Decode strictly decodes and validates one bounded hash summary.
func Decode(raw []byte) (Summary, error) {
	if len(raw) == 0 || len(raw) > MaxSummaryBytes {
		return Summary{}, errors.New("document hash summary size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var summary Summary
	if err := decoder.Decode(&summary); err != nil {
		return Summary{}, errors.New("document hash summary is malformed")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Summary{}, errors.New("document hash summary has trailing data")
	}
	if err := summary.Validate(); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

// Validate enforces the closed document vocabulary and canonical hash syntax.
func (s Summary) Validate() error {
	if s.Version != CurrentVersion {
		return errors.New("document hash summary version is unsupported")
	}
	if len(s.Documents) == 0 || len(s.Documents) > MaxDocuments {
		return errors.New("document hash summary count is invalid")
	}
	for name, hash := range s.Documents {
		if len(name) == 0 || len(name) > MaxNameBytes || !knownNames[name] {
			return errors.New("document hash name is invalid")
		}
		if !hashPattern.MatchString(hash) {
			return errors.New("document hash is invalid")
		}
	}
	return nil
}

// Digest returns a lower-case SHA-256 digest over the versioned,
// document-type-domain-separated canonical semantic bytes.
func Digest(name string, canonical []byte) (string, error) {
	if !knownNames[name] {
		return "", fmt.Errorf("unknown document hash domain")
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("remotr-sync-document\x00v1\x00"))
	_, _ = hasher.Write([]byte(name))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write(canonical)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

// CanonicalJSON decodes one bounded JSON document and returns stable semantic
// bytes. Object keys are ordered by encoding/json and number spellings remain
// exact through UseNumber.
func CanonicalJSON(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxDocumentBytes {
		return nil, errors.New("document JSON size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("document JSON is malformed")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("document JSON has trailing data")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("document JSON cannot be canonicalized")
	}
	return canonical, nil
}

// CanonicalDelivery returns stable semantic bytes for the artifact
// acknowledgement carried by every Sync request.
func CanonicalDelivery(releaseRef, digest string) ([]byte, error) {
	return boundedCanonical(struct {
		ReleaseRef string `json:"releaseRef"`
		Digest     string `json:"digest"`
	}{ReleaseRef: releaseRef, Digest: digest})
}

// CanonicalTargeting returns stable semantic bytes for the complete set of
// endpoint inputs that can affect targeting. Usernames are set-like and are
// therefore sorted and deduplicated before hashing.
func CanonicalTargeting(labels map[string]string, usernames []string) ([]byte, error) {
	if len(labels) > MaxTargetLabels || len(usernames) > MaxTargetUsers {
		return nil, errors.New("targeting document count is invalid")
	}
	stableLabels := make(map[string]string, len(labels))
	for key, value := range labels {
		if len(key) == 0 || len(key) > MaxTargetKeyBytes || len(value) > MaxTargetValueBytes {
			return nil, errors.New("targeting label is invalid")
		}
		stableLabels[key] = value
	}
	stableUsers := make([]string, 0, len(usernames))
	seenUsers := make(map[string]bool, len(usernames))
	for _, username := range usernames {
		if len(username) == 0 || len(username) > MaxTargetValueBytes {
			return nil, errors.New("targeting username is invalid")
		}
		if !seenUsers[username] {
			seenUsers[username] = true
			stableUsers = append(stableUsers, username)
		}
	}
	sort.Strings(stableUsers)
	return boundedCanonical(struct {
		Labels    map[string]string `json:"labels"`
		Usernames []string          `json:"usernames"`
	}{Labels: stableLabels, Usernames: stableUsers})
}

func boundedCanonical(value any) ([]byte, error) {
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("document cannot be canonicalized")
	}
	if len(canonical) > MaxDocumentBytes {
		return nil, errors.New("document is too large")
	}
	return canonical, nil
}

// Equal compares canonical hash strings without content-dependent timing.
func Equal(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
