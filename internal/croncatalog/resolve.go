package croncatalog

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/safepath"
	"github.com/DavidHoenisch/remotr/internal/types"
)

//go:embed builtin/system-upgrade.yaml
var systemUpgradeYAML []byte

//go:embed builtin/clamav-scan.yaml
var clamavScanYAML []byte

var embeddedTemplates = map[string][]byte{
	"system-upgrade":        systemUpgradeYAML,
	"system-upgrade-debian": systemUpgradeYAML,
	"system-upgrade-arch":   systemUpgradeYAML,
	"clamav-scan":           clamavScanYAML,
	"clamav-scan-debian":    clamavScanYAML,
	"clamav-scan-arch":      clamavScanYAML,
}

// Resolve expands use: references in state and returns fully materialized cron jobs.
func Resolve(repoRoot string, state models.CronState) (models.CronState, error) {
	if len(state.Crons) == 0 {
		return state, nil
	}
	out := models.CronState{Crons: make([]models.CronJob, 0, len(state.Crons))}
	for _, job := range state.Crons {
		expanded, err := expandJob(repoRoot, job)
		if err != nil {
			return models.CronState{}, err
		}
		out.Crons = append(out.Crons, expanded...)
	}
	return out, nil
}

func expandJob(repoRoot string, job models.CronJob) ([]models.CronJob, error) {
	use := strings.TrimSpace(job.Use)
	if use == "" {
		if !job.HasResources() {
			name := strings.TrimSpace(job.Name)
			if name == "" {
				return nil, fmt.Errorf("cron missing name")
			}
			return nil, fmt.Errorf("cron %q: use or resources required", name)
		}
		return []models.CronJob{job}, nil
	}

	templateJobs, err := loadTemplate(repoRoot, use)
	if err != nil {
		refName := strings.TrimSpace(job.Name)
		if refName == "" {
			refName = use
		}
		return nil, fmt.Errorf("cron %q: %w", refName, err)
	}
	if len(templateJobs) == 0 {
		return nil, fmt.Errorf("template %q defines no jobs", use)
	}

	selected := selectTemplateJobs(use, templateJobs)
	if len(selected) == 0 {
		return nil, fmt.Errorf("template %q matched no jobs", use)
	}

	out := make([]models.CronJob, 0, len(selected))
	for _, tmpl := range selected {
		merged := mergeJob(tmpl, job)
		if strings.TrimSpace(merged.Name) == "" {
			return nil, fmt.Errorf("cron from %q: resolved job missing name", use)
		}
		if strings.TrimSpace(merged.Schedule) == "" {
			return nil, fmt.Errorf("cron %q: schedule required", merged.Name)
		}
		if !merged.HasResources() {
			return nil, fmt.Errorf("cron %q: no resources after resolving %q", merged.Name, use)
		}
		out = append(out, merged)
	}
	return out, nil
}

func selectTemplateJobs(use string, jobs []models.CronJob) []models.CronJob {
	ref := strings.TrimPrefix(strings.TrimSpace(use), "builtin/")
	switch ref {
	case "system-upgrade-debian":
		return filterJobs(jobs, "system-upgrade-debian")
	case "system-upgrade-arch":
		return filterJobs(jobs, "system-upgrade-arch")
	case "clamav-scan-debian":
		return filterJobs(jobs, "clamav-scan-debian")
	case "clamav-scan-arch":
		return filterJobs(jobs, "clamav-scan-arch")
	default:
		return jobs
	}
}

func filterJobs(jobs []models.CronJob, name string) []models.CronJob {
	for _, job := range jobs {
		if job.Name == name {
			return []models.CronJob{job}
		}
	}
	return nil
}

func loadTemplate(repoRoot, use string) ([]models.CronJob, error) {
	use = strings.TrimSpace(use)
	switch {
	case strings.HasPrefix(use, "builtin/"):
		return loadEmbedded(strings.TrimPrefix(use, "builtin/"))
	case strings.HasPrefix(use, "crons/"):
		return loadRepoTemplate(repoRoot, use)
	default:
		return nil, fmt.Errorf("use %q: must start with builtin/ or crons/", use)
	}
}

func loadEmbedded(name string) ([]models.CronJob, error) {
	raw, ok := embeddedTemplates[name]
	if !ok {
		return nil, fmt.Errorf("unknown builtin template %q", name)
	}
	state, err := models.ParseCronState(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse builtin %q: %w", name, err)
	}
	return state.Crons, nil
}

func loadRepoTemplate(repoRoot, rel string) ([]models.CronJob, error) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil, fmt.Errorf("repository root required for %q", rel)
	}
	rel = strings.Trim(rel, "/")
	if !strings.HasSuffix(strings.ToLower(rel), ".yaml") {
		rel += ".yaml"
	}
	parts := strings.Split(rel, "/")
	data, err := safepath.ReadUnderRoot(repoRoot, parts...)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("template not found: %s", rel)
		}
		return nil, err
	}
	state, err := models.ParseCronState(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", rel, err)
	}
	return state.Crons, nil
}

func mergeJob(base, overrides models.CronJob) models.CronJob {
	out := base
	if n := strings.TrimSpace(overrides.Name); n != "" {
		out.Name = n
	}
	if d := strings.TrimSpace(overrides.Description); d != "" {
		out.Description = d
	}
	if s := strings.TrimSpace(overrides.Schedule); s != "" {
		out.Schedule = s
	}
	if tz := strings.TrimSpace(overrides.Timezone); tz != "" {
		out.Timezone = tz
	}
	if len(overrides.TargetDistros) > 0 {
		out.TargetDistros = append([]types.Distro(nil), overrides.TargetDistros...)
	}
	if len(overrides.TargetArch) > 0 {
		out.TargetArch = append([]types.Architecture(nil), overrides.TargetArch...)
	}
	out.Use = ""
	mergeResources(&out, overrides)
	return out
}

func mergeResources(out *models.CronJob, overrides models.CronJob) {
	if len(overrides.Packages) > 0 {
		out.Packages = overrides.Packages
	}
	if len(overrides.Files) > 0 {
		out.Files = overrides.Files
	}
	if len(overrides.Directories) > 0 {
		out.Directories = overrides.Directories
	}
	if len(overrides.Links) > 0 {
		out.Links = overrides.Links
	}
	if len(overrides.Groups) > 0 {
		out.Groups = overrides.Groups
	}
	if len(overrides.UserFiles) > 0 {
		out.UserFiles = overrides.UserFiles
	}
	if len(overrides.Downloads) > 0 {
		out.Downloads = overrides.Downloads
	}
	if len(overrides.Users) > 0 {
		out.Users = overrides.Users
	}
	if len(overrides.Systemd) > 0 {
		out.Systemd = overrides.Systemd
	}
	if len(overrides.SystemdUser) > 0 {
		out.SystemdUser = overrides.SystemdUser
	}
	if len(overrides.Bootstrap) > 0 {
		out.Bootstrap = overrides.Bootstrap
	}
	if len(overrides.AgentInstall) > 0 {
		out.AgentInstall = overrides.AgentInstall
	}
	if len(overrides.Commands) > 0 {
		out.Commands = overrides.Commands
	}
}

// BuiltinNames returns builtin template identifiers.
func BuiltinNames() []string {
	return []string{
		"system-upgrade", "system-upgrade-debian", "system-upgrade-arch",
		"clamav-scan", "clamav-scan-debian", "clamav-scan-arch",
	}
}

func init() {
	for name, raw := range embeddedTemplates {
		if _, err := models.ParseCronState(bytes.NewReader(raw)); err != nil {
			panic("invalid embedded cron template " + name + ": " + err.Error())
		}
	}
}
