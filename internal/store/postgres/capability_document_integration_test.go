//go:build postgres

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DavidHoenisch/remotr/internal/agent/inventory"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func TestPostgresCapabilityDocumentEqualContentIssuesNoRowUpdate(t *testing.T) {
	databaseURL := os.Getenv("REMOTR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("REMOTR_TEST_DATABASE_URL is required for Postgres integration evidence")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	for _, statement := range []string{
		`CREATE TEMP TABLE endpoints (LIKE public.endpoints INCLUDING ALL) ON COMMIT DROP`,
		`CREATE TEMP TABLE endpoint_capability_documents (LIKE public.endpoint_capability_documents INCLUDING ALL) ON COMMIT DROP`,
		`INSERT INTO endpoints (id, fleet) VALUES ('11111111-1111-1111-1111-111111111111', 'engineering')`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	store := NewFromQueries(db.New(tx))
	first := validCapabilityDocumentRecord(t, "11111111-1111-1111-1111-111111111111", time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC))
	if changed, err := store.StoreEndpointCapabilityDocument(ctx, first); err != nil || !changed {
		t.Fatalf("initial store changed=%t err=%v", changed, err)
	}
	var beforeXID string
	if err := tx.QueryRow(ctx, `SELECT xmin::text FROM endpoint_capability_documents WHERE endpoint_id = $1`, first.EndpointID).Scan(&beforeXID); err != nil {
		t.Fatal(err)
	}
	equal := first
	equal.ReceivedAt = first.ReceivedAt.Add(time.Hour)
	if changed, err := store.StoreEndpointCapabilityDocument(ctx, equal); err != nil || changed {
		t.Fatalf("equal store changed=%t err=%v", changed, err)
	}
	var afterXID string
	var receivedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT xmin::text, received_at FROM endpoint_capability_documents WHERE endpoint_id = $1`, first.EndpointID).Scan(&afterXID, &receivedAt); err != nil {
		t.Fatal(err)
	}
	if afterXID != beforeXID || !receivedAt.Equal(first.ReceivedAt) {
		t.Fatalf("equal content rewrote row: xmin %s -> %s, received_at %s -> %s", beforeXID, afterXID, first.ReceivedAt, receivedAt)
	}
}

func TestPostgresSystemInformationEqualContentIssuesNoRowUpdate(t *testing.T) {
	databaseURL := os.Getenv("REMOTR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("REMOTR_TEST_DATABASE_URL is required for Postgres integration evidence")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	for _, statement := range []string{
		`CREATE TEMP TABLE endpoints (LIKE public.endpoints INCLUDING ALL) ON COMMIT DROP`,
		`CREATE TEMP TABLE endpoint_system_info (LIKE public.endpoint_system_info INCLUDING ALL) ON COMMIT DROP`,
		`INSERT INTO endpoints (id, fleet) VALUES ('11111111-1111-1111-1111-111111111111', 'engineering')`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	store := NewFromQueries(db.New(tx))
	endpointID := "11111111-1111-1111-1111-111111111111"
	snapshot := inventory.Snapshot{
		OSRelease: inventory.OSReleaseInfo{Name: "Pop!_OS", ID: "pop"},
		CPU:       inventory.CPUInfo{ModelName: "Test CPU"},
	}
	report, err := inventory.MarshalJSON(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := inventory.Digest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := store.UpsertEndpointSystemInfo(ctx, endpointID, digest, report); err != nil || !changed {
		t.Fatalf("initial store changed=%t err=%v", changed, err)
	}
	var beforeXID string
	var beforeReported time.Time
	if err := tx.QueryRow(ctx, `SELECT xmin::text, reported_at FROM endpoint_system_info WHERE endpoint_id = $1`, endpointID).Scan(&beforeXID, &beforeReported); err != nil {
		t.Fatal(err)
	}
	equal := append([]byte(nil), report...)
	if changed, err := store.UpsertEndpointSystemInfo(ctx, endpointID, digest, equal); err != nil || changed {
		t.Fatalf("equal store changed=%t err=%v", changed, err)
	}
	var afterXID string
	var afterReported time.Time
	if err := tx.QueryRow(ctx, `SELECT xmin::text, reported_at FROM endpoint_system_info WHERE endpoint_id = $1`, endpointID).Scan(&afterXID, &afterReported); err != nil {
		t.Fatal(err)
	}
	if beforeXID != afterXID || !beforeReported.Equal(afterReported) {
		t.Fatalf("equal content rewrote row: xmin %s -> %s, reported_at %s -> %s", beforeXID, afterXID, beforeReported, afterReported)
	}
}

func TestPostgresDeliveryTimestampReplayIssuesNoRowUpdate(t *testing.T) {
	databaseURL := os.Getenv("REMOTR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("REMOTR_TEST_DATABASE_URL is required for Postgres integration evidence")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	for _, statement := range []string{
		`CREATE TEMP TABLE endpoints (LIKE public.endpoints INCLUDING ALL) ON COMMIT DROP`,
		`CREATE TEMP TABLE endpoint_delivery_states (LIKE public.endpoint_delivery_states INCLUDING ALL) ON COMMIT DROP`,
		`INSERT INTO endpoints (id, fleet) VALUES ('11111111-1111-1111-1111-111111111111', 'engineering')`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	store := NewFromDeliveryStateQueries(db.New(tx))
	first := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	state := registry.EndpointDeliveryState{
		EndpointID: "11111111-1111-1111-1111-111111111111", TargetReleaseRef: "release-target",
		OfferedReleaseRef: "release-target", OfferedDigest: "digest-target", OfferedSchemaVersion: 1, OfferedAt: first,
		ActiveReleaseRef: "release-active", ActiveDigest: "digest-active", ActiveSchemaVersion: 1, ActiveAt: first,
	}
	if changed, err := store.StoreEndpointDeliveryState(ctx, state); err != nil || !changed {
		t.Fatalf("initial store changed=%t err=%v", changed, err)
	}
	var beforeXID string
	var beforeUpdated time.Time
	if err := tx.QueryRow(ctx, `SELECT xmin::text, updated_at FROM endpoint_delivery_states WHERE endpoint_id = $1`, state.EndpointID).Scan(&beforeXID, &beforeUpdated); err != nil {
		t.Fatal(err)
	}
	state.OfferedAt, state.ActiveAt = first.Add(time.Hour), first.Add(time.Hour)
	if changed, err := store.StoreEndpointDeliveryState(ctx, state); err != nil || changed {
		t.Fatalf("timestamp replay changed=%t err=%v", changed, err)
	}
	var afterXID string
	var afterUpdated time.Time
	if err := tx.QueryRow(ctx, `SELECT xmin::text, updated_at FROM endpoint_delivery_states WHERE endpoint_id = $1`, state.EndpointID).Scan(&afterXID, &afterUpdated); err != nil {
		t.Fatal(err)
	}
	if beforeXID != afterXID || !beforeUpdated.Equal(afterUpdated) {
		t.Fatalf("timestamp replay rewrote row: xmin %s -> %s, updated_at %s -> %s", beforeXID, afterXID, beforeUpdated, afterUpdated)
	}
}

func TestPostgresTargetingEqualContentIssuesNoRowUpdate(t *testing.T) {
	databaseURL := os.Getenv("REMOTR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("REMOTR_TEST_DATABASE_URL is required for Postgres integration evidence")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	for _, statement := range []string{
		`CREATE TEMP TABLE endpoints (LIKE public.endpoints INCLUDING ALL) ON COMMIT DROP`,
		`CREATE TEMP TABLE endpoint_labels (LIKE public.endpoint_labels INCLUDING ALL) ON COMMIT DROP`,
		`INSERT INTO endpoints (id, fleet) VALUES ('11111111-1111-1111-1111-111111111111', 'engineering')`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	store := NewFromQueries(db.New(tx))
	endpointID := "11111111-1111-1111-1111-111111111111"
	labels := map[string]string{"distro": "ubuntu", "arch": "x86"}
	if changed, err := store.StoreEndpointTargeting(ctx, endpointID, labels, []string{"bob", "alice"}); err != nil || !changed {
		t.Fatalf("initial targeting store changed=%t err=%v", changed, err)
	}
	var beforeEndpointXID, beforeLabelXID string
	if err := tx.QueryRow(ctx, `SELECT xmin::text FROM endpoints WHERE id = $1`, endpointID).Scan(&beforeEndpointXID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT xmin::text FROM endpoint_labels WHERE endpoint_id = $1 AND key = 'arch'`, endpointID).Scan(&beforeLabelXID); err != nil {
		t.Fatal(err)
	}
	if changed, err := store.StoreEndpointTargeting(ctx, endpointID, map[string]string{"arch": "x86", "distro": "ubuntu"}, []string{"alice", "bob"}); err != nil || changed {
		t.Fatalf("equal targeting store changed=%t err=%v", changed, err)
	}
	var afterEndpointXID, afterLabelXID string
	if err := tx.QueryRow(ctx, `SELECT xmin::text FROM endpoints WHERE id = $1`, endpointID).Scan(&afterEndpointXID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT xmin::text FROM endpoint_labels WHERE endpoint_id = $1 AND key = 'arch'`, endpointID).Scan(&afterLabelXID); err != nil {
		t.Fatal(err)
	}
	if beforeEndpointXID != afterEndpointXID || beforeLabelXID != afterLabelXID {
		t.Fatalf("equal targeting rewrote rows: endpoint %s -> %s label %s -> %s", beforeEndpointXID, afterEndpointXID, beforeLabelXID, afterLabelXID)
	}
	if changed, err := store.StoreEndpointTargeting(ctx, endpointID, map[string]string{"arch": "arm"}, nil); err != nil || !changed {
		t.Fatalf("changed targeting store changed=%t err=%v", changed, err)
	}
	var usernameValid bool
	var labelCount int
	if err := tx.QueryRow(ctx, `SELECT reported_usernames IS NOT NULL FROM endpoints WHERE id = $1`, endpointID).Scan(&usernameValid); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM endpoint_labels WHERE endpoint_id = $1`, endpointID).Scan(&labelCount); err != nil {
		t.Fatal(err)
	}
	if usernameValid || labelCount != 1 {
		t.Fatalf("changed targeting did not replace complete document: username_valid=%t labels=%d", usernameValid, labelCount)
	}
}
