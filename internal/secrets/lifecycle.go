package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

const SecurityEventCompromiseRekey = "secret.compromise-rekey"

// SecretVersionMetadata is safe to place in audit and recovery diagnostics.
type SecretVersionMetadata struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Scope      Scope  `json:"scope"`
	Fleet      string `json:"fleet,omitempty"`
	EndpointID string `json:"endpointId,omitempty"`
}

// SecretSecurityEvent records a cryptographic lifecycle action without secret
// bytes or key material.
type SecretSecurityEvent struct {
	Action            string                  `json:"action"`
	ActiveKEKProvider string                  `json:"activeKekProvider"`
	ActiveKEKID       string                  `json:"activeKekId"`
	PreviousKEKIDs    []string                `json:"previousKekIds"`
	Versions          []SecretVersionMetadata `json:"versions"`
}

type SecretSecurityEventSink interface {
	RecordSecretSecurityEvent(context.Context, SecretSecurityEvent) error
}

// KeyCoverageGap identifies recoverability metadata for an encrypted version
// whose external KEK is unavailable.
type KeyCoverageGap struct {
	ProviderID string `json:"kekProvider"`
	KEKID      string `json:"kekId"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Fleet      string `json:"fleet,omitempty"`
	EndpointID string `json:"endpointId,omitempty"`
}

// ClassifiedMetadata returns the approved backup/restore diagnostic
// projection for one missing key-encryption key. Secret records are identified
// only by references; key and secret bytes are not representable here.
func (gap KeyCoverageGap) ClassifiedMetadata() (executor.SafeSummary, error) {
	fields := []executor.SafeField{
		{Path: "kek_id", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.KEKID},
		{Path: "kek_provider", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.ProviderID},
		{Path: "secret_name", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: gap.Name},
		{Path: "secret_version", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: gap.Version},
	}
	if gap.Fleet != "" {
		fields = append(fields, executor.SafeField{
			Path: "fleet", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.Fleet,
		})
	}
	if gap.EndpointID != "" {
		fields = append(fields, executor.SafeField{
			Path: "endpoint_id", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.EndpointID,
		})
	}
	return executor.NewSafeSummary(fields)
}

// MarshalJSON prevents backup/restore diagnostics from falling back to an
// unclassified map of database record metadata.
func (gap KeyCoverageGap) MarshalJSON() ([]byte, error) {
	classified, err := gap.ClassifiedMetadata()
	if err != nil {
		return nil, err
	}
	return json.Marshal(classified)
}

type KeyCoverageReport struct {
	Complete bool             `json:"complete"`
	Missing  []KeyCoverageGap `json:"missing"`
}

// CheckKeyCoverage verifies that a restored external keyring can unwrap every
// supplied database record. It never returns key or secret material.
func CheckKeyCoverage(ctx context.Context, records []EncryptedRecord, provider KeyEncryptionProvider) (KeyCoverageReport, error) {
	report := KeyCoverageReport{Complete: true, Missing: []KeyCoverageGap{}}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return KeyCoverageReport{}, fmt.Errorf("validate restored encrypted secret record: %w", err)
		}
		available := false
		var err error
		if provider != nil {
			available, err = provider.KeyAvailable(ctx, record.KEKProvider, record.KEKID)
			if err != nil {
				gap := keyCoverageGapFromRecord(record)
				details, classifyErr := gap.ClassifiedMetadata()
				if classifyErr != nil {
					return KeyCoverageReport{}, executor.NewSafeError("secret_key_coverage_failed", "secret_key_coverage", err)
				}
				return KeyCoverageReport{}, executor.NewSafeErrorWithDetails("secret_key_coverage_failed", "secret_key_coverage", err, details)
			}
		}
		if available {
			continue
		}
		report.Complete = false
		report.Missing = append(report.Missing, keyCoverageGapFromRecord(record))
	}
	sort.Slice(report.Missing, func(i, j int) bool {
		left, right := report.Missing[i], report.Missing[j]
		if left.ProviderID != right.ProviderID {
			return left.ProviderID < right.ProviderID
		}
		if left.KEKID != right.KEKID {
			return left.KEKID < right.KEKID
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Version < right.Version
	})
	return report, nil
}

func keyCoverageGapFromRecord(record EncryptedRecord) KeyCoverageGap {
	return KeyCoverageGap{
		ProviderID: record.KEKProvider,
		KEKID:      record.KEKID,
		Name:       record.Scope.Name,
		Version:    record.Scope.Version,
		Fleet:      record.Scope.Fleet,
		EndpointID: record.Scope.EndpointID,
	}
}

// CheckKeyCoverage verifies restored encrypted records against this envelope's
// configured current and historical key-encryption providers.
func (e *Envelope) CheckKeyCoverage(ctx context.Context, records []EncryptedRecord) (KeyCoverageReport, error) {
	if e == nil || e.provider == nil {
		return KeyCoverageReport{}, errors.New("secret envelope is required for restored key coverage")
	}
	return CheckKeyCoverage(ctx, records, e.provider)
}

// CompromiseRekey decrypts and fully re-encrypts every supplied version under
// fresh DEKs. Unlike routine Rewrap, it deliberately replaces ciphertext.
func (e *Envelope) CompromiseRekey(ctx context.Context, records []EncryptedRecord, sink SecretSecurityEventSink) ([]EncryptedRecord, error) {
	if sink == nil {
		return nil, fmt.Errorf("security event sink is required for compromise rekey")
	}
	rekeyed := make([]EncryptedRecord, 0, len(records))
	versions := make([]SecretVersionMetadata, 0, len(records))
	previousSet := make(map[string]struct{})
	for _, record := range records {
		plaintext, err := e.DecryptContext(ctx, record)
		if err != nil {
			return nil, fmt.Errorf("rekey secret %q version %q: %w", record.Scope.Name, record.Scope.Version, err)
		}
		next, err := e.EncryptContext(ctx, record.Scope, plaintext)
		clear(plaintext)
		if err != nil {
			return nil, fmt.Errorf("rekey secret %q version %q: %w", record.Scope.Name, record.Scope.Version, err)
		}
		rekeyed = append(rekeyed, next)
		versions = append(versions, metadataFromScope(record.Scope))
		previousSet[record.KEKProvider+"/"+record.KEKID] = struct{}{}
	}
	previous := make([]string, 0, len(previousSet))
	for id := range previousSet {
		previous = append(previous, id)
	}
	sort.Strings(previous)
	activeKeyID, err := e.provider.ActiveKeyID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read active key-encryption key: %w", err)
	}
	event := SecretSecurityEvent{
		Action:            SecurityEventCompromiseRekey,
		ActiveKEKProvider: e.provider.ProviderID(),
		ActiveKEKID:       activeKeyID,
		PreviousKEKIDs:    previous,
		Versions:          versions,
	}
	if err := sink.RecordSecretSecurityEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("record compromise-rekey security event: %w", err)
	}
	return rekeyed, nil
}

func metadataFromScope(scope ScopeMetadata) SecretVersionMetadata {
	kind, _, _, _ := normalizeScope(scope.Scope, scope.Fleet, scope.EndpointID, true)
	return SecretVersionMetadata{
		Name:       scope.Name,
		Version:    scope.Version,
		Scope:      kind,
		Fleet:      scope.Fleet,
		EndpointID: scope.EndpointID,
	}
}
