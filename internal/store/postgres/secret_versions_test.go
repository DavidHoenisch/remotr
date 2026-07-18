package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func TestStorePersistsEncryptedSecretEnvelopeWithoutExternalKEK(t *testing.T) {
	queries := &recordingSecretQueries{}
	store := NewFromSecretQueries(queries)
	key := bytes.Repeat([]byte{0xd1}, 32)
	keyring, err := secrets.NewKeyring("kek-storage", map[string][]byte{"kek-storage": key})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := secrets.NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	service, err := secrets.NewRegistryService(store, envelope, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("postgres-secret-canary")
	if _, err := service.Upload(context.Background(), secrets.UploadRequest{Name: "database/password", Fleet: "production", Material: plaintext, ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(queries.created.EnvelopeJson, plaintext) || bytes.Contains(queries.created.EnvelopeJson, key) {
		t.Fatal("database envelope exposed plaintext or external KEK")
	}
	var record secrets.EncryptedRecord
	if err := json.Unmarshal(queries.created.EnvelopeJson, &record); err != nil {
		t.Fatal(err)
	}
	if record.KEKProvider != secrets.StaticKeyProviderID || record.KEKID != "kek-storage" || len(record.Ciphertext) == 0 || len(record.WrappedDEK) == 0 {
		t.Fatalf("stored envelope = %#v", record)
	}
	queries.backupEnvelopes = [][]byte{queries.created.EnvelopeJson}
	restored, err := store.ListEncryptedSecretRecords(t.Context())
	if err != nil || len(restored) != 1 {
		t.Fatalf("restored encrypted records = %+v, %v", restored, err)
	}
	if report, err := envelope.CheckKeyCoverage(t.Context(), restored); err != nil || !report.Complete {
		t.Fatalf("restored key coverage = %+v, %v", report, err)
	}

	const malformedCanary = "malformed-restored-secret-canary"
	queries.backupEnvelopes = [][]byte{[]byte(`{"ciphertext":"` + malformedCanary + `"`)}
	if _, err := store.ListEncryptedSecretRecords(t.Context()); err == nil {
		t.Fatal("malformed restored encrypted record was accepted")
	} else if bytes.Contains([]byte(err.Error()), []byte(malformedCanary)) {
		t.Fatalf("malformed restore diagnostic leaked database bytes: %v", err)
	}
}

type recordingSecretQueries struct {
	created         db.CreateSecretVersionParams
	backupEnvelopes [][]byte
}

func (*recordingSecretQueries) AllocateSecretVersion(context.Context, string) (int64, error) {
	return 1, nil
}
func (q *recordingSecretQueries) CreateSecretVersion(_ context.Context, params db.CreateSecretVersionParams) error {
	q.created = params
	return nil
}
func (*recordingSecretQueries) GetExactSecretVersion(context.Context, db.GetExactSecretVersionParams) (db.GetExactSecretVersionRow, error) {
	return db.GetExactSecretVersionRow{}, pgx.ErrNoRows
}
func (*recordingSecretQueries) GetActiveSecretVersion(context.Context, string) (db.GetActiveSecretVersionRow, error) {
	return db.GetActiveSecretVersionRow{}, pgx.ErrNoRows
}
func (*recordingSecretQueries) ListSecretVersions(context.Context, string) ([]db.ListSecretVersionsRow, error) {
	return nil, nil
}
func (q *recordingSecretQueries) ListSecretVersionEnvelopes(context.Context) ([][]byte, error) {
	return q.backupEnvelopes, nil
}
func (*recordingSecretQueries) GetSecretActivationGeneration(context.Context, string) (int64, error) {
	return 0, pgx.ErrNoRows
}
func (*recordingSecretQueries) ActivateSecretVersion(context.Context, db.ActivateSecretVersionParams) (db.ActivateSecretVersionRow, error) {
	return db.ActivateSecretVersionRow{}, pgx.ErrNoRows
}
func (*recordingSecretQueries) RevokeSecretVersion(context.Context, db.RevokeSecretVersionParams) (db.RevokeSecretVersionRow, error) {
	return db.RevokeSecretVersionRow{}, pgx.ErrNoRows
}
func (*recordingSecretQueries) CreateSecretRollbackReference(context.Context, db.CreateSecretRollbackReferenceParams) error {
	return nil
}
func (*recordingSecretQueries) ListActiveSecretRollbackReferences(context.Context, db.ListActiveSecretRollbackReferencesParams) ([]db.SecretRollbackReference, error) {
	return nil, nil
}
func (*recordingSecretQueries) AbandonSecretRollbackReferences(context.Context, db.AbandonSecretRollbackReferencesParams) error {
	return nil
}
func (*recordingSecretQueries) DeleteSecretVersion(context.Context, db.DeleteSecretVersionParams) (int64, error) {
	return 0, pgx.ErrNoRows
}
