package configcompose

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

// CronManifest is the source document for composing a crons.yaml artifact.
type CronManifest struct {
	Extends   string           `yaml:"extends,omitempty"`
	Modules   []string         `yaml:"modules,omitempty"`
	Overrides []models.CronJob `yaml:"overrides,omitempty"`
}

func parseCronManifest(data []byte) (CronManifest, error) {
	var m CronManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return CronManifest{}, err
	}
	return m, nil
}

func loadCronManifest(repoRoot, relPath string) (CronManifest, error) {
	data, err := readRepoRelative(repoRoot, relPath)
	if err != nil {
		return CronManifest{}, err
	}
	return parseCronManifest(data)
}

func resolveCronManifestChain(repoRoot, manifestRel string, seen map[string]struct{}) (CronManifest, error) {
	manifestRel = normalizeRelPath(manifestRel)
	if manifestRel == "" {
		return CronManifest{}, fmt.Errorf("empty crons manifest path")
	}
	if _, ok := seen[manifestRel]; ok {
		return CronManifest{}, fmt.Errorf("extends cycle at %q", manifestRel)
	}
	if len(seen) >= maxExtendsDepth {
		return CronManifest{}, fmt.Errorf("extends depth exceeds %d at %q", maxExtendsDepth, manifestRel)
	}
	seen[manifestRel] = struct{}{}

	m, err := loadCronManifest(repoRoot, manifestRel)
	if err != nil {
		return CronManifest{}, fmt.Errorf("load crons manifest %q: %w", manifestRel, err)
	}

	var merged CronManifest
	if ext := strings.TrimSpace(m.Extends); ext != "" {
		parent, err := resolveCronManifestChain(repoRoot, ext, seen)
		if err != nil {
			return CronManifest{}, err
		}
		merged.Modules = append(merged.Modules, parent.Modules...)
		merged.Overrides = append(merged.Overrides, parent.Overrides...)
	}
	merged.Modules = append(merged.Modules, m.Modules...)
	merged.Overrides = append(merged.Overrides, m.Overrides...)
	return merged, nil
}

func loadCronModule(repoRoot, modulePath string) (models.CronState, error) {
	data, err := readRepoRelative(repoRoot, modulePath)
	if err != nil {
		return models.CronState{}, fmt.Errorf("read cron module %q: %w", modulePath, err)
	}
	state, err := models.ParseCronState(bytes.NewReader(data))
	if err != nil {
		return models.CronState{}, fmt.Errorf("parse cron module %q: %w", modulePath, err)
	}
	return state, nil
}

func mergeCronJobs(base []models.CronJob, overrides []models.CronJob) ([]models.CronJob, error) {
	byName := make(map[string]int, len(base))
	out := append([]models.CronJob(nil), base...)
	for i, job := range out {
		name := strings.TrimSpace(job.Name)
		if name == "" {
			if strings.TrimSpace(job.Use) != "" {
				continue
			}
			return nil, fmt.Errorf("cron missing name")
		}
		byName[name] = i
	}

	for _, override := range overrides {
		name := strings.TrimSpace(override.Name)
		if name == "" {
			return nil, fmt.Errorf("cron override missing name")
		}
		idx, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("cron override %q: no matching job", name)
		}
		out[idx] = mergeCronJob(out[idx], override)
	}
	return out, nil
}

func mergeCronJob(base, override models.CronJob) models.CronJob {
	out := base
	if n := strings.TrimSpace(override.Name); n != "" {
		out.Name = n
	}
	if d := strings.TrimSpace(override.Description); d != "" {
		out.Description = d
	}
	if s := strings.TrimSpace(override.Schedule); s != "" {
		out.Schedule = s
	}
	if tz := strings.TrimSpace(override.Timezone); tz != "" {
		out.Timezone = tz
	}
	if len(override.TargetDistros) > 0 {
		out.TargetDistros = append([]types.Distro(nil), override.TargetDistros...)
	}
	if len(override.TargetArch) > 0 {
		out.TargetArch = append([]types.Architecture(nil), override.TargetArch...)
	}
	if u := strings.TrimSpace(override.Use); u != "" {
		out.Use = u
	}
	if len(override.Packages) > 0 {
		out.Packages = override.Packages
	}
	if len(override.Files) > 0 {
		out.Files = override.Files
	}
	if len(override.UserFiles) > 0 {
		out.UserFiles = override.UserFiles
	}
	if len(override.Downloads) > 0 {
		out.Downloads = override.Downloads
	}
	if len(override.Users) > 0 {
		out.Users = override.Users
	}
	if len(override.Systemd) > 0 {
		out.Systemd = override.Systemd
	}
	if len(override.SystemdUser) > 0 {
		out.SystemdUser = override.SystemdUser
	}
	if len(override.Bootstrap) > 0 {
		out.Bootstrap = override.Bootstrap
	}
	if len(override.AgentInstall) > 0 {
		out.AgentInstall = override.AgentInstall
	}
	if len(override.Commands) > 0 {
		out.Commands = override.Commands
	}
	return out
}

func composeCronManifest(repoRoot, manifestRel string) (models.CronState, error) {
	merged, err := resolveCronManifestChain(repoRoot, manifestRel, map[string]struct{}{})
	if err != nil {
		return models.CronState{}, err
	}
	if len(merged.Modules) == 0 && len(merged.Overrides) == 0 {
		return models.CronState{}, fmt.Errorf("crons manifest %q: no modules or overrides", manifestRel)
	}

	var jobs []models.CronJob
	seen := map[string]struct{}{}
	for _, modulePath := range merged.Modules {
		modulePath = normalizeRelPath(modulePath)
		if modulePath == "" {
			continue
		}
		state, err := loadCronModule(repoRoot, modulePath)
		if err != nil {
			return models.CronState{}, err
		}
		for _, job := range state.Crons {
			name := strings.TrimSpace(job.Name)
			use := strings.TrimSpace(job.Use)
			if name == "" && use == "" {
				return models.CronState{}, fmt.Errorf("cron module %q: job missing name and use", modulePath)
			}
			key := name
			if key == "" {
				key = "use:" + use
			}
			if _, dup := seen[key]; dup {
				return models.CronState{}, fmt.Errorf("duplicate cron %q (module %q)", key, modulePath)
			}
			seen[key] = struct{}{}
			jobs = append(jobs, job)
		}
	}

	jobs, err = mergeCronJobs(jobs, merged.Overrides)
	if err != nil {
		return models.CronState{}, fmt.Errorf("crons manifest %q: %w", manifestRel, err)
	}
	if len(jobs) == 0 {
		return models.CronState{}, fmt.Errorf("crons manifest %q: composed state is empty", manifestRel)
	}
	return models.CronState{Crons: jobs}, nil
}

func marshalCronState(state models.CronState) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(state); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cronsPathForManifest(manifestRel string) string {
	dir := filepath.Dir(manifestRel)
	return filepath.ToSlash(filepath.Join(dir, "crons.yaml"))
}

func cronManifestExtendsFleet(repoRoot, manifestRel, fleet string) (bool, error) {
	target := normalizeRelPath(filepath.Join("fleets", fleet, "crons.manifest.yaml"))
	seen := map[string]struct{}{}
	for cur := normalizeRelPath(manifestRel); cur != ""; {
		if cur == target {
			return true, nil
		}
		if _, ok := seen[cur]; ok {
			return false, fmt.Errorf("extends cycle at %q", cur)
		}
		seen[cur] = struct{}{}
		m, err := loadCronManifest(repoRoot, cur)
		if err != nil {
			return false, err
		}
		ext := strings.TrimSpace(m.Extends)
		if ext == "" {
			return false, nil
		}
		cur = normalizeRelPath(ext)
	}
	return false, nil
}

func discoverCronManifests(repoRoot, fleet string) ([]string, error) {
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
		manifest := filepath.ToSlash(filepath.Join("fleets", name, "crons.manifest.yaml"))
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
		manifest := filepath.ToSlash(filepath.Join("endpoints", entry.Name(), "crons.manifest.yaml"))
		if !fileExists(filepath.Join(repoRoot, filepath.FromSlash(manifest))) {
			continue
		}
		if fleet != "" {
			ok, err := cronManifestExtendsFleet(repoRoot, manifest, fleet)
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
