package resolve

import (
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// ResolvedState is desired state after in-document targeting.
type ResolvedState struct {
	Configurations []models.Configuration
}

// Resolve filters configurations and nested resources for local facts.
func Resolve(state models.State, f facts.Facts) ResolvedState {
	out := ResolvedState{}
	for _, cfg := range state.Configurations {
		if !matchesDistro(cfg.TargetDistros, f.Distro) {
			continue
		}
		if !matchesArch(cfg.TargetArch, f.Arch) {
			continue
		}
		resolved := models.Configuration{
			Name:        cfg.Name,
			Description: cfg.Description,
			LastUpdated: cfg.LastUpdated,
		}
		pm := facts.PackageManagerForDistro(f.Distro)
		for _, pkg := range cfg.Packages {
			if pkg.PM != "" && pkg.PM != pm {
				continue
			}
			if pkg.Arch != "" && pkg.Arch != f.Arch {
				continue
			}
			resolved.Packages = append(resolved.Packages, pkg)
		}
		resolved.Files = append(resolved.Files, cfg.Files...)
		resolved.UserFiles = append(resolved.UserFiles, cfg.UserFiles...)
		resolved.Downloads = append(resolved.Downloads, cfg.Downloads...)
		resolved.Users = append(resolved.Users, cfg.Users...)
		resolved.Systemd = append(resolved.Systemd, cfg.Systemd...)
		resolved.SystemdUser = append(resolved.SystemdUser, cfg.SystemdUser...)
		resolved.Bootstrap = append(resolved.Bootstrap, cfg.Bootstrap...)
		resolved.AgentInstall = append(resolved.AgentInstall, cfg.AgentInstall...)
		resolved.Commands = append(resolved.Commands, cfg.Commands...)
		out.Configurations = append(out.Configurations, resolved)
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
