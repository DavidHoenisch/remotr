package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

const postgresBenchmarkFleet = "benchmark-fleet"

// BenchmarkPostgresCompiledArtifactLookup measures the database-backed cache
// read used by authenticated Sync after a Release ref has been composed.
func BenchmarkPostgresCompiledArtifactLookup(b *testing.B) {
	ctx, tx, store := postgresBenchmarkStore(b, "compiled_artifacts")
	artifact := benchmarkfixture.Artifact(benchmarkfixture.ResourceCount(1000))
	if err := store.StoreCompiledArtifactForFleet(ctx, postgresBenchmarkFleet, "release-benchmark", "desired", artifact, "sha256:benchmark"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(artifact)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loaded, digest, err := store.GetCompiledArtifactForFleet(ctx, postgresBenchmarkFleet, "release-benchmark", "desired")
		if err != nil {
			b.Fatal(err)
		}
		if len(loaded) != len(artifact) || digest != "sha256:benchmark" {
			b.Fatalf("compiled artifact bytes=%d digest=%q", len(loaded), digest)
		}
	}
	b.StopTimer()
	var storageBytes int64
	if err := tx.QueryRow(ctx, `SELECT pg_column_size(artifact) FROM compiled_artifacts WHERE fleet_name = $1`, postgresBenchmarkFleet).Scan(&storageBytes); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(storageBytes), "storage_bytes")
}

// BenchmarkPostgresEndpointCheckIn measures the bounded update issued for an
// authenticated endpoint on every successful Sync.
func BenchmarkPostgresEndpointCheckIn(b *testing.B) {
	ctx, _, store := postgresBenchmarkStore(b, "fleet_settings", "endpoints")
	endpointIDs := postgresBenchmarkEndpoints(b, ctx, store, 400)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		endpointID := endpointIDs[i%len(endpointIDs)]
		if err := store.RecordEndpointCheckIn(ctx, endpointID, "release-benchmark", "sha256:benchmark"); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(endpointIDs)), "endpoint_population")
}

// BenchmarkPostgresTelemetryWrite measures both append-only compliance
// telemetry and the bounded latest-inventory upsert used by Sync.
func BenchmarkPostgresTelemetryWrite(b *testing.B) {
	b.Run("state-report", func(b *testing.B) {
		ctx, tx, store := postgresBenchmarkStore(b, "fleet_settings", "endpoints", "drift_reports")
		endpointID := postgresBenchmarkEndpoints(b, ctx, store, 1)[0]
		payload := registry.StateReportPayload{InCompliance: true, Items: []registry.StateReportItem{}}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := store.InsertDriftReport(ctx, endpointID, "release-benchmark", "sha256:benchmark", payload); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		var rows int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM drift_reports WHERE endpoint_id = $1`, endpointID).Scan(&rows); err != nil {
			b.Fatal(err)
		}
		if rows != int64(b.N) {
			b.Fatalf("state-report rows=%d, want %d", rows, b.N)
		}
		b.ReportMetric(float64(rows), "report_rows")
	})

	b.Run("system-info-upsert", func(b *testing.B) {
		ctx, tx, store := postgresBenchmarkStore(b, "fleet_settings", "endpoints", "endpoint_system_info")
		endpointID := postgresBenchmarkEndpoints(b, ctx, store, 1)[0]
		payload := []byte(`{"hostname":"benchmark","architecture":"x86_64","distro":"debian"}`)

		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := store.UpsertEndpointSystemInfo(ctx, endpointID, "sha256:system-info", payload); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		var rows int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM endpoint_system_info WHERE endpoint_id = $1`, endpointID).Scan(&rows); err != nil {
			b.Fatal(err)
		}
		if rows != 1 {
			b.Fatalf("system-info rows=%d, want 1 bounded row", rows)
		}
		b.ReportMetric(float64(rows), "inventory_rows")
	})
}

// BenchmarkPostgresFleetReporting measures the operator-visible Fleet state
// report over the 400-endpoint reference population.
func BenchmarkPostgresFleetReporting(b *testing.B) {
	ctx, _, store := postgresBenchmarkStore(b, "fleet_settings", "endpoints", "endpoint_labels", "drift_reports", "apply_failures")
	endpointIDs := postgresBenchmarkEndpoints(b, ctx, store, 400)
	payload := registry.StateReportPayload{InCompliance: true, Items: []registry.StateReportItem{}}
	for _, endpointID := range endpointIDs {
		if err := store.InsertDriftReport(ctx, endpointID, "release-benchmark", "sha256:benchmark", payload); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report, err := store.ListFleetStateReports(ctx, postgresBenchmarkFleet)
		if err != nil {
			b.Fatal(err)
		}
		if report.Summary.Total != len(endpointIDs) || len(report.Endpoints) != len(endpointIDs) {
			b.Fatalf("fleet report total=%d endpoints=%d, want %d", report.Summary.Total, len(report.Endpoints), len(endpointIDs))
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(endpointIDs)), "endpoint_population")
}

func postgresBenchmarkStore(b *testing.B, tableNames ...string) (context.Context, pgx.Tx, *Store) {
	b.Helper()
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
	for _, tableName := range tableNames {
		statement := fmt.Sprintf("CREATE TEMP TABLE %s (LIKE public.%s INCLUDING ALL) ON COMMIT DROP", tableName, tableName)
		if _, err := tx.Exec(ctx, statement); err != nil {
			b.Fatalf("create temporary %s table: %v", tableName, err)
		}
	}
	return ctx, tx, NewFromQueries(db.New(tx))
}

func postgresBenchmarkEndpoints(b *testing.B, ctx context.Context, store *Store, count int) []string {
	b.Helper()
	endpointIDs := make([]string, count)
	for i := range endpointIDs {
		endpointID := fmt.Sprintf("benchmark-endpoint-%03d", i)
		if _, err := store.RegisterEndpoint(ctx, endpointID, postgresBenchmarkFleet, fmt.Sprintf("fingerprint-%03d", i)); err != nil {
			b.Fatal(err)
		}
		endpointIDs[i] = endpointID
	}
	return endpointIDs
}
