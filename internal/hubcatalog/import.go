package hubcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

const defaultGitHubRepo = "DavidHoenisch/remotr"

// ImportOptions controls copying a Hub catalog snippet into a config repository.
type ImportOptions struct {
	EntryID     string
	OutPath     string
	RepoRoot    string
	HubRoot     string
	CatalogPath string
	HTTPClient  *http.Client
}

// ImportResult summarizes a snippet import.
type ImportResult struct {
	EntryID    string `json:"entry_id"`
	OutPath    string `json:"out_path"`
	SnippetSrc string `json:"snippet_src"`
}

// LoadCatalog reads catalog JSON from catalogPath.
func LoadCatalog(catalogPath string) (Catalog, error) {
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return Catalog{}, err
	}
	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("parse catalog: %w", err)
	}
	return catalog, nil
}

// FindEntry returns the catalog entry for id.
func FindEntry(catalog Catalog, id string) (Entry, error) {
	id = strings.TrimSpace(id)
	for _, entry := range catalog.Entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return Entry{}, fmt.Errorf("catalog entry %q not found", id)
}

// ImportSnippet copies a Hub catalog snippet into a configuration repository module file.
func ImportSnippet(ctx context.Context, opts ImportOptions) (ImportResult, error) {
	entryID := strings.TrimSpace(opts.EntryID)
	if entryID == "" {
		return ImportResult{}, fmt.Errorf("entry id is required")
	}
	repoRoot := strings.TrimSpace(opts.RepoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return ImportResult{}, err
	}

	catalog, hubRoot, err := resolveCatalog(opts)
	if err != nil {
		return ImportResult{}, err
	}
	entry, err := FindEntry(catalog, entryID)
	if err != nil {
		return ImportResult{}, err
	}
	if strings.TrimSpace(entry.SnippetPath) == "" {
		return ImportResult{}, fmt.Errorf("entry %q has no snippetPath", entryID)
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	snippet, src, err := readSnippet(ctx, client, entry, hubRoot)
	if err != nil {
		return ImportResult{}, err
	}
	if err := validateSnippet(snippet); err != nil {
		return ImportResult{}, fmt.Errorf("snippet %q: %w", entry.SnippetPath, err)
	}

	outPath := strings.TrimSpace(opts.OutPath)
	if outPath == "" {
		outPath = filepath.Join("modules", sanitizeFilename(entryID)+".yaml")
	}
	if filepath.IsAbs(outPath) {
		return ImportResult{}, fmt.Errorf("out path must be relative to repository root")
	}
	dest := filepath.Join(repoRoot, filepath.FromSlash(filepath.ToSlash(outPath)))
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ImportResult{}, err
	}
	if err := os.WriteFile(dest, snippet, 0o644); err != nil { // #nosec G306 -- public module file
		return ImportResult{}, err
	}
	return ImportResult{
		EntryID:    entryID,
		OutPath:    filepath.ToSlash(outPath),
		SnippetSrc: src,
	}, nil
}

func resolveCatalog(opts ImportOptions) (Catalog, string, error) {
	if p := strings.TrimSpace(opts.CatalogPath); p != "" {
		catalog, err := LoadCatalog(p)
		if err != nil {
			return Catalog{}, "", err
		}
		hubRoot := strings.TrimSpace(opts.HubRoot)
		if hubRoot == "" {
			hubRoot = filepath.Dir(filepath.Dir(p))
		}
		return catalog, hubRoot, nil
	}
	if hubRoot := strings.TrimSpace(opts.HubRoot); hubRoot != "" {
		catalog, err := LoadCatalog(filepath.Join(hubRoot, "data", "catalog.json"))
		if err != nil {
			return Catalog{}, "", err
		}
		return catalog, hubRoot, nil
	}
	if root := findHubRoot("."); root != "" {
		catalog, err := LoadCatalog(filepath.Join(root, "data", "catalog.json"))
		if err != nil {
			return Catalog{}, "", err
		}
		return catalog, root, nil
	}
	return Catalog{}, "", fmt.Errorf("hub catalog not found; set --hub-root or --catalog")
}

func findHubRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "data", "catalog.json")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "snippets")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func readSnippet(ctx context.Context, client *http.Client, entry Entry, hubRoot string) ([]byte, string, error) {
	if hubRoot != "" {
		path := filepath.Join(hubRoot, filepath.FromSlash(entry.SnippetPath))
		data, err := os.ReadFile(path)
		if err == nil {
			return data, path, nil
		}
		if !os.IsNotExist(err) {
			return nil, "", err
		}
	}
	commit := strings.TrimSpace(entry.SourceCommit)
	if commit == "" {
		return nil, "", fmt.Errorf("snippet unavailable locally and entry %q has no sourceCommit for remote fetch", entry.ID)
	}
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/hub/%s", defaultGitHubRepo, commit, filepath.ToSlash(entry.SnippetPath))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch snippet: HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, "", err
	}
	return data, rawURL, nil
}

func validateSnippet(data []byte) error {
	if _, err := models.ParseState(bytes.NewReader(data)); err == nil {
		return nil
	}
	if _, err := models.ParseCronState(bytes.NewReader(data)); err == nil {
		return nil
	}
	return fmt.Errorf("snippet is neither desired-state nor crons YAML")
}

func sanitizeFilename(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "snippet"
	}
	return out
}
