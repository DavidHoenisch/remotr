package hubcatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_thirdPartyRequiresSourceCommit(t *testing.T) {
	catalog := Catalog{
		Categories: []Category{{ID: "manifests", Label: "Manifests"}},
		Entries: []Entry{
			{
				ID:        "external",
				Category:  "manifests",
				SourceURL: "https://github.com/acme/fleet-config/blob/main/desired.yaml",
			},
		},
	}

	res := Validate(catalog, "catalog.json", t.TempDir())
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
	if res.Issues[0].Message != "sourceCommit is required for third-party sourceUrl entries (pin to an immutable commit hash)" {
		t.Fatalf("message = %q", res.Issues[0].Message)
	}
}

func TestValidate_thirdPartyAcceptsPinnedCommit(t *testing.T) {
	catalog := Catalog{
		Categories: []Category{{ID: "repos", Label: "Repos"}},
		Entries: []Entry{
			{
				ID:           "external",
				Category:     "repos",
				SourceURL:    "https://github.com/acme/fleet-config/tree/deadbee12345",
				SourceCommit: "deadbee1234567890abcdef1234567890abcdef1",
			},
		},
	}

	res := Validate(catalog, "catalog.json", t.TempDir())
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidate_firstPartySourceURLWithoutCommitAllowed(t *testing.T) {
	catalog := Catalog{
		Categories: []Category{{ID: "manifests", Label: "Manifests"}},
		Entries: []Entry{
			{
				ID:        "internal",
				Category:  "manifests",
				SourceURL: "https://github.com/DavidHoenisch/remotr/blob/master/docs/reference/configuration-format.md",
			},
		},
	}

	res := Validate(catalog, "catalog.json", t.TempDir())
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidate_sourceCommitMustMatchURLRef(t *testing.T) {
	catalog := Catalog{
		Categories: []Category{{ID: "manifests", Label: "Manifests"}},
		Entries: []Entry{
			{
				ID:           "mismatch",
				Category:     "manifests",
				SourceURL:    "https://github.com/acme/fleet-config/blob/aaa1111/desired.yaml",
				SourceCommit: "bbb2222cccccccccccccccccccccccccccccccc",
			},
		},
	}

	res := Validate(catalog, "catalog.json", t.TempDir())
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidate_sourceCommitWithoutURLRejected(t *testing.T) {
	catalog := Catalog{
		Categories: []Category{{ID: "manifests", Label: "Manifests"}},
		Entries: []Entry{
			{
				ID:           "orphan-commit",
				Category:     "manifests",
				SourceCommit: "deadbee1234567890abcdef1234567890abcdef1",
			},
		},
	}

	res := Validate(catalog, "catalog.json", t.TempDir())
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateCatalogJSON(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(repoRoot, "hub", "data", "catalog.json")
	hubRoot := filepath.Join(repoRoot, "hub")

	res, err := ValidateFile(catalogPath, hubRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("catalog issues = %+v", res.Issues)
	}
}

func TestValidate_snippetPathMustExist(t *testing.T) {
	hubRoot := t.TempDir()
	catalog := Catalog{
		Categories: []Category{{ID: "snippets", Label: "Snippets"}},
		Entries: []Entry{
			{
				ID:          "missing-snippet",
				Category:    "snippets",
				SnippetPath: "snippets/does-not-exist.yaml",
			},
		},
	}

	res := Validate(catalog, "catalog.json", hubRoot)
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidate_snippetPathMustBeFile(t *testing.T) {
	hubRoot := t.TempDir()
	snippetDir := filepath.Join(hubRoot, "snippets")
	if err := os.MkdirAll(snippetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	catalog := Catalog{
		Categories: []Category{{ID: "snippets", Label: "Snippets"}},
		Entries: []Entry{
			{
				ID:          "dir-snippet",
				Category:    "snippets",
				SnippetPath: "snippets",
			},
		},
	}

	res := Validate(catalog, "catalog.json", hubRoot)
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}
