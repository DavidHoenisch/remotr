package rollbackstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

const maxSuccessfulPriorStates = 3

// RecordInfo is the safe, payload-free retention view of one transaction.
type RecordInfo struct {
	Version          int
	State            Lifecycle
	Address          string
	ArtifactDigest   string
	Attempt          int
	CreatedAt        time.Time
	Armed            bool
	Sensitive        bool
	Successful       bool
	ExpiresAt        time.Time
	PayloadAvailable bool
}

// ClassifiedMetadata returns the only generic serialization shape for
// rollback metadata. Recovery payload bytes and their fingerprints never enter
// this projection.
func (record RecordInfo) ClassifiedMetadata() (executor.SafeSummary, error) {
	fields := []executor.SafeField{
		{Path: "address", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: record.Address},
		{Path: "armed", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: rollbackBoolPointer(record.Armed)},
		{Path: "artifact_digest", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: record.ArtifactDigest},
		{Path: "attempt", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: rollbackIntPointer(record.Attempt)},
		{Path: "created_at", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: record.CreatedAt.UTC().Format(time.RFC3339Nano)},
		{Path: "lifecycle", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: string(record.State)},
		{Path: "payload_available", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: rollbackBoolPointer(record.PayloadAvailable)},
		{Path: "sensitive", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: rollbackBoolPointer(record.Sensitive)},
		{Path: "successful", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: rollbackBoolPointer(record.Successful)},
		{Path: "version", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: rollbackIntPointer(record.Version)},
	}
	if !record.ExpiresAt.IsZero() {
		fields = append(fields, executor.SafeField{
			Path: "expires_at", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata,
			Text: record.ExpiresAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return executor.NewSafeSummary(fields)
}

// MarshalJSON prevents generic JSON sinks from falling back to the raw Go
// structure when recording rollback metadata.
func (record RecordInfo) MarshalJSON() ([]byte, error) {
	classified, err := record.ClassifiedMetadata()
	if err != nil {
		return nil, err
	}
	return json.Marshal(classified)
}

func rollbackIntPointer(value int) *int    { return &value }
func rollbackBoolPointer(value bool) *bool { return &value }

type storedRecord struct {
	key              recordKey
	meta             metadata
	dir              string
	payloadAvailable bool
}

// Cleanup applies deterministic age, sensitivity, successful-state, attempt,
// supersession, and configured-disk retention without evicting armed records.
func (s *Store) Cleanup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupLocked()
}

// Records returns a stable safe view of retained transaction metadata.
func (s *Store) Records(ctx context.Context, address string) ([]RecordInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.readStoredRecordsLocked()
	if err != nil {
		return nil, err
	}
	out := make([]RecordInfo, 0, len(records))
	for _, record := range records {
		if address != "" && record.key.Address != address {
			continue
		}
		out = append(out, record.info())
	}
	return out, nil
}

func (s *Store) applyRetentionLocked() error {
	records, err := s.readStoredRecordsLocked()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	cutoff := now.Add(-s.maxAge)
	for _, record := range records {
		if record.meta.Sensitive && s.sensitivePayloadExpired(record.meta, now) {
			if err := s.transitionAndDropPayloadLocked(record.key, LifecycleExpired); err != nil {
				return err
			}
			continue
		}
		if record.meta.Sensitive && terminalLifecycle(record.meta.State) && record.payloadAvailable {
			if err := s.transitionAndDropPayloadLocked(record.key, record.meta.State); err != nil {
				return err
			}
		}
		if !record.meta.Armed && !record.meta.CreatedAt.After(cutoff) {
			if err := os.RemoveAll(record.dir); err != nil {
				return err
			}
		}
	}

	records, err = s.readStoredRecordsLocked()
	if err != nil {
		return err
	}
	byAddress := groupStoredRecords(records)
	for _, group := range byAddress {
		if err := s.supersedeIncompleteLocked(group); err != nil {
			return err
		}
	}

	records, err = s.readStoredRecordsLocked()
	if err != nil {
		return err
	}
	byAddress = groupStoredRecords(records)
	for _, group := range byAddress {
		if err := s.pruneSuccessfulPayloadsLocked(group); err != nil {
			return err
		}
		if err := s.pruneAttemptMetadataLocked(group); err != nil {
			return err
		}
	}
	return s.pruneToConfiguredLimitLocked(0, recordKey{})
}

func (s *Store) supersedeIncompleteLocked(records []storedRecord) error {
	if len(records) < 2 {
		return nil
	}
	sortNewest(records)
	newest := records[0].key
	for _, record := range records {
		if record.meta.Armed || record.meta.Successful || terminalLifecycle(record.meta.State) {
			continue
		}
		if record.key == newest {
			continue
		}
		if err := s.transitionAndDropPayloadLocked(record.key, LifecycleSuperseded); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pruneSuccessfulPayloadsLocked(records []storedRecord) error {
	eligible := make([]storedRecord, 0, len(records))
	for _, record := range records {
		if record.meta.Successful && !record.meta.Sensitive && !record.meta.Armed && record.payloadAvailable {
			eligible = append(eligible, record)
		}
	}
	sortNewest(eligible)
	if len(eligible) <= maxSuccessfulPriorStates {
		return nil
	}
	for _, record := range eligible[maxSuccessfulPriorStates:] {
		if err := s.transitionAndDropPayloadLocked(record.key, LifecycleSuperseded); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pruneAttemptMetadataLocked(records []storedRecord) error {
	if len(records) <= s.maxAttempts {
		return nil
	}
	sortOldest(records)
	remaining := len(records)
	for _, record := range records {
		if remaining <= s.maxAttempts {
			break
		}
		if record.meta.Armed {
			continue
		}
		if err := os.RemoveAll(record.dir); err != nil {
			return err
		}
		remaining--
	}
	return nil
}

func (s *Store) pruneToConfiguredLimitLocked(required int64, replacing recordKey) error {
	for {
		used, err := s.configuredUsageLocked(replacing)
		if err != nil {
			return err
		}
		reserved := s.reservedBytesLocked(replacing)
		if used+reserved+required <= s.maxBytes {
			return nil
		}
		records, err := s.readStoredRecordsLocked()
		if err != nil {
			return err
		}
		sortOldest(records)
		pruned := false
		for _, record := range records {
			if record.meta.Armed || record.key == replacing {
				continue
			}
			if err := os.RemoveAll(record.dir); err != nil {
				return err
			}
			pruned = true
			break
		}
		if !pruned {
			return nil
		}
	}
}

func (s *Store) transitionAndDropPayloadLocked(key recordKey, state Lifecycle) error {
	dir := s.recordDir(key.Address, key.ArtifactDigest, key.Attempt)
	meta, payload, err := s.readEnvelope(key)
	if err != nil {
		return err
	}
	clear(payload)
	meta.Version = RecordVersion
	meta.State = state
	meta.Armed = false
	encoded, err := s.sealEnvelope(meta, nil, false)
	if err != nil {
		return err
	}
	if err := s.writeEnvelopeAtomic(dir, encoded); err != nil {
		return err
	}
	delete(s.armed, key)
	return nil
}

func (s *Store) readStoredRecordsLocked() ([]storedRecord, error) {
	root := filepath.Join(s.root, "records")
	records := []storedRecord{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || info.Name() != envelopeFilename {
			return walkErr
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		meta, payload, err := s.openEnvelope(raw)
		if err != nil {
			return err
		}
		clear(payload)
		key := recordKey{Address: meta.Address, ArtifactDigest: meta.ArtifactDigest, Attempt: meta.Attempt}
		dir := filepath.Dir(path)
		if path != s.envelopePath(key) {
			return errors.New("rollback metadata path does not match record identity")
		}
		records = append(records, storedRecord{
			key: key, meta: meta, dir: dir, payloadAvailable: meta.PayloadPresent,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortOldest(records)
	return records, nil
}

func (s *Store) sensitivePayloadExpired(meta metadata, now time.Time) bool {
	if !meta.ExpiresAt.After(now) {
		return true
	}
	return !meta.CreatedAt.Add(MaxSensitiveRetention).After(now)
}

func terminalLifecycle(state Lifecycle) bool {
	switch state {
	case LifecycleAcknowledged, LifecycleRolledBack, LifecycleExpired,
		LifecycleSuperseded, LifecycleAbandoned:
		return true
	default:
		return false
	}
}

func groupStoredRecords(records []storedRecord) map[string][]storedRecord {
	groups := make(map[string][]storedRecord)
	for _, record := range records {
		groups[record.key.Address] = append(groups[record.key.Address], record)
	}
	return groups
}

func sortOldest(records []storedRecord) {
	sort.Slice(records, func(i, j int) bool {
		if !records[i].meta.CreatedAt.Equal(records[j].meta.CreatedAt) {
			return records[i].meta.CreatedAt.Before(records[j].meta.CreatedAt)
		}
		if records[i].key.Attempt != records[j].key.Attempt {
			return records[i].key.Attempt < records[j].key.Attempt
		}
		return records[i].key.ArtifactDigest < records[j].key.ArtifactDigest
	})
}

func sortNewest(records []storedRecord) {
	sortOldest(records)
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
}

func (record storedRecord) info() RecordInfo {
	return RecordInfo{
		Version: record.meta.Version, State: record.meta.State,
		Address: record.key.Address, ArtifactDigest: record.key.ArtifactDigest, Attempt: record.key.Attempt,
		CreatedAt: record.meta.CreatedAt, Armed: record.meta.Armed, Sensitive: record.meta.Sensitive,
		Successful: record.meta.Successful, ExpiresAt: record.meta.ExpiresAt,
		PayloadAvailable: record.payloadAvailable,
	}
}
