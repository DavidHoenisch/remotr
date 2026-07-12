//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/pipeline"
	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
	"github.com/DavidHoenisch/remotr/test/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStateReportRedactionPersistsNoDesiredSecretCanary(t *testing.T) {
	ctx := testsupport.Context(t, 30*time.Second)
	databaseURL := envOr("REMOTR_E2E_DATABASE_URL", "postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable")
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Postgres unavailable: %v", err)
	}
	store := pgstore.NewFromPool(pool)

	canary := testsupport.SecretCanary("state-report-postgres")
	managedPath := filepath.Join(t.TempDir(), "managed.conf")
	artifact := []byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: file\n        name: managed\n        path: " + managedPath + "\n        content: " + canary + "\n")
	result, err := pipeline.Run(ctx, artifact, engine.PolicyReport, nil, nil, "https://remotr.example")
	if err != nil {
		t.Fatal(err)
	}
	var pending agentsync.Pending
	pending.SetFromPipeline(result.Labels, result.Drift, result.Apply, result.ApplyFailure, "digest-canary")
	if pending.Drift == nil || strings.Contains(string(pending.Drift.Report), canary) {
		t.Fatalf("agent report leaked desired-state canary: %s", pending.Drift.Report)
	}

	endpointID := uuid.NewString()
	if _, err := store.RegisterEndpoint(ctx, endpointID, "redaction-e2e", "redaction-"+endpointID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := store.DeleteEndpoint(context.Background(), endpointID); err != nil {
			t.Errorf("delete redaction endpoint: %v", err)
		}
	})
	if err := store.InsertDriftReport(ctx, endpointID, "redaction", "digest-canary", pending.Drift.Report); err != nil {
		t.Fatal(err)
	}

	var stored []byte
	if err := pool.QueryRow(ctx, `SELECT report_json FROM drift_reports WHERE endpoint_id = $1 ORDER BY reported_at DESC LIMIT 1`, endpointID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), canary) {
		t.Fatalf("Postgres report_json leaked desired-state canary: %s", stored)
	}
	report, ok, err := store.GetEndpointStateReport(ctx, endpointID)
	if err != nil || !ok {
		t.Fatalf("read state report: ok=%t err=%v", ok, err)
	}
	apiJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(apiJSON), canary) {
		t.Fatalf("persisted state report model leaked canary: %s", apiJSON)
	}
}
