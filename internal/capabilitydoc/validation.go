package capabilitydoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
)

const (
	MaxDocumentBytes         = 65_536
	MaxArtifactSchemas       = 8
	MaxCapabilities          = 512
	MaxCapabilityFeatures    = 64
	MaxFacts                 = 128
	MaxCapabilityIDBytes     = 128
	MaxContractRevisionBytes = 32
	MaxFactKeyBytes          = 64
	MaxFactValueBytes        = 256
	MaxAgentVersionBytes     = 128
	MaxDiagnosticEntries     = 32
	MaxDiagnosticBytes       = 256
)

var (
	identifierPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._:/-][a-z0-9]+)*$`)
	revisionPattern      = regexp.MustCompile(`^(?:0|[1-9][0-9]*(?:\.(?:0|[1-9][0-9]*)){0,2}|[A-Za-z][A-Za-z0-9]*(?:[._-][A-Za-z0-9]+)*)$`)
	factValuePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	distroVersionPattern = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]+){0,3}|[0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	agentVersionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	allowedFactKeys      = map[string]bool{
		"distro": true, "distro-family": true, "distro-version": true,
		"architecture": true, "init": true, "package": true,
		"firewall": true, "network": true, "security": true,
		"desktop": true, "browser": true,
	}
	multiValueFactKeys = map[string]bool{"desktop": true, "browser": true}
	allowedFactValues  = map[string]map[string]bool{
		"distro":        {"ubuntu": true, "debian": true, "arch": true},
		"distro-family": {"debian": true, "arch": true},
		"architecture":  {"x86": true, "arm": true},
		"init":          {"systemd": true, "openrc": true, "sysv": true},
		"package":       {"apt": true, "pacman": true, "yay": true, "dnf": true},
		"firewall":      {"firewalld": true, "nftables": true},
		"network":       {"network-manager": true, "systemd-networkd": true, "netplan": true},
		"security":      {"apparmor": true, "selinux": true},
		"desktop":       {"dconf": true, "gsettings": true},
		"browser":       {"chromium": true, "google-chrome": true, "firefox": true},
	}
)

// ValidationError is a bounded, value-free diagnostic safe to return from an
// authenticated protocol boundary. It intentionally excludes submitted fact
// and capability values.
type ValidationError struct {
	Code  string
	Field string
	cause error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("capability document %s at %s", e.Code, e.Field)
}

func (e *ValidationError) Unwrap() error { return e.cause }

func invalid(code, field string) error {
	return &ValidationError{Code: code, Field: field}
}

func invalidCause(code, field string, cause error) error {
	return &ValidationError{Code: code, Field: field, cause: cause}
}

// Decode strictly decodes one bounded JSON capability document. Unknown
// fields and trailing values are rejected without echoing caller-controlled
// content into diagnostics.
func Decode(raw []byte) (Document, error) {
	if len(raw) > MaxDocumentBytes {
		return Document{}, invalid("document_size", "document")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, invalidCause("document_json", "document", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Document{}, invalid("document_json", "document")
	}
	return document, nil
}

// DecodeCanonical restores and validates a stored canonical body and its
// separately indexed digest. Non-canonical or malformed persisted state fails
// closed without exposing stored values in the error.
func DecodeCanonical(canonical []byte, digest string) (Document, error) {
	document, err := Decode(canonical)
	if err != nil {
		return Document{}, err
	}
	document.Digest = digest
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	reencoded, err := document.CanonicalBody()
	if err != nil || !bytes.Equal(reencoded, canonical) {
		return Document{}, invalid("noncanonical_document", "document")
	}
	return document, nil
}

func validateDocument(document Document) error {
	if document.DocumentVersion != CurrentDocumentVersion {
		return invalid("unsupported_document_version", "documentVersion")
	}
	if len(document.ArtifactSchemaVersions) == 0 || len(document.ArtifactSchemaVersions) > MaxArtifactSchemas {
		return invalid("artifact_schema_count", "artifactSchemaVersions")
	}
	seenSchemas := make(map[int]bool, len(document.ArtifactSchemaVersions))
	for _, schema := range document.ArtifactSchemaVersions {
		if schema != 0 && schema != 1 {
			return invalid("unsupported_artifact_schema", "artifactSchemaVersions")
		}
		if seenSchemas[schema] {
			return invalid("duplicate_artifact_schema", "artifactSchemaVersions")
		}
		seenSchemas[schema] = true
	}
	if len(document.Capabilities) == 0 || len(document.Capabilities) > MaxCapabilities {
		return invalid("capability_count", "capabilities")
	}
	seenCapabilities := make(map[string]string, len(document.Capabilities))
	for _, capability := range document.Capabilities {
		if len(capability.ID) == 0 || len(capability.ID) > MaxCapabilityIDBytes || !identifierPattern.MatchString(capability.ID) {
			return invalid("capability_id", "capabilities.id")
		}
		if len(capability.Revision) == 0 || len(capability.Revision) > MaxContractRevisionBytes || !revisionPattern.MatchString(capability.Revision) {
			return invalid("contract_revision", "capabilities.revision")
		}
		if revision, exists := seenCapabilities[capability.ID]; exists {
			if revision == capability.Revision {
				return invalid("duplicate_capability", "capabilities.id")
			}
			return invalid("conflicting_capability_revision", "capabilities.revision")
		}
		seenCapabilities[capability.ID] = capability.Revision
		if len(capability.Features) > MaxCapabilityFeatures {
			return invalid("capability_feature_count", "capabilities.features")
		}
		seenFeatures := make(map[string]bool, len(capability.Features))
		for _, feature := range capability.Features {
			if len(feature) == 0 || len(feature) > MaxCapabilityIDBytes || !identifierPattern.MatchString(feature) {
				return invalid("capability_feature", "capabilities.features")
			}
			if seenFeatures[feature] {
				return invalid("duplicate_capability_feature", "capabilities.features")
			}
			seenFeatures[feature] = true
		}
	}
	if len(document.Facts) > MaxFacts {
		return invalid("fact_count", "facts")
	}
	seenFacts := make(map[string]bool, len(document.Facts))
	scalarFacts := make(map[string]string, len(document.Facts))
	for _, fact := range document.Facts {
		if len(fact.Key) == 0 || len(fact.Key) > MaxFactKeyBytes || !identifierPattern.MatchString(fact.Key) || !allowedFactKeys[fact.Key] {
			return invalid("fact_key", "facts.key")
		}
		if len(fact.Value) == 0 || len(fact.Value) > MaxFactValueBytes || !factValuePattern.MatchString(fact.Value) || !validFactValue(fact.Key, fact.Value) {
			return invalid("fact_value", "facts.value")
		}
		pair := fact.Key + "\x00" + fact.Value
		if seenFacts[pair] {
			return invalid("duplicate_fact", "facts")
		}
		seenFacts[pair] = true
		if previous, exists := scalarFacts[fact.Key]; exists && previous != fact.Value && !multiValueFactKeys[fact.Key] {
			return invalid("conflicting_fact", "facts")
		}
		scalarFacts[fact.Key] = fact.Value
	}
	if len(document.AgentVersion) == 0 || len(document.AgentVersion) > MaxAgentVersionBytes || !agentVersionPattern.MatchString(document.AgentVersion) {
		return invalid("agent_version", "agentVersion")
	}
	body, err := document.CanonicalBody()
	if err != nil {
		return invalidCause("document_json", "document", err)
	}
	if len(body) > MaxDocumentBytes {
		return invalid("document_size", "document")
	}
	digest, err := document.CanonicalDigest()
	if err != nil {
		return invalidCause("document_json", "document", err)
	}
	if document.Digest != digest {
		return invalidCause("digest_mismatch", "digest", ErrDigestMismatch)
	}
	return nil
}

func validFactValue(key, value string) bool {
	if key == "distro-version" {
		return distroVersionPattern.MatchString(value)
	}
	return allowedFactValues[key][value]
}
