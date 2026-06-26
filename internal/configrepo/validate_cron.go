package configrepo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/croncatalog"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/schedule"
)

func validateFleetCrons(repoRoot, fleet string, res *ValidationResult) {
	rel := filepath.Join("fleets", fleet, "crons.yaml")
	path := filepath.Join(repoRoot, rel)
	if err := validateCronsFile(repoRoot, path, rel); err != nil {
		if os.IsNotExist(err) {
			return
		}
		res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
		return
	}
	res.OK = append(res.OK, rel)
}

func validateEndpointCrons(repoRoot, endpointID string, res *ValidationResult) {
	rel := filepath.Join("endpoints", endpointID, "crons.yaml")
	path := filepath.Join(repoRoot, rel)
	if err := validateCronsFile(repoRoot, path, rel); err != nil {
		if os.IsNotExist(err) {
			return
		}
		res.Issues = append(res.Issues, ValidationIssue{Path: rel, Message: err.Error()})
		return
	}
	res.OK = append(res.OK, rel)
}

func validateCronsFile(repoRoot, path, displayPath string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	state, err := models.ParseCronState(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse crons artifact: %w", err)
	}
	resolved, err := croncatalog.Resolve(repoRoot, state)
	if err != nil {
		return err
	}
	return validateCronState(resolved, displayPath)
}

// ValidateCronState checks a composed or hand-authored crons artifact after use: resolution.
func ValidateCronState(state models.CronState, path string) error {
	return validateCronState(state, path)
}

func validateCronState(state models.CronState, path string) error {
	if len(state.Crons) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(state.Crons))
	for _, job := range state.Crons {
		name := strings.TrimSpace(job.Name)
		if name == "" {
			return fmt.Errorf("cron missing name")
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("duplicate cron %q", name)
		}
		seen[name] = struct{}{}

		if strings.TrimSpace(job.Use) != "" {
			return fmt.Errorf("cron %q: unresolved use reference", name)
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
