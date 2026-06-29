package configcompose

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

// CronsFile is a kind: crons source document.
type CronsFile struct {
	Kind  types.Kind       `yaml:"kind"`
	Crons []models.CronJob `yaml:"crons"`
}

func parseCronsFile(data []byte) (CronsFile, error) {
	var f CronsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return CronsFile{}, err
	}
	if f.Kind != types.KindCrons {
		return CronsFile{}, fmt.Errorf("want kind %s, got %q", types.KindCrons, f.Kind)
	}
	return f, nil
}

func loadCronsFile(repoRoot, relPath string) (CronsFile, error) {
	data, err := readRepoRelative(repoRoot, relPath)
	if err != nil {
		return CronsFile{}, err
	}
	f, err := parseCronsFile(data)
	if err != nil {
		return CronsFile{}, fmt.Errorf("parse crons file %q: %w", relPath, err)
	}
	return f, nil
}

func composeCronsFromManifest(repoRoot, manifestRel string) (models.CronState, error) {
	merged, err := resolveManifestChain(repoRoot, manifestRel, map[string]struct{}{})
	if err != nil {
		return models.CronState{}, err
	}
	if len(merged.Crons) == 0 {
		return models.CronState{}, nil
	}

	manifestDir := filepath.Dir(manifestRel)
	cronPaths, err := resolveCronRefs(repoRoot, manifestDir, merged.Crons)
	if err != nil {
		return models.CronState{}, fmt.Errorf("manifest %q: %w", manifestRel, err)
	}

	var jobs []models.CronJob
	seen := map[string]struct{}{}
	for _, cronPath := range cronPaths {
		file, err := loadCronsFile(repoRoot, cronPath)
		if err != nil {
			return models.CronState{}, err
		}
		for _, job := range file.Crons {
			name := strings.TrimSpace(job.Name)
			use := strings.TrimSpace(job.Use)
			if name == "" && use == "" {
				return models.CronState{}, fmt.Errorf("crons file %q: job missing name and use", cronPath)
			}
			key := name
			if key == "" {
				key = "use:" + use
			}
			if _, dup := seen[key]; dup {
				return models.CronState{}, fmt.Errorf("duplicate cron %q (file %q)", key, cronPath)
			}
			seen[key] = struct{}{}
			jobs = append(jobs, job)
		}
	}
	if len(jobs) == 0 {
		return models.CronState{}, fmt.Errorf("manifest %q: composed crons are empty", manifestRel)
	}
	return models.CronState{Crons: jobs}, nil
}
