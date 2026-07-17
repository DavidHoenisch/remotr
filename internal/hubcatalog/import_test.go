package hubcatalog_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/hubcatalog"
	"github.com/DavidHoenisch/remotr/internal/models"
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

func TestImportSnippet_realCatalogEntryIsValidRepositorySource(t *testing.T) {
	repo := t.TempDir()
	hubRoot := filepath.Join("..", "..", "hub")

	res, err := hubcatalog.ImportSnippet(context.Background(), hubcatalog.ImportOptions{
		EntryID:  "base-packages-debian-arch",
		RepoRoot: repo,
		HubRoot:  hubRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OutPath != "modules/base-packages-debian-arch.yaml" {
		t.Fatalf("out = %q", res.OutPath)
	}
	manifestPath := filepath.Join(repo, "fleets", "test", "manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := "kind: manifest\nmodules:\n  - " + res.OutPath + "\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	validation, err := configrepo.ValidateRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(validation.Issues) != 0 {
		t.Fatalf("imported Hub entry is not a valid repository source: %+v", validation.Issues)
	}
}

func TestImportSnippet_cronEntryDefaultsToCronsDirectory(t *testing.T) {
	repo := t.TempDir()
	hubRoot := filepath.Join("..", "..", "hub")

	res, err := hubcatalog.ImportSnippet(context.Background(), hubcatalog.ImportOptions{
		EntryID:  "weekly-system-upgrade-builtin",
		RepoRoot: repo,
		HubRoot:  hubRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OutPath != "crons/weekly-system-upgrade-builtin.yaml" {
		t.Fatalf("out = %q", res.OutPath)
	}
	manifestPath := filepath.Join(repo, "fleets", "test", "manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := "kind: manifest\ncrons:\n  - " + res.OutPath + "\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	validation, err := configrepo.ValidateRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(validation.Issues) != 0 {
		t.Fatalf("imported Hub cron is not a valid repository source: %+v", validation.Issues)
	}
}

func TestCatalogEntriesImportAsValidRepositorySources(t *testing.T) {
	hubRoot := filepath.Join("..", "..", "hub")
	catalog, err := hubcatalog.LoadCatalog(filepath.Join(hubRoot, "data", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range catalog.Entries {
		if entry.SnippetPath == "" {
			continue
		}
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			repo := t.TempDir()
			res, err := hubcatalog.ImportSnippet(context.Background(), hubcatalog.ImportOptions{
				EntryID:  entry.ID,
				RepoRoot: repo,
				HubRoot:  hubRoot,
			})
			if err != nil {
				t.Fatal(err)
			}

			var manifestField string
			switch {
			case strings.HasPrefix(res.OutPath, "modules/"):
				manifestField = "modules"
			case strings.HasPrefix(res.OutPath, "crons/"):
				manifestField = "crons"
			default:
				t.Fatalf("unsupported default output path %q", res.OutPath)
			}
			manifestPath := filepath.Join(repo, "fleets", "test", "manifest.yaml")
			if err := os.MkdirAll(filepath.Dir(manifestPath), 0o750); err != nil {
				t.Fatal(err)
			}
			manifest := "kind: manifest\n" + manifestField + ":\n  - " + res.OutPath + "\n"
			if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			validation, err := configrepo.ValidateRepository(repo)
			if err != nil {
				t.Fatal(err)
			}
			if len(validation.Issues) != 0 {
				t.Fatalf("imported Hub entry is not a valid repository source: %+v", validation.Issues)
			}
		})
	}

}

func TestCatalogRegistersEverySnippetFile(t *testing.T) {
	hubRoot := filepath.Join("..", "..", "hub")
	catalog, err := hubcatalog.LoadCatalog(filepath.Join(hubRoot, "data", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	registered := make(map[string]struct{}, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if entry.SnippetPath != "" {
			registered[filepath.ToSlash(entry.SnippetPath)] = struct{}{}
		}
	}

	files, err := filepath.Glob(filepath.Join(hubRoot, "snippets", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var unregistered []string
	for _, path := range files {
		rel, err := filepath.Rel(hubRoot, path)
		if err != nil {
			t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		if _, ok := registered[rel]; !ok {
			unregistered = append(unregistered, rel)
		}
	}
	sort.Strings(unregistered)
	if len(unregistered) != 0 {
		t.Fatalf("unregistered Hub snippets: %v", unregistered)
	}
}

func TestMultiDistroBuiltinCronSnippetsReferenceDebianAndArchJobs(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{path: "weekly-system-upgrade.yaml", want: []string{"builtin/system-upgrade-debian", "builtin/system-upgrade-arch"}},
		{path: "clamav-scan-debian.yaml", want: []string{"builtin/clamav-scan-debian", "builtin/clamav-scan-arch"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "hub", "snippets", tt.path))
			if err != nil {
				t.Fatal(err)
			}
			state, err := models.ParseCronState(strings.NewReader(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, job := range state.Crons {
				got = append(got, job.Use)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("builtin references = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindEntry_missing(t *testing.T) {
	_, err := hubcatalog.FindEntry(hubcatalog.Catalog{}, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}
