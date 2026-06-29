package configcompose

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// Options controls repository-wide composition preview (legacy Result shape).
type Options struct {
	RepoRoot string
	Fleet    string // optional: render one fleet and related endpoint manifests
	Check    bool   // deprecated: use ValidateComposition
	DryRun   bool   // deprecated
	Stdout   string // optional: desired, crons, or all — render to Result.Rendered
}

// Issue is one composition problem.
type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Rendered is one composed artifact for stdout output.
type Rendered struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Result summarizes a composition run.
type Result struct {
	RepoRoot string     `json:"repo_root"`
	Written  []string   `json:"written,omitempty"`
	Stale    []string   `json:"stale,omitempty"`
	OK       []string   `json:"ok,omitempty"`
	Diffs    []Diff     `json:"diffs,omitempty"`
	Rendered []Rendered `json:"rendered,omitempty"`
	Issues   []Issue    `json:"issues,omitempty"`
}

// Compose previews composed artifacts (no disk writes). Deprecated: use RenderStdout or ValidateComposition.
func Compose(opts Options) (Result, error) {
	repoRoot := strings.TrimSpace(opts.RepoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Result{}, fmt.Errorf("repository: %w", err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("repository: %s is not a directory", abs)
	}

	stdout, err := parseStdoutMode(opts.Stdout)
	if err != nil {
		return Result{}, err
	}
	if stdout != "" {
		return RenderStdout(abs, opts.Fleet, stdout)
	}
	if opts.Check || opts.DryRun {
		return ValidateComposition(abs)
	}
	return ValidateComposition(abs)
}

func parseStdoutMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return "", nil
	}
	switch mode {
	case "desired", "crons", "all":
		return mode, nil
	default:
		return "", fmt.Errorf("stdout must be desired, crons, or all")
	}
}

func normalizeYAML(data []byte) []byte {
	state, err := models.ParseState(bytes.NewReader(data))
	if err != nil {
		return data
	}
	out, err := marshalState(state)
	if err != nil {
		return data
	}
	return out
}
