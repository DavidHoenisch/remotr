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
}

type recordingSecretQueries struct {
	created db.CreateSecretVersionParams
}

func (*recordingSecretQueries) AllocateSecretVersion(context.Context, string) (int64, error) {
	return 1, nil
}
func (q *recordingSecretQueries) CreateSecretVersion(_ context.Context, params db.CreateSecretVersionParams) error {
	q.created = params
	return nil
}
func (*recordingSecretQueries) GetExactSecretVersion(context.Context, db.GetExactSecretVersionParams) (db.SecretVersion, error) {
	return db.SecretVersion{}, pgx.ErrNoRows
}
func (*recordingSecretQueries) GetActiveSecretVersion(context.Context, string) (db.SecretVersion, error) {
	return db.SecretVersion{}, pgx.ErrNoRows
}
func (*recordingSecretQueries) ListSecretVersions(context.Context, string) ([]db.SecretVersion, error) {
	return nil, nil
}
func (*recordingSecretQueries) GetSecretActivationGeneration(context.Context, string) (int64, error) {
	return 0, pgx.ErrNoRows
}
func (*recordingSecretQueries) ActivateSecretVersion(context.Context, db.ActivateSecretVersionParams) (db.SecretVersion, error) {
	return db.SecretVersion{}, pgx.ErrNoRows
}
func (*recordingSecretQueries) RevokeSecretVersion(context.Context, db.RevokeSecretVersionParams) (db.SecretVersion, error) {
	return db.SecretVersion{}, pgx.ErrNoRows
}
