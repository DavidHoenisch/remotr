package configrepo

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

func findManifestInTree(repoRoot, dirRel string) (string, error) {
	dirRel = filepath.ToSlash(strings.TrimSpace(dirRel))
	dirPath := filepath.Join(repoRoot, filepath.FromSlash(dirRel))
	info, err := os.Stat(dirPath)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", dirRel, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", dirRel)
	}
	var manifests []string
	err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		kind, err := parseFileKind(repoRoot, rel)
		if err != nil {
			return nil
		}
		if kind == types.KindManifest {
			manifests = append(manifests, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	switch len(manifests) {
	case 0:
		return "", fmt.Errorf("no kind: manifest found under %q", dirRel)
	case 1:
		return manifests[0], nil
	default:
		return "", fmt.Errorf("multiple kind: manifest files under %q: %s", dirRel, strings.Join(manifests, ", "))
	}
}

func parseFileKind(repoRoot, relPath string) (types.Kind, error) {
	raw, err := readRepoFile(repoRoot, relPath)
	if err != nil {
		return "", err
	}
	var head struct {
		Kind types.Kind `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &head); err != nil {
		return "", err
	}
	kind := types.Kind(strings.TrimSpace(head.Kind.String()))
	if kind == "" {
		return "", fmt.Errorf("missing kind")
	}
	return kind, nil
}

func validateKind(data []byte, want types.Kind) error {
	var head struct {
		Kind types.Kind `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if strings.TrimSpace(head.Kind.String()) == "" {
		return fmt.Errorf("file missing required 'kind' field")
	}
	if head.Kind != want {
		return fmt.Errorf("want kind %s, got %q", want, head.Kind)
	}
	return nil
}

func validateManifestFile(repoRoot, relPath string) error {
	raw, err := readRepoFile(repoRoot, relPath)
	if err != nil {
		return err
	}
	if err := validateKind(raw, types.KindManifest); err != nil {
		return err
	}
	var m struct {
		Kind types.Kind `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	return nil
}

func validateModuleFile(repoRoot, relPath string) ([]models.Diagnostic, error) {
	raw, err := readRepoFile(repoRoot, relPath)
	if err != nil {
		return nil, err
	}
	if err := validateKind(raw, types.KindModule); err != nil {
		return nil, err
	}
	state, diagnostics, err := models.ParseStateWithDiagnostics(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse module: %w", err)
	}
	if err := validateSourceModuleState(state, relPath); err != nil {
		return nil, err
	}
	return diagnostics, nil
}

func validateApplicationFile(repoRoot, relPath string) error {
	raw, err := readRepoFile(repoRoot, relPath)
	if err != nil {
		return err
	}
	return validateKind(raw, types.KindApplication)
}

func validateCronsSourceFile(repoRoot, relPath string) error {
	raw, err := readRepoFile(repoRoot, relPath)
	if err != nil {
		return err
	}
	if err := validateKind(raw, types.KindCrons); err != nil {
		return err
	}
	var f struct {
		Crons []models.CronJob `yaml:"crons"`
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("parse crons file: %w", err)
	}
	if len(f.Crons) == 0 {
		return fmt.Errorf("crons file has no cron definitions")
	}
	return nil
}

func readRepoFile(repoRoot, relPath string) ([]byte, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	return os.ReadFile(path)
}

func validateSharedModules(repoRoot string, res *ValidationResult) {
	modulesDir := filepath.Join(repoRoot, "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		res.Issues = append(res.Issues, ValidationIssue{Path: modulesDir, Message: err.Error()})
		return
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ".yaml") {
			continue
		}
		rel := filepath.Join("modules", ent.Name())
		diagnostics, err := validateModuleFile(repoRoot, rel)
		if err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
			continue
		}
		for _, diagnostic := range diagnostics {
			res.Diagnostics = append(res.Diagnostics, ValidationDiagnostic{
				Path: rel, Code: diagnostic.Code, Message: diagnostic.Message,
			})
		}
		res.OK = append(res.OK, rel)
	}
}

func validateApplicationsDir(repoRoot string, res *ValidationResult) {
	appsDir := filepath.Join(repoRoot, "applications")
	err := filepath.WalkDir(appsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".yaml") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := validateApplicationFile(repoRoot, rel); err != nil {
			res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
			return nil
		}
		res.OK = append(res.OK, rel)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		res.Issues = append(res.Issues, ValidationIssue{Path: appsDir, Message: err.Error()})
	}
}
