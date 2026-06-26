package configcompose

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/croncatalog"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Options controls repository-wide composition.
type Options struct {
	RepoRoot string
	Fleet    string // optional: compose one fleet and related endpoint manifests
	Check    bool   // compare generated output to on-disk artifacts without writing
	DryRun   bool   // show diffs for changes without writing
}

// Issue is one composition problem.
type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Result summarizes a composition run.
type Result struct {
	RepoRoot string  `json:"repo_root"`
	Written  []string `json:"written,omitempty"`
	Stale    []string `json:"stale,omitempty"`
	OK       []string `json:"ok,omitempty"`
	Diffs    []Diff  `json:"diffs,omitempty"`
	Issues   []Issue `json:"issues,omitempty"`
}

// HasManifests reports whether repoRoot contains any composition manifest sources.
func HasManifests(repoRoot string) (bool, error) {
	desired, err := discoverManifests(repoRoot, "")
	if err != nil {
		return false, err
	}
	crons, err := discoverCronManifests(repoRoot, "")
	if err != nil {
		return false, err
	}
	return len(desired) > 0 || len(crons) > 0, nil
}

// Compose generates desired.yaml and crons.yaml artifacts from manifest sources.
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

	desiredManifests, err := discoverManifests(abs, opts.Fleet)
	if err != nil {
		return Result{}, err
	}
	cronManifests, err := discoverCronManifests(abs, opts.Fleet)
	if err != nil {
		return Result{}, err
	}
	if len(desiredManifests) == 0 && len(cronManifests) == 0 {
		return Result{
			RepoRoot: abs,
			Issues: []Issue{{
				Path:    abs,
				Message: "no manifest.yaml or crons.manifest.yaml files found under fleets/ or endpoints/",
			}},
		}, nil
	}

	res := Result{RepoRoot: abs}
	write := !opts.Check && !opts.DryRun
	for _, manifestRel := range desiredManifests {
		if err := composeDesiredOne(abs, manifestRel, write, opts.Check || opts.DryRun, opts.DryRun, &res); err != nil {
			return res, err
		}
	}
	for _, manifestRel := range cronManifests {
		if err := composeCronOne(abs, manifestRel, write, opts.Check || opts.DryRun, opts.DryRun, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

func discoverManifests(repoRoot, fleet string) ([]string, error) {
	fleet = strings.TrimSpace(fleet)
	if fleet != "" {
		if err := configrepo.ValidateFleetName(fleet); err != nil {
			return nil, fmt.Errorf("fleet: %w", err)
		}
	}

	var out []string
	fleetsDir := filepath.Join(repoRoot, "fleets")
	fleetEntries, err := os.ReadDir(fleetsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range fleetEntries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if fleet != "" && name != fleet {
			continue
		}
		manifest := filepath.ToSlash(filepath.Join("fleets", name, "manifest.yaml"))
		if fileExists(filepath.Join(repoRoot, filepath.FromSlash(manifest))) {
			out = append(out, manifest)
		}
	}

	endpointsDir := filepath.Join(repoRoot, "endpoints")
	endpointEntries, err := os.ReadDir(endpointsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range endpointEntries {
		if !entry.IsDir() {
			continue
		}
		manifest := filepath.ToSlash(filepath.Join("endpoints", entry.Name(), "manifest.yaml"))
		if !fileExists(filepath.Join(repoRoot, filepath.FromSlash(manifest))) {
			continue
		}
		if fleet != "" {
			ok, err := manifestExtendsFleet(repoRoot, manifest, fleet)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", manifest, err)
			}
			if !ok {
				continue
			}
		}
		out = append(out, manifest)
	}
	return out, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func composeDesiredOne(repoRoot, manifestRel string, write, compare, dryRun bool, res *Result) error {
	state, err := composeManifest(repoRoot, manifestRel)
	if err != nil {
		res.Issues = append(res.Issues, Issue{Path: manifestRel, Message: err.Error()})
		return nil
	}
	if err := configrepo.ValidateState(state, desiredPathForManifest(manifestRel)); err != nil {
		res.Issues = append(res.Issues, Issue{Path: manifestRel, Message: err.Error()})
		return nil
	}

	generated, err := marshalState(state)
	if err != nil {
		res.Issues = append(res.Issues, Issue{Path: manifestRel, Message: err.Error()})
		return nil
	}

	artifactRel := desiredPathForManifest(manifestRel)
	return finalizeArtifact(repoRoot, artifactRel, generated, write, compare, dryRun, lineDiff, res)
}

func composeCronOne(repoRoot, manifestRel string, write, compare, dryRun bool, res *Result) error {
	state, err := composeCronManifest(repoRoot, manifestRel)
	if err != nil {
		res.Issues = append(res.Issues, Issue{Path: manifestRel, Message: err.Error()})
		return nil
	}
	resolved, err := croncatalog.Resolve(repoRoot, state)
	if err != nil {
		res.Issues = append(res.Issues, Issue{Path: manifestRel, Message: err.Error()})
		return nil
	}
	if err := configrepo.ValidateCronState(resolved, cronsPathForManifest(manifestRel)); err != nil {
		res.Issues = append(res.Issues, Issue{Path: manifestRel, Message: err.Error()})
		return nil
	}

	generated, err := marshalCronState(state)
	if err != nil {
		res.Issues = append(res.Issues, Issue{Path: manifestRel, Message: err.Error()})
		return nil
	}

	artifactRel := cronsPathForManifest(manifestRel)
	return finalizeArtifact(repoRoot, artifactRel, generated, write, compare, dryRun, lineDiffCron, res)
}

func finalizeArtifact(
	repoRoot, artifactRel string,
	generated []byte,
	write, compare, dryRun bool,
	diffFn func(string, []byte, []byte) string,
	res *Result,
) error {
	artifactPath := filepath.Join(repoRoot, filepath.FromSlash(artifactRel))
	if compare {
		existing, err := os.ReadFile(artifactPath)
		if err != nil {
			if os.IsNotExist(err) {
				res.Stale = append(res.Stale, artifactRel)
				if dryRun {
					res.Diffs = append(res.Diffs, Diff{
						Path: artifactRel,
						Text: diffFn(artifactRel, nil, generated),
					})
				}
				return nil
			}
			res.Issues = append(res.Issues, Issue{Path: artifactRel, Message: err.Error()})
			return nil
		}
		if diffText := diffFn(artifactRel, existing, generated); diffText != "" {
			res.Stale = append(res.Stale, artifactRel)
			if dryRun {
				res.Diffs = append(res.Diffs, Diff{Path: artifactRel, Text: diffText})
			}
			return nil
		}
		res.OK = append(res.OK, artifactRel)
		return nil
	}

	if err := os.WriteFile(artifactPath, generated, 0o644); err != nil { // #nosec G306 -- public config artifact
		res.Issues = append(res.Issues, Issue{Path: artifactRel, Message: err.Error()})
		return nil
	}
	res.Written = append(res.Written, artifactRel)
	return nil
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
