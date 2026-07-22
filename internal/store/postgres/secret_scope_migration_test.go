package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestGlobalSecretScopeMigrationBackfillsAndConstrainsLegacyRows(t *testing.T) {
	raw, err := os.ReadFile("../../../sql/migrations/020_global_secret_scope.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS scope_type",
		"ADD COLUMN IF NOT EXISTS scope_id",
		"legacy secret version has neither or both scope identifiers",
		"logical secret versions disagree on scope",
		"CHECK (scope_type IN ('global', 'fleet', 'endpoint'))",
		"scope_type = 'global' AND scope_id IS NULL",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
	if strings.Contains(sql, "UPDATE secret_versions") || strings.Contains(sql, "SET envelope_json") {
		t.Fatal("scope migration mutates authenticated secret envelope metadata")
	}
}
