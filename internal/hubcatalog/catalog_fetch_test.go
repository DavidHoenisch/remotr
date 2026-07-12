package hubcatalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/hubcatalog"
)

func TestFetchCatalogURL(t *testing.T) {
	const body = `{"categories":[],"entries":[{"id":"demo","title":"Demo","snippetPath":"snippets/demo.yaml"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	catalog, err := hubcatalog.FetchCatalogURL(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 1 || catalog.Entries[0].ID != "demo" {
		t.Fatalf("entries = %+v", catalog.Entries)
	}
}

func TestResolveCatalog_remoteFallback(t *testing.T) {
	const body = `{"categories":[],"entries":[{"id":"remote","title":"Remote","snippetPath":"snippets/remote.yaml"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	catalog, hubRoot, err := hubcatalog.ResolveCatalog(context.Background(), hubcatalog.ImportOptions{
		RepoRoot:         dir,
		RemoteCatalogURL: srv.URL,
		HTTPClient:       srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if hubRoot != "" {
		t.Fatalf("hubRoot = %q want empty", hubRoot)
	}
	if len(catalog.Entries) != 1 || catalog.Entries[0].ID != "remote" {
		t.Fatalf("entries = %+v", catalog.Entries)
	}
}

func TestResolveCatalog_localHub(t *testing.T) {
	hubRoot := filepath.Join("..", "..", "hub")
	if _, err := os.Stat(filepath.Join(hubRoot, "data", "catalog.json")); err != nil {
		// test-exception: EXC-009
		t.Skip("hub catalog not available")
	}

	catalog, root, err := hubcatalog.ResolveCatalog(context.Background(), hubcatalog.ImportOptions{
		HubRoot: hubRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if root != hubRoot {
		t.Fatalf("hubRoot = %q", root)
	}
	if len(catalog.Entries) == 0 {
		t.Fatal("expected catalog entries")
	}
}
