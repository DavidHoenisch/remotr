package cronresolve

import (
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// FilterJobs returns cron jobs that apply to local facts.
func FilterJobs(jobs []models.CronJob, f facts.Facts) []models.CronJob {
	out := make([]models.CronJob, 0, len(jobs))
	for _, job := range jobs {
		if !matchesDistro(job.TargetDistros, f.Distro) {
			continue
		}
		if !matchesArch(job.TargetArch, f.Arch) {
			continue
		}
		out = append(out, job)
	}
	return out
}

// FilterJobsByLabels filters cron jobs using endpoint labels (distro, arch).
func FilterJobsByLabels(jobs []models.CronJob, labels map[string]string) []models.CronJob {
	distro := types.Distro(labels["distro"])
	arch := types.Architecture(labels["arch"])
	if distro == "" && arch == "" {
		return jobs
	}
	out := make([]models.CronJob, 0, len(jobs))
	for _, job := range jobs {
		if distro != "" && !matchesDistro(job.TargetDistros, distro) {
			continue
		}
		if arch != "" && !matchesArch(job.TargetArch, arch) {
			continue
		}
		out = append(out, job)
	}
	return out
}

func matchesDistro(targets []types.Distro, d types.Distro) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if t == d {
			return true
		}
	}
	return false
}

func matchesArch(targets []types.Architecture, a types.Architecture) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if t == a {
			return true
		}
	}
	return false
}
