package configrepo

import (
	"fmt"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/schedule"
)

// ValidateCronState checks a composed crons artifact after use: resolution.
func ValidateCronState(state models.CronState, path string) error {
	return validateCronState(state, path, false)
}

// ValidateComposedCronState checks a pre-resolution composed crons artifact (use: refs allowed).
func ValidateComposedCronState(state models.CronState, path string) error {
	return validateCronState(state, path, true)
}

func validateCronState(state models.CronState, path string, allowUnresolvedUse bool) error {
	if len(state.Crons) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(state.Crons))
	for _, job := range state.Crons {
		name := strings.TrimSpace(job.Name)
		use := strings.TrimSpace(job.Use)
		if name == "" && use == "" {
			return fmt.Errorf("cron missing name")
		}
		key := name
		if key == "" {
			key = "use:" + use
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("duplicate cron %q", key)
		}
		seen[key] = struct{}{}

		if use != "" && !allowUnresolvedUse {
			return fmt.Errorf("cron %q: unresolved use reference", name)
		}
		if use != "" {
			if strings.TrimSpace(job.Schedule) == "" {
				return fmt.Errorf("cron %q: schedule required", key)
			}
			if err := schedule.ValidateCronExpression(job.Schedule); err != nil {
				return fmt.Errorf("cron %q: %w", key, err)
			}
			continue
		}
		if err := schedule.ValidateCronExpression(job.Schedule); err != nil {
			return fmt.Errorf("cron %q: %w", name, err)
		}
		if tz := strings.TrimSpace(job.Timezone); tz != "" && tz != "UTC" {
			if _, err := time.LoadLocation(tz); err != nil {
				return fmt.Errorf("cron %q: invalid timezone %q", name, tz)
			}
		}
		cfg := job.ToConfiguration()
		if err := validatePackages(cfg, name); err != nil {
			return err
		}
		if err := validateFiles(cfg, name); err != nil {
			return err
		}
		if err := validateDirectories(cfg, name); err != nil {
			return err
		}
		if err := validateLinks(cfg, name); err != nil {
			return err
		}
		if err := validateGroups(cfg, name); err != nil {
			return err
		}
		if err := validateAuthorizedKeys(cfg, name); err != nil {
			return err
		}
		if err := validateUserFiles(cfg, name); err != nil {
			return err
		}
		if err := validateDownloads(cfg, name); err != nil {
			return err
		}
		if err := validateUsers(cfg, name); err != nil {
			return err
		}
		if err := validateSystemd(cfg, name); err != nil {
			return err
		}
		if err := validateSystemdUser(cfg, name); err != nil {
			return err
		}
		if err := validateBootstrap(cfg, name); err != nil {
			return err
		}
		if err := validateAgentInstall(cfg, name); err != nil {
			return err
		}
		if err := validateCommands(cfg, name); err != nil {
			return err
		}
		if !job.HasResources() {
			return fmt.Errorf("cron %q: no resources defined", name)
		}
	}
	return nil
}
