package models

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/DavidHoenisch/remotr/internal/schedule"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

// QueryMode controls how query results are tracked over time.
type QueryMode string

const (
	QueryModeSnapshot QueryMode = "snapshot"
	QueryModeDiff     QueryMode = "diff"
)

const (
	DefaultQueryTimeout       = 15 * time.Second
	MaxQueryTimeout           = 60 * time.Second
	DefaultQueryMaxRows       = 500
	MaxQueryMaxRows           = 5000
	DefaultQueryMaxBytes      = 1 << 20 // 1 MiB
	DefaultQueryRetentionRuns = 20
	MaxQueryRetentionRuns     = 100
	MaxQuerySQLBytes          = 16 << 10 // 16 KiB
)

// QuerySpec is one scheduled read-only osquery query inside a kind: queries document.
type QuerySpec struct {
	Name          string               `yaml:"name" json:"name"`
	Description   string               `yaml:"description,omitempty" json:"description,omitempty"`
	SQL           string               `yaml:"sql" json:"sql"`
	Schedule      string               `yaml:"schedule" json:"schedule"`
	Timezone      string               `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	Mode          QueryMode            `yaml:"mode,omitempty" json:"mode,omitempty"`
	Timeout       string               `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	MaxRows       int                  `yaml:"maxRows,omitempty" json:"max_rows,omitempty"`
	RetentionRuns int                  `yaml:"retentionRuns,omitempty" json:"retention_runs,omitempty"`
	TargetDistros []types.Distro       `yaml:"targetDistros,omitempty" json:"target_distros,omitempty"`
	TargetArch    []types.Architecture `yaml:"targetArch,omitempty" json:"target_arch,omitempty"`
}

// QueryState is the compiled per-fleet queries artifact (also used for parsing kind: queries).
type QueryState struct {
	Kind          types.Kind  `yaml:"kind,omitempty" json:"kind,omitempty"`
	SchemaVersion int         `yaml:"schemaVersion,omitempty" json:"schema_version,omitempty"`
	Queries       []QuerySpec `yaml:"queries" json:"queries"`
}

// EffectiveMode returns the query mode, defaulting to snapshot.
func (q QuerySpec) EffectiveMode() QueryMode {
	if q.Mode == "" {
		return QueryModeSnapshot
	}
	return q.Mode
}

// EffectiveTimeout returns the parsed query timeout, defaulting to DefaultQueryTimeout.
func (q QuerySpec) EffectiveTimeout() time.Duration {
	if strings.TrimSpace(q.Timeout) == "" {
		return DefaultQueryTimeout
	}
	d, err := time.ParseDuration(strings.TrimSpace(q.Timeout))
	if err != nil || d <= 0 {
		return DefaultQueryTimeout
	}
	return d
}

// EffectiveMaxRows returns the configured row cap, defaulting to DefaultQueryMaxRows.
func (q QuerySpec) EffectiveMaxRows() int {
	if q.MaxRows <= 0 {
		return DefaultQueryMaxRows
	}
	return q.MaxRows
}

// EffectiveRetentionRuns returns the diff/history retention count, defaulting to DefaultQueryRetentionRuns.
func (q QuerySpec) EffectiveRetentionRuns() int {
	if q.RetentionRuns <= 0 {
		return DefaultQueryRetentionRuns
	}
	return q.RetentionRuns
}

// Validate checks a single scheduled query specification.
func (q QuerySpec) Validate() error {
	if !validQueryName(q.Name) {
		return fmt.Errorf("query name %q must be a safe identifier (alphanumeric, -, _)", q.Name)
	}
	if err := ValidateReadOnlySQL(q.SQL); err != nil {
		return fmt.Errorf("query %q: %w", q.Name, err)
	}
	if err := schedule.ValidateCronExpression(q.Schedule); err != nil {
		return fmt.Errorf("query %q: %w", q.Name, err)
	}
	if _, err := schedule.LocationForCron(q.Timezone); err != nil {
		return fmt.Errorf("query %q timezone: %w", q.Name, err)
	}
	switch q.EffectiveMode() {
	case QueryModeSnapshot, QueryModeDiff:
	default:
		return fmt.Errorf("query %q mode %q must be snapshot or diff", q.Name, q.Mode)
	}
	if strings.TrimSpace(q.Timeout) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(q.Timeout))
		if err != nil || d <= 0 || d > MaxQueryTimeout {
			return fmt.Errorf("query %q timeout %q must be >0 and <=%s", q.Name, q.Timeout, MaxQueryTimeout)
		}
	}
	if q.MaxRows < 0 || q.MaxRows > MaxQueryMaxRows {
		return fmt.Errorf("query %q maxRows %d must be between 1 and %d", q.Name, q.MaxRows, MaxQueryMaxRows)
	}
	if q.RetentionRuns < 0 || q.RetentionRuns > MaxQueryRetentionRuns {
		return fmt.Errorf("query %q retentionRuns %d must be between 1 and %d", q.Name, q.RetentionRuns, MaxQueryRetentionRuns)
	}
	return nil
}

// Validate checks all queries in a QueryState for validity and uniqueness.
func (s QueryState) Validate() error {
	seen := map[string]struct{}{}
	for _, q := range s.Queries {
		if err := q.Validate(); err != nil {
			return err
		}
		if _, dup := seen[q.Name]; dup {
			return fmt.Errorf("duplicate query %q", q.Name)
		}
		seen[q.Name] = struct{}{}
	}
	return nil
}

// ParseQueryState decodes and validates a kind: queries YAML document (or compiled queries artifact).
func ParseQueryState(r io.Reader) (QueryState, error) {
	data, err := io.ReadAll(io.LimitReader(r, 2<<20))
	if err != nil {
		return QueryState{}, fmt.Errorf("read queries: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return QueryState{}, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var state QueryState
	if err := dec.Decode(&state); err != nil {
		return QueryState{}, fmt.Errorf("parse queries yaml: %w", err)
	}
	if state.Kind != "" && state.Kind != types.KindQueries {
		return QueryState{}, fmt.Errorf("want kind %s, got %q", types.KindQueries, state.Kind)
	}
	if state.SchemaVersion != 0 && state.SchemaVersion != 1 {
		return QueryState{}, fmt.Errorf("unsupported queries schemaVersion %d", state.SchemaVersion)
	}
	if err := state.Validate(); err != nil {
		return QueryState{}, err
	}
	return state, nil
}

// ValidateReadOnlySQL verifies that sql is a single read-only SELECT or WITH
// statement without mutating keywords, ATTACH/PRAGMA/LOAD_EXTENSION, or NUL bytes.
func ValidateReadOnlySQL(sql string) error {
	if len(sql) > MaxQuerySQLBytes {
		return fmt.Errorf("sql exceeds maximum length of %d bytes", MaxQuerySQLBytes)
	}
	if strings.ContainsRune(sql, '\x00') {
		return fmt.Errorf("sql contains NUL byte")
	}

	tokens, err := tokenizeSQL(sql)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return fmt.Errorf("sql is required")
	}

	// Allow at most a single trailing semicolon; reject any semicolon followed by another token.
	for i, tok := range tokens {
		if tok.kind == sqlTokSemicolon {
			if i != len(tokens)-1 {
				return fmt.Errorf("sql must be a single statement without chained semicolons")
			}
		}
	}
	if tokens[len(tokens)-1].kind == sqlTokSemicolon {
		tokens = tokens[:len(tokens)-1]
	}
	if len(tokens) == 0 {
		return fmt.Errorf("sql is required")
	}

	first := strings.ToUpper(tokens[0].text)
	if tokens[0].kind != sqlTokWord || (first != "SELECT" && first != "WITH") {
		return fmt.Errorf("sql must begin with SELECT or WITH")
	}

	for _, tok := range tokens {
		if tok.kind != sqlTokWord {
			continue
		}
		switch strings.ToUpper(tok.text) {
		case "INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "REPLACE",
			"ATTACH", "DETACH", "PRAGMA", "VACUUM", "REINDEX",
			"BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE",
			"LOAD_EXTENSION":
			return fmt.Errorf("sql contains forbidden keyword %q", strings.ToUpper(tok.text))
		}
	}
	return nil
}

type sqlTokKind int

const (
	sqlTokWord sqlTokKind = iota
	sqlTokString
	sqlTokSemicolon
	sqlTokOther
)

type sqlToken struct {
	kind sqlTokKind
	text string
}

func tokenizeSQL(s string) ([]sqlToken, error) {
	var tokens []sqlToken
	n := len(s)
	i := 0
	for i < n {
		c := s[i]
		// Whitespace
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		// Line comment --
		if c == '-' && i+1 < n && s[i+1] == '-' {
			i += 2
			for i < n && s[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment /* ... */
		if c == '/' && i+1 < n && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("sql has unterminated block comment")
			}
			i += 2 + end + 2
			continue
		}
		// Single-quoted string literal or double-quoted identifier
		if c == '\'' || c == '"' {
			quote := c
			i++
			closed := false
			for i < n {
				if s[i] == quote {
					if i+1 < n && s[i+1] == quote {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("sql has unterminated quoted literal")
			}
			tokens = append(tokens, sqlToken{kind: sqlTokString})
			continue
		}
		// Semicolon
		if c == ';' {
			tokens = append(tokens, sqlToken{kind: sqlTokSemicolon, text: ";"})
			i++
			continue
		}
		// Identifier / keyword
		if isSQLWordStart(c) {
			start := i
			i++
			for i < n && isSQLWordPart(s[i]) {
				i++
			}
			tokens = append(tokens, sqlToken{kind: sqlTokWord, text: s[start:i]})
			continue
		}
		tokens = append(tokens, sqlToken{kind: sqlTokOther, text: s[i : i+1]})
		i++
	}
	return tokens, nil
}

func isSQLWordStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isSQLWordPart(c byte) bool {
	return isSQLWordStart(c) || (c >= '0' && c <= '9')
}

func validQueryName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i, r := range name {
		isAlpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isAlpha && !isDigit {
			return false
		}
		if !isAlpha && !isDigit && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
