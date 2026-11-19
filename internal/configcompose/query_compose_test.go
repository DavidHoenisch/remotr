package configcompose_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-OSQ-001: kind: queries documents referenced by fleet manifests are
// discovered, validated, and composed into per-fleet queries artifacts.
func TestRenderAllAndDiscoverFleet_ComposesQueries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    packages:
      - name: curl
        present: true
`))
	writeFile(t, filepath.Join(dir, "queries", "network.yaml"), `kind: queries
schemaVersion: 1
queries:
  - name: listening-ports
    description: Non-loopback listening sockets
    schedule: "*/30 * * * *"
    mode: diff
    sql: "SELECT port, protocol FROM listening_ports WHERE address != '127.0.0.1'"
`)
	writeFile(t, filepath.Join(dir, "fleets", "prod", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
queries:
  - queries/network.yaml
`))

	summary, err := configcompose.DiscoverFleet(dir, "prod")
	if err != nil {
		t.Fatalf("DiscoverFleet() error = %v", err)
	}
	if len(summary.Queries) != 1 || summary.Queries[0] != "queries/network.yaml" {
		t.Fatalf("DiscoverFleet().Queries = %v, want [queries/network.yaml]", summary.Queries)
	}

	artifacts, err := configcompose.RenderAll(dir)
	if err != nil {
		t.Fatalf("RenderAll() error = %v", err)
	}
	var queryArtifact *configcompose.RenderedArtifact
	for i := range artifacts {
		if artifacts[i].TargetType == "fleet" && artifacts[i].TargetID == "prod" && artifacts[i].ArtifactType == "queries" {
			queryArtifact = &artifacts[i]
			break
		}
	}
	if queryArtifact == nil {
		t.Fatalf("RenderAll() missing queries artifact: %+v", artifacts)
	}
	parsed, err := models.ParseQueryState(bytes.NewReader(queryArtifact.YAML))
	if err != nil {
		t.Fatalf("ParseQueryState(rendered) error = %v", err)
	}
	if len(parsed.Queries) != 1 || parsed.Queries[0].Name != "listening-ports" || parsed.Queries[0].Mode != models.QueryModeDiff {
		t.Fatalf("parsed queries = %#v", parsed.Queries)
	}
}

// OS-OSQ-002: unsafe SQL or duplicate query names block composition.
func TestRenderAll_RejectsUnsafeSQLAndDuplicateQueries(t *testing.T) {
	t.Run("unsafe sql rejected", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    packages:
      - name: curl
        present: true
`))
		writeFile(t, filepath.Join(dir, "queries", "bad.yaml"), `kind: queries
schemaVersion: 1
queries:
  - name: evil
    schedule: "0 * * * *"
    sql: "SELECT 1; DROP TABLE users"
`)
		writeFile(t, filepath.Join(dir, "fleets", "prod", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
queries:
  - queries/bad.yaml
`))
		_, err := configcompose.RenderAll(dir)
		if err == nil || !strings.Contains(err.Error(), "semicolon") {
			t.Fatalf("RenderAll() error = %v, want unsafe SQL rejection", err)
		}
	})

	t.Run("duplicate query names across files rejected", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    packages:
      - name: curl
        present: true
`))
		writeFile(t, filepath.Join(dir, "queries", "a.yaml"), `kind: queries
schemaVersion: 1
queries:
  - name: dup-query
    schedule: "0 * * * *"
    sql: "SELECT 1"
`)
		writeFile(t, filepath.Join(dir, "queries", "b.yaml"), `kind: queries
schemaVersion: 1
queries:
  - name: dup-query
    schedule: "0 * * * *"
    sql: "SELECT 2"
`)
		writeFile(t, filepath.Join(dir, "fleets", "prod", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
queries:
  - queries/a.yaml
  - queries/b.yaml
`))
		_, err := configcompose.RenderAll(dir)
		if err == nil || !strings.Contains(err.Error(), "duplicate query") {
			t.Fatalf("RenderAll() error = %v, want duplicate query error", err)
		}
	})
}
