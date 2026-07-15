package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// BenchmarkChangeControlStateRoundTrip400Endpoints measures the real JSONB
// compare-and-swap path used by one durable Change-control mutation. It uses a
// transaction-local table and leaves the controlled database unchanged.
func BenchmarkChangeControlStateRoundTrip400Endpoints(b *testing.B) {
	databaseURL := os.Getenv("REMOTR_BENCH_DATABASE_URL")
	if databaseURL == "" {
		b.Fatal("REMOTR_BENCH_DATABASE_URL is required for the Postgres benchmark")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE change_control_state (
singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
state_json JSONB NOT NULL,
revision BIGINT NOT NULL CHECK (revision > 0),
updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
) ON COMMIT DROP`); err != nil {
		b.Fatal(err)
	}

	payload := benchmarkChangeControlPayload(b, 400)
	store := NewFromChangeControlQueries(db.New(tx))
	revision := int64(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, err := store.SaveChangeControlState(ctx, revision, payload)
		if err != nil {
			b.Fatal(err)
		}
		loaded, loadedRevision, err := store.LoadChangeControlState(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if loadedRevision != next || len(loaded) == 0 {
			b.Fatalf("round trip revision=%d loaded_revision=%d payload_bytes=%d", next, loadedRevision, len(loaded))
		}
		revision = next
	}
	b.StopTimer()
	b.ReportMetric(float64(len(payload)), "input_bytes")
	var storageBytes int64
	if err := tx.QueryRow(ctx, `SELECT pg_column_size(state_json) FROM change_control_state WHERE singleton = TRUE`).Scan(&storageBytes); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(storageBytes), "storage_bytes")
}

func benchmarkChangeControlPayload(b *testing.B, endpoints int) []byte {
	b.Helper()
	targets := make([]map[string]any, endpoints)
	leases := make(map[string]any, endpoints)
	attempts := make(map[string]int, endpoints)
	for i := 0; i < endpoints; i++ {
		endpointID := fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
		targets[i] = map[string]any{"endpoint_id": endpointID, "compatible": true, "preflight_ready": true}
		leaseID := fmt.Sprintf("lease-%04d", i)
		leases[leaseID] = map[string]any{
			"id": leaseID, "change_request_id": "request-1", "endpoint_id": endpointID,
			"resource_hashes": map[string]string{"base/firewall": "sha256:firewall"},
			"attempt":         1, "issued_at": time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC),
			"expires_at": time.Date(2026, 7, 11, 20, 5, 0, 0, time.UTC),
			"risk":       "connectivity", "progress": "lease-issued",
		}
		attempts["9:request-1"+endpointID] = 1
	}
	payload, err := json.Marshal(map[string]any{
		"version": 1,
		"requests": map[string]any{
			"request-1": map[string]any{
				"id": "request-1", "fleet": "engineering", "release_ref": "release-1",
				"artifact_digest": "sha256:artifact", "authorization_group": "network",
				"risk": "connectivity", "frozen_targets": targets, "authorization_state": "authorized",
			},
		},
		"rollouts": map[string]any{}, "baselines": map[string]any{},
		"policy": map[string]any{}, "automatic_promotion": map[string]any{},
		"leases": leases, "attempts": attempts, "break_glass": map[string]any{},
	})
	if err != nil {
		b.Fatal(err)
	}
	return payload
}
