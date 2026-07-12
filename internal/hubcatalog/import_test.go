package hubcatalog_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/hubcatalog"
)

func TestImportSnippet_localHub(t *testing.T) {
	repo := t.TempDir()
	hubRoot := filepath.Join("..", "..", "hub")
	if _, err := os.Stat(filepath.Join(hubRoot, "data", "catalog.json")); err != nil {
		// test-exception: EXC-010
		t.Skip("hub catalog not available")
	}

	res, err := hubcatalog.ImportSnippet(context.Background(), hubcatalog.ImportOptions{
		EntryID:  "base-packages-debian-arch",
		RepoRoot: repo,
		HubRoot:  hubRoot,
		OutPath:  "modules/base-packages.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OutPath != "modules/base-packages.yaml" {
		t.Fatalf("out = %q", res.OutPath)
	}
	data, err := os.ReadFile(filepath.Join(repo, "modules", "base-packages.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "base-packages") {
		t.Fatalf("unexpected snippet: %s", data)
	}
}

func TestFindEntry_missing(t *testing.T) {
	_, err := hubcatalog.FindEntry(hubcatalog.Catalog{}, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}
