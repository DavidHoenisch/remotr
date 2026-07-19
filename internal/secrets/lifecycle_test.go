package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

type failingKeyCoverageProvider struct {
	KeyEncryptionProvider
	err error
}

func (provider failingKeyCoverageProvider) KeyAvailable(context.Context, string, string) (bool, error) {
	return false, provider.err
}

func TestRoutineRewrapPreservesCiphertextAndMigratesDEKToActiveKEK(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x11}, 32)
	oldKeyring, err := NewKeyring("kek-old", map[string][]byte{"kek-old": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	oldEnvelope, err := NewEnvelope(oldKeyring)
	if err != nil {
		t.Fatal(err)
	}
	record, err := oldEnvelope.Encrypt(ScopeMetadata{Name: "database/password", Version: "2"}, []byte("rotation-canary"))
	if err != nil {
		t.Fatal(err)
	}

	rotatedKeyring, err := NewKeyring("kek-new", map[string][]byte{
		"kek-new": bytes.Repeat([]byte{0x22}, 32),
		"kek-old": oldKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	rotatedEnvelope, err := NewEnvelope(rotatedKeyring)
	if err != nil {
		t.Fatal(err)
	}
	rewrapped, err := rotatedEnvelope.Rewrap(record)
	if err != nil {
		t.Fatal(err)
	}

	if rewrapped.KEKID != "kek-new" {
		t.Fatalf("KEK ID = %q", rewrapped.KEKID)
	}
	if !bytes.Equal(rewrapped.Ciphertext, record.Ciphertext) || !bytes.Equal(rewrapped.CipherNonce, record.CipherNonce) || rewrapped.Fingerprint != record.Fingerprint {
		t.Fatal("routine rotation changed secret ciphertext")
	}
	if bytes.Equal(rewrapped.WrappedDEK, record.WrappedDEK) || bytes.Equal(rewrapped.WrapMetadata, record.WrapMetadata) {
		t.Fatal("routine rotation did not independently rewrap the DEK")
	}
	plaintext, err := rotatedEnvelope.Decrypt(rewrapped)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "rotation-canary" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	if _, err := oldEnvelope.Decrypt(rewrapped); err == nil {
		t.Fatal("record remained decryptable without the new active KEK")
	}
}

func TestCompromiseRekeyReencryptsEveryVersionAndRecordsSecurityEvent(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x31}, 32)
	oldKeyring, err := NewKeyring("kek-compromised", map[string][]byte{"kek-compromised": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	oldEnvelope, err := NewEnvelope(oldKeyring)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]EncryptedRecord, 0, 2)
	for _, version := range []string{"7", "8"} {
		record, err := oldEnvelope.Encrypt(ScopeMetadata{Name: "vpn/private-key", Version: version, Fleet: "production"}, []byte("compromise-canary"))
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}

	newKeyring, err := NewKeyring("kek-recovery", map[string][]byte{
		"kek-recovery":    bytes.Repeat([]byte{0x41}, 32),
		"kek-compromised": oldKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	newEnvelope, err := NewEnvelope(newKeyring)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingSecurityEventSink{}
	rekeyed, err := newEnvelope.CompromiseRekey(context.Background(), records, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if len(rekeyed) != len(records) {
		t.Fatalf("rekeyed records = %d", len(rekeyed))
	}
	for i := range records {
		if rekeyed[i].KEKID != "kek-recovery" || bytes.Equal(rekeyed[i].Ciphertext, records[i].Ciphertext) || bytes.Equal(rekeyed[i].CipherNonce, records[i].CipherNonce) || bytes.Equal(rekeyed[i].WrappedDEK, records[i].WrappedDEK) {
			t.Fatalf("record %d was not fully re-encrypted", i)
		}
		plaintext, err := newEnvelope.Decrypt(rekeyed[i])
		if err != nil {
			t.Fatal(err)
		}
		if string(plaintext) != "compromise-canary" {
			t.Fatalf("record %d plaintext = %q", i, plaintext)
		}
	}
	if len(recorder.events) != 1 {
		t.Fatalf("security events = %d", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Action != SecurityEventCompromiseRekey || event.ActiveKEKID != "kek-recovery" || len(event.Versions) != 2 {
		t.Fatalf("security event = %#v", event)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("compromise-canary")) {
		t.Fatal("security event exposed secret material")
	}
}

func TestCompromiseRekeyFailsClosedWhenSecurityEventCannotBeRecorded(t *testing.T) {
	keyring, err := NewKeyring("kek-1", map[string][]byte{"kek-1": bytes.Repeat([]byte{0x71}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	record, err := envelope.Encrypt(ScopeMetadata{Name: "service/token", Version: "1"}, []byte("audit-canary"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := envelope.CompromiseRekey(context.Background(), []EncryptedRecord{record}, failingSecurityEventSink{})
	if err == nil || got != nil {
		t.Fatalf("rekeyed=%v err=%v", got, err)
	}
}

func TestKeyCoverageAndRemovalProtectionIdentifyReferencedHistoricalKEKs(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x51}, 32)
	newKey := bytes.Repeat([]byte{0x61}, 32)
	keyring, err := NewKeyring("kek-new", map[string][]byte{"kek-old": oldKey, "kek-new": newKey})
	if err != nil {
		t.Fatal(err)
	}
	oldOnly, err := NewKeyring("kek-old", map[string][]byte{"kek-old": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	oldEnvelope, err := NewEnvelope(oldOnly)
	if err != nil {
		t.Fatal(err)
	}
	record, err := oldEnvelope.Encrypt(ScopeMetadata{Name: "archive/signing-key", Version: "3", Fleet: "finance"}, []byte("coverage-canary"))
	if err != nil {
		t.Fatal(err)
	}

	newOnly, err := NewKeyring("kek-new", map[string][]byte{"kek-new": newKey})
	if err != nil {
		t.Fatal(err)
	}
	report, err := CheckKeyCoverage(context.Background(), []EncryptedRecord{record}, newOnly)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || len(report.Missing) != 1 {
		t.Fatalf("coverage report = %#v", report)
	}
	missing := report.Missing[0]
	if missing.ProviderID != StaticKeyProviderID || missing.KEKID != "kek-old" || missing.Name != "archive/signing-key" || missing.Version != "3" {
		t.Fatalf("missing coverage = %#v", missing)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, oldKey) || bytes.Contains(encoded, []byte("coverage-canary")) {
		t.Fatal("coverage diagnostic exposed key or secret material")
	}
	var wire struct {
		Missing []json.RawMessage `json:"missing"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil || len(wire.Missing) != 1 {
		t.Fatalf("coverage report wire shape = %s, %v", encoded, err)
	}
	var classified executor.SafeSummary
	if err := json.Unmarshal(wire.Missing[0], &classified); err != nil {
		t.Fatalf("coverage gap is not classified: %v", err)
	}
	if err := classified.Validate(); err != nil || len(classified.Fields) == 0 {
		t.Fatalf("classified coverage gap = %+v, %v", classified, err)
	}
	newEnvelope, err := NewEnvelope(newOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newEnvelope.Decrypt(record); err == nil {
		t.Fatal("record resolved without its historical KEK")
	} else if !strings.Contains(err.Error(), "kek-old") || !strings.Contains(err.Error(), "archive/signing-key") || !strings.Contains(err.Error(), "3") {
		t.Fatalf("missing-key diagnostic = %q", err)
	}

	if _, err := keyring.Without("kek-old", []EncryptedRecord{record}); err == nil {
		t.Fatal("referenced historical KEK was removed")
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	rewrapped, err := envelope.Rewrap(record)
	if err != nil {
		t.Fatal(err)
	}
	pruned, err := keyring.Without("kek-old", []EncryptedRecord{rewrapped})
	if err != nil {
		t.Fatalf("remove unreferenced historical KEK: %v", err)
	}
	if pruned.Has("kek-old") {
		t.Fatal("unreferenced historical KEK remains installed")
	}
	if _, err := keyring.Without("kek-new", nil); err == nil {
		t.Fatal("active KEK was removed")
	}
}

func TestKeyCoverageClassifiesProviderFailure(t *testing.T) {
	const canary = "key-coverage-provider-secret-canary"
	keyring, err := NewKeyring("kek-1", map[string][]byte{"kek-1": bytes.Repeat([]byte{0x71}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	record, err := envelope.Encrypt(ScopeMetadata{Name: "service/token", Version: "1", Fleet: "production"}, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckKeyCoverage(t.Context(), []EncryptedRecord{record}, failingKeyCoverageProvider{
		KeyEncryptionProvider: keyring,
		err:                   errors.New(canary),
	})
	if err == nil {
		t.Fatal("provider key-coverage failure was accepted")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("key-coverage failure leaked provider error: %v", err)
	}
	var safe executor.SafeError
	if !errors.As(err, &safe) {
		t.Fatalf("key-coverage failure = %T, want classified SafeError", err)
	}
	if len(safe.Details.Fields) == 0 || !strings.Contains(safe.Details.String(), "secret_name=service/token") {
		t.Fatalf("key-coverage failure omitted classified record details: %#v", safe.Details)
	}
}

func TestKeyCoverageFailsClosedForInvalidRecordsAndMissingProvider(t *testing.T) {
	invalid := EncryptedRecord{}
	if _, err := CheckKeyCoverage(t.Context(), []EncryptedRecord{invalid}, nil); err == nil {
		t.Fatal("invalid restored encrypted record was accepted")
	}

	keyring, err := NewKeyring("kek-1", map[string][]byte{"kek-1": bytes.Repeat([]byte{0x71}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	record, err := envelope.Encrypt(ScopeMetadata{Name: "service/token", Version: "1"}, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := CheckKeyCoverage(t.Context(), []EncryptedRecord{record}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || len(report.Missing) != 1 || report.Missing[0].Name != "service/token" {
		t.Fatalf("missing-provider coverage report = %#v", report)
	}
}

func TestKeyCoverageGapClassifiesOptionalRecoveryMetadataExactly(t *testing.T) {
	gap := KeyCoverageGap{
		ProviderID: "provider-1",
		KEKID:      "kek-7",
		Name:       "database/password",
		Version:    "3",
		Fleet:      "production",
		EndpointID: "11111111-1111-1111-1111-111111111111",
	}
	encoded, err := json.Marshal(gap)
	if err != nil {
		t.Fatal(err)
	}
	var summary executor.SafeSummary
	if err := json.Unmarshal(encoded, &summary); err != nil {
		t.Fatal(err)
	}
	fields := make(map[string]executor.SafeField, len(summary.Fields))
	for _, field := range summary.Fields {
		fields[field.Path] = field
	}
	want := map[string]executor.SafeField{
		"endpoint_id":    {Path: "endpoint_id", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.EndpointID},
		"fleet":          {Path: "fleet", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.Fleet},
		"kek_id":         {Path: "kek_id", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.KEKID},
		"kek_provider":   {Path: "kek_provider", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: gap.ProviderID},
		"secret_name":    {Path: "secret_name", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: gap.Name},
		"secret_version": {Path: "secret_version", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: gap.Version},
	}
	if len(fields) != len(want) {
		t.Fatalf("classified recovery fields = %#v", fields)
	}
	for path, expected := range want {
		if fields[path] != expected {
			t.Fatalf("classified recovery field %q = %#v, want %#v", path, fields[path], expected)
		}
	}

	withoutOptional, err := (KeyCoverageGap{
		ProviderID: gap.ProviderID, KEKID: gap.KEKID, Name: gap.Name, Version: gap.Version,
	}).ClassifiedMetadata()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range withoutOptional.Fields {
		if field.Path == "fleet" || field.Path == "endpoint_id" {
			t.Fatalf("empty optional recovery metadata was projected: %#v", field)
		}
	}
}

type recordingSecurityEventSink struct {
	events []SecretSecurityEvent
}

func (s *recordingSecurityEventSink) RecordSecretSecurityEvent(_ context.Context, event SecretSecurityEvent) error {
	s.events = append(s.events, event)
	return nil
}

type failingSecurityEventSink struct{}

func (failingSecurityEventSink) RecordSecretSecurityEvent(context.Context, SecretSecurityEvent) error {
	return errors.New("audit unavailable")
}
