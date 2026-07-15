//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/secrets"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
	"github.com/DavidHoenisch/remotr/test/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OS-LSM-028, OS-AEC-072: durable rollback references protect an encrypted
// prior version from deletion until explicitly authorized abandonment.
func TestSecretRecoveryReferenceProtectsPostgresVersionUntilAuthorizedAbandonment(t *testing.T) {
	ctx := testsupport.Context(t, 30*time.Second)
	pool, err := pgxpool.New(ctx, envOr("REMOTR_E2E_DATABASE_URL", "postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Postgres unavailable: %v", err)
	}
	store := pgstore.NewFromPool(pool)
	name := "recovery/" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM secret_rollback_references WHERE name = $1`, name)
		_, _ = pool.Exec(context.Background(), `UPDATE secret_names SET active_version = NULL WHERE name = $1`, name)
		_, _ = pool.Exec(context.Background(), `DELETE FROM secret_versions WHERE name = $1`, name)
		_, _ = pool.Exec(context.Background(), `DELETE FROM secret_names WHERE name = $1`, name)
	})
	keyring, err := secrets.NewKeyring("kek-e2e", map[string][]byte{"kek-e2e": bytes.Repeat([]byte{0xe2}, 32)})
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
	for _, label := range []string{"postgres-prior", "postgres-replacement"} {
		if _, err := service.Upload(ctx, secrets.UploadRequest{Name: name, Fleet: "production", Material: []byte(testsupport.SecretCanary(label)), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Activate(ctx, secrets.ActivationRequest{Name: name, Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RetainRollbackReference(ctx, secrets.RollbackReferenceRequest{
		Name: name, Version: "1", ResourceAddress: "office/wifi", ArtifactDigest: "sha256:e2e", Attempt: 1, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(ctx, secrets.ActivationRequest{Name: name, Version: "2", ActorID: "operator-2"}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteVersion(ctx, secrets.DeleteVersionRequest{Name: name, Version: "1", ActorID: "operator-2"}); !errors.Is(err, secrets.ErrVersionReferenced) {
		t.Fatalf("protected deletion error = %v", err)
	}
	authorized, err := secrets.NewRegistryService(store, envelope, nil, nil, secrets.WithRecoveryAbandonmentAuthorizer(e2eAbandonAuthorizer{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := authorized.DeleteVersion(ctx, secrets.DeleteVersionRequest{Name: name, Version: "1", ActorID: "recovery-admin", AbandonRecovery: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := authorized.GetMetadata(ctx, name, "1"); !errors.Is(err, secrets.ErrVersionNotFound) {
		t.Fatalf("deleted version lookup error = %v", err)
	}
}

type e2eAbandonAuthorizer struct{}

func (e2eAbandonAuthorizer) AuthorizeRecoveryAbandonment(_ context.Context, request secrets.RecoveryAbandonmentRequest) bool {
	return request.ActorID == "recovery-admin" && len(request.References) == 1
}
