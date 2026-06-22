package hubcatalog

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const canonicalRepo = "davidhoenisch/remotr"

var commitHashPattern = regexp.MustCompile(`(?i)^[0-9a-f]{7,40}$`)

// Catalog is the Remotr Hub community catalog document.
type Catalog struct {
	Categories []Category `json:"categories"`
	Entries    []Entry    `json:"entries"`
}

// Category groups catalog entries for display and filtering.
type Category struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Entry is one item in the hub catalog.
type Entry struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Tags         []string `json:"tags"`
	Distros      []string `json:"distros"`
	Author       string   `json:"author"`
	SourceURL    string   `json:"sourceUrl"`
	SourceCommit string   `json:"sourceCommit"`
	SnippetPath  string   `json:"snippetPath"`
	Featured     bool     `json:"featured"`
}

// ValidationIssue is one problem found in the hub catalog.
type ValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationResult summarizes a catalog validation run.
type ValidationResult struct {
	CatalogPath string            `json:"catalog_path"`
	Issues      []ValidationIssue `json:"issues,omitempty"`
}

// ValidateFile loads and validates the catalog JSON at catalogPath.
// hubRoot is the directory containing data/ and snippets/ (typically hub/).
func ValidateFile(catalogPath, hubRoot string) (ValidationResult, error) {
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return ValidationResult{}, err
	}
	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return ValidationResult{}, fmt.Errorf("parse catalog: %w", err)
	}
	return Validate(catalog, catalogPath, hubRoot), nil
}

// Validate checks catalog structure and contribution policy.
func Validate(catalog Catalog, catalogPath, hubRoot string) ValidationResult {
	res := ValidationResult{CatalogPath: catalogPath}

	categoryIDs := map[string]struct{}{}
	for _, category := range catalog.Categories {
		if strings.TrimSpace(category.ID) == "" {
			res.Issues = append(res.Issues, ValidationIssue{
				Path:    catalogPath,
				Message: "category is missing id",
			})
			continue
		}
		categoryIDs[category.ID] = struct{}{}
	}

	seenIDs := map[string]struct{}{}
	for _, entry := range catalog.Entries {
		entryPath := fmt.Sprintf("%s entries[%q]", catalogPath, entry.ID)

		if strings.TrimSpace(entry.ID) == "" {
			res.Issues = append(res.Issues, ValidationIssue{
				Path:    catalogPath,
				Message: "entry is missing id",
			})
			continue
		}
		if _, ok := seenIDs[entry.ID]; ok {
			res.Issues = append(res.Issues, ValidationIssue{
				Path:    entryPath,
				Message: "duplicate entry id",
			})
		}
		seenIDs[entry.ID] = struct{}{}

		if entry.Category != "" {
			if _, ok := categoryIDs[entry.Category]; !ok {
				res.Issues = append(res.Issues, ValidationIssue{
					Path:    entryPath,
					Message: fmt.Sprintf("unknown category %q", entry.Category),
				})
			}
		}

		if entry.SnippetPath != "" {
			snippetPath := filepath.Join(hubRoot, filepath.FromSlash(entry.SnippetPath))
			info, err := os.Stat(snippetPath)
			if err != nil {
				res.Issues = append(res.Issues, ValidationIssue{
					Path:    entryPath,
					Message: fmt.Sprintf("snippetPath %q: %v", entry.SnippetPath, err),
				})
			} else if info.IsDir() {
				res.Issues = append(res.Issues, ValidationIssue{
					Path:    entryPath,
					Message: fmt.Sprintf("snippetPath %q is a directory", entry.SnippetPath),
				})
			}
		}

		validateSource(entry, entryPath, &res)
	}

	return res
}

func validateSource(entry Entry, entryPath string, res *ValidationResult) {
	sourceURL := strings.TrimSpace(entry.SourceURL)
	sourceCommit := strings.TrimSpace(entry.SourceCommit)

	if sourceURL == "" {
		if sourceCommit != "" {
			res.Issues = append(res.Issues, ValidationIssue{
				Path:    entryPath,
				Message: "sourceCommit requires sourceUrl",
			})
		}
		return
	}

	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		res.Issues = append(res.Issues, ValidationIssue{
			Path:    entryPath,
			Message: "sourceUrl must be a valid https URL",
		})
		return
	}

	thirdParty := isThirdPartySourceURL(parsed)
	if thirdParty && sourceCommit == "" {
		res.Issues = append(res.Issues, ValidationIssue{
			Path:    entryPath,
			Message: "sourceCommit is required for third-party sourceUrl entries (pin to an immutable commit hash)",
		})
		return
	}

	if sourceCommit != "" && !commitHashPattern.MatchString(sourceCommit) {
		res.Issues = append(res.Issues, ValidationIssue{
			Path:    entryPath,
			Message: "sourceCommit must be a git commit hash (7–40 hexadecimal characters)",
		})
		return
	}

	if sourceCommit == "" {
		return
	}

	if ref, ok := gitHostingRef(parsed); ok && !commitRefsMatch(ref, sourceCommit) {
		res.Issues = append(res.Issues, ValidationIssue{
			Path:    entryPath,
			Message: fmt.Sprintf("sourceUrl ref %q must match sourceCommit %q", ref, sourceCommit),
		})
	}
}

func isThirdPartySourceURL(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Hostname())
	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")

	switch host {
	case "github.com", "www.github.com":
		if len(pathParts) >= 2 {
			return strings.EqualFold(pathParts[0]+"/"+pathParts[1], canonicalRepo) == false
		}
	case "raw.githubusercontent.com":
		if len(pathParts) >= 2 {
			return strings.EqualFold(pathParts[0]+"/"+pathParts[1], canonicalRepo) == false
		}
	}

	// Unknown hosts and malformed GitHub paths are treated as third-party.
	return true
}

func gitHostingRef(parsed *url.URL) (string, bool) {
	host := strings.ToLower(parsed.Hostname())
	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")

	switch host {
	case "github.com", "www.github.com":
		if len(pathParts) >= 4 && (pathParts[2] == "blob" || pathParts[2] == "tree") {
			return pathParts[3], true
		}
		if len(pathParts) >= 4 && pathParts[2] == "commit" {
			return pathParts[3], true
		}
	case "raw.githubusercontent.com":
		if len(pathParts) >= 3 {
			return pathParts[2], true
		}
	}

	return "", false
}

func commitRefsMatch(urlRef, sourceCommit string) bool {
	urlRef = strings.ToLower(urlRef)
	sourceCommit = strings.ToLower(sourceCommit)
	return strings.HasPrefix(sourceCommit, urlRef) || strings.HasPrefix(urlRef, sourceCommit)
}
