package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-OSQ-001 & OS-OSQ-002: kind: queries documents parse and validate read-only
// SQL, schedules, snapshot/diff modes, and strict execution bounds.
func TestParseQueryState_ValidAndInvalid(t *testing.T) {
	validYAML := `kind: queries
schemaVersion: 1
queries:
  - name: listening-ports
    description: Non-loopback listening TCP/UDP sockets
    schedule: "*/30 * * * *"
    timezone: UTC
    mode: diff
    timeout: 15s
    maxRows: 500
    retentionRuns: 20
    targetDistros: [ubuntu, debian, arch]
    sql: |
      -- comment allowed before select
      SELECT lp.port, lp.protocol, p.name AS process
      FROM listening_ports lp
      JOIN processes p ON lp.pid = p.pid
      WHERE lp.address NOT IN ('127.0.0.1', '::1');
  - name: kernel-version
    schedule: "0 * * * *"
    sql: "WITH k AS (SELECT version FROM kernel_info) SELECT version FROM k"
`
	if !types.KindQueries.Valid() || types.KindQueries != "queries" {
		t.Fatalf("KindQueries = %q valid=%v, want queries/true", types.KindQueries, types.KindQueries.Valid())
	}

	state, err := models.ParseQueryState(strings.NewReader(validYAML))
	if err != nil {
		t.Fatalf("ParseQueryState() error = %v", err)
	}
	if len(state.Queries) != 2 {
		t.Fatalf("len(Queries) = %d, want 2", len(state.Queries))
	}
	if state.Queries[0].EffectiveMode() != models.QueryModeDiff {
		t.Fatalf("query[0].EffectiveMode() = %q, want diff", state.Queries[0].EffectiveMode())
	}
	if state.Queries[1].EffectiveMode() != models.QueryModeSnapshot {
		t.Fatalf("query[1].EffectiveMode() = %q, want snapshot", state.Queries[1].EffectiveMode())
	}
	if state.Queries[1].EffectiveMaxRows() != models.DefaultQueryMaxRows {
		t.Fatalf("default maxRows = %d, want %d", state.Queries[1].EffectiveMaxRows(), models.DefaultQueryMaxRows)
	}
	if state.Queries[1].EffectiveRetentionRuns() != models.DefaultQueryRetentionRuns {
		t.Fatalf("default retentionRuns = %d, want %d", state.Queries[1].EffectiveRetentionRuns(), models.DefaultQueryRetentionRuns)
	}
}

func TestValidateReadOnlySQL(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{name: "simple select", sql: "SELECT * FROM os_version"},
		{name: "select with trailing semicolon", sql: "SELECT name, path FROM processes;"},
		{name: "cte with select", sql: "WITH p AS (SELECT pid FROM processes) SELECT * FROM p"},
		{name: "select with string containing drop keyword", sql: "SELECT name FROM processes WHERE name != 'drop_table'"},
		{name: "select with block comment", sql: "/* check */ SELECT version FROM os_version"},
		{name: "empty", sql: "   ", wantErr: true},
		{name: "drop table", sql: "DROP TABLE users", wantErr: true},
		{name: "delete statement", sql: "DELETE FROM processes", wantErr: true},
		{name: "insert statement", sql: "INSERT INTO foo VALUES (1)", wantErr: true},
		{name: "update statement", sql: "UPDATE foo SET a=1", wantErr: true},
		{name: "attach database", sql: "SELECT 1; ATTACH DATABASE '/tmp/x' AS x", wantErr: true},
		{name: "standalone attach", sql: "ATTACH DATABASE '/tmp/x' AS x", wantErr: true},
		{name: "pragma statement", sql: "PRAGMA table_info(users)", wantErr: true},
		{name: "multi-statement select", sql: "SELECT 1; SELECT 2", wantErr: true},
		{name: "load_extension call", sql: "SELECT load_extension('/tmp/evil.so')", wantErr: true},
		{name: "cte with delete", sql: "WITH x AS (SELECT 1) DELETE FROM users", wantErr: true},
		{name: "nul byte", sql: "SELECT 1\x00", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := models.ValidateReadOnlySQL(tc.sql)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateReadOnlySQL(%q) error = %v, wantErr %v", tc.sql, err, tc.wantErr)
			}
		})
	}
}

func TestQuerySpec_ValidateBoundsAndFields(t *testing.T) {
	cases := []struct {
		name    string
		spec    models.QuerySpec
		wantErr bool
	}{
		{
			name: "valid minimal",
			spec: models.QuerySpec{Name: "os-info", Schedule: "0 * * * *", SQL: "SELECT * FROM os_version"},
		},
		{
			name:    "invalid name with slash",
			spec:    models.QuerySpec{Name: "../bad", Schedule: "0 * * * *", SQL: "SELECT 1"},
			wantErr: true,
		},
		{
			name:    "invalid mode",
			spec:    models.QuerySpec{Name: "q1", Schedule: "0 * * * *", Mode: "stream", SQL: "SELECT 1"},
			wantErr: true,
		},
		{
			name:    "maxRows exceeds cap",
			spec:    models.QuerySpec{Name: "q1", Schedule: "0 * * * *", MaxRows: 5001, SQL: "SELECT 1"},
			wantErr: true,
		},
		{
			name:    "retentionRuns exceeds cap",
			spec:    models.QuerySpec{Name: "q1", Schedule: "0 * * * *", RetentionRuns: 101, SQL: "SELECT 1"},
			wantErr: true,
		},
		{
			name:    "timeout exceeds 60s",
			spec:    models.QuerySpec{Name: "q1", Schedule: "0 * * * *", Timeout: "90s", SQL: "SELECT 1"},
			wantErr: true,
		},
		{
			name:    "invalid schedule",
			spec:    models.QuerySpec{Name: "q1", Schedule: "not a cron", SQL: "SELECT 1"},
			wantErr: true,
		},
		{
			name:    "invalid timezone",
			spec:    models.QuerySpec{Name: "q1", Schedule: "0 * * * *", Timezone: "Mars/Olympus", SQL: "SELECT 1"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func FuzzValidateReadOnlySQL(f *testing.F) {
	f.Add("SELECT * FROM os_version")
	f.Add("WITH a AS (SELECT 1) SELECT * FROM a")
	f.Add("SELECT 1; DROP TABLE users")
	f.Add("PRAGMA database_list")
	f.Add("ATTACH '/tmp/a' AS a")

	f.Fuzz(func(t *testing.T, sql string) {
		if len(sql) > 2048 {
			return
		}
		err := models.ValidateReadOnlySQL(sql)
		if err != nil {
			return
		}
		upper := strings.ToUpper(sql)
		if strings.Contains(upper, "LOAD_EXTENSION") {
			t.Fatalf("accepted load_extension in %q", sql)
		}
		if strings.ContainsRune(sql, '\x00') {
			t.Fatalf("accepted NUL byte in %q", sql)
		}
	})
}
