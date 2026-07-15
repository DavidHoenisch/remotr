package models

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/schedule"
	"github.com/DavidHoenisch/remotr/internal/secretref"
)

// ScheduleBackend identifies the endpoint-local scheduler that owns a
// persistent schedule. It is deliberately unrelated to server CronJob work.
type ScheduleBackend string

const (
	ScheduleBackendCron         ScheduleBackend = "cron"
	ScheduleBackendSystemdTimer ScheduleBackend = "systemd-timer"
)

// ScheduleOverlap controls whether a second occurrence may run while the
// previous occurrence is still active.
type ScheduleOverlap string

const (
	ScheduleOverlapAllow  ScheduleOverlap = "allow"
	ScheduleOverlapForbid ScheduleOverlap = "forbid"
)

// ScheduleEnvironment declares one public literal or protected secret
// reference for a scheduled process. The two value sources are exclusive.
type ScheduleEnvironment struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value,omitempty"`
	SecretRef string `yaml:"secretRef,omitempty"`
}

// EndpointScheduleResource manages a persistent local cron entry or systemd
// timer. It does not participate in the server-dispatched CronJob API.
type EndpointScheduleResource struct {
	ResourceMeta     `yaml:",inline"`
	Name             string                `yaml:"name"`
	Backend          ScheduleBackend       `yaml:"backend"`
	Schedule         string                `yaml:"schedule,omitempty"`
	User             string                `yaml:"user,omitempty"`
	Argv             []string              `yaml:"argv,omitempty"`
	Shell            string                `yaml:"shell,omitempty"`
	WorkingDirectory string                `yaml:"workingDirectory,omitempty"`
	Environment      []ScheduleEnvironment `yaml:"environment,omitempty"`
	Timeout          string                `yaml:"timeout,omitempty"`
	Overlap          ScheduleOverlap       `yaml:"overlap,omitempty"`
	Persistent       *bool                 `yaml:"persistent,omitempty"`
}

var (
	endpointScheduleName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	scheduleUserName     = regexp.MustCompile(`^[a-z_][a-z0-9_-]*[$]?$`)
	scheduleEnvName      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Validate checks the portable resource contract before backend selection.
func (r EndpointScheduleResource) Validate() error {
	if !endpointScheduleName.MatchString(r.Name) {
		return fmt.Errorf("endpoint schedule name %q is invalid", r.Name)
	}
	switch r.Backend {
	case ScheduleBackendCron, ScheduleBackendSystemdTimer:
	default:
		return fmt.Errorf("endpoint schedule backend %q is invalid", r.Backend)
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleDisabled && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("endpoint schedule lifecycle %q is unsupported", r.Lifecycle)
	}
	if r.Backend == ScheduleBackendCron {
		if r.Persistent != nil {
			return fmt.Errorf("persistent is supported only by systemd-timer")
		}
		if r.Lifecycle != LifecycleAbsent {
			if err := schedule.ValidateCronExpression(r.Schedule); err != nil {
				return err
			}
		}
	} else if r.Lifecycle != LifecycleAbsent {
		if strings.TrimSpace(r.Schedule) == "" || strings.ContainsAny(r.Schedule, "\x00\r\n") {
			return fmt.Errorf("systemd timer schedule is required and must be one line")
		}
		if r.Persistent == nil {
			return fmt.Errorf("systemd-timer requires explicit persistent missed-run policy")
		}
		if r.Overlap == ScheduleOverlapAllow {
			return fmt.Errorf("systemd-timer does not support concurrent overlap")
		}
	}
	if r.Lifecycle == LifecycleAbsent {
		return r.validateAbsent()
	}
	if !scheduleUserName.MatchString(r.User) {
		return fmt.Errorf("endpoint schedule user %q is invalid", r.User)
	}
	if (len(r.Argv) == 0) == (strings.TrimSpace(r.Shell) == "") {
		return fmt.Errorf("exactly one of argv or shell is required")
	}
	if len(r.Argv) > 0 {
		if !filepath.IsAbs(r.Argv[0]) || filepath.Clean(r.Argv[0]) != r.Argv[0] {
			return fmt.Errorf("argv executable %q must be a clean absolute path", r.Argv[0])
		}
		for _, arg := range r.Argv {
			if strings.ContainsRune(arg, '\x00') {
				return fmt.Errorf("argv must not contain NUL")
			}
		}
	}
	if strings.ContainsAny(r.Shell, "\x00\r\n") {
		return fmt.Errorf("shell command must be one line")
	}
	if r.WorkingDirectory != "" && (!filepath.IsAbs(r.WorkingDirectory) || filepath.Clean(r.WorkingDirectory) != r.WorkingDirectory) {
		return fmt.Errorf("workingDirectory %q must be a clean absolute path", r.WorkingDirectory)
	}
	if err := r.validateEnvironment(); err != nil {
		return err
	}
	if r.Timeout != "" {
		timeout, err := time.ParseDuration(r.Timeout)
		if err != nil || timeout <= 0 {
			return fmt.Errorf("timeout must be a positive duration")
		}
	}
	if r.Overlap != "" && r.Overlap != ScheduleOverlapAllow && r.Overlap != ScheduleOverlapForbid {
		return fmt.Errorf("endpoint schedule overlap %q is invalid", r.Overlap)
	}
	return nil
}

func (r EndpointScheduleResource) validateAbsent() error {
	if r.Schedule != "" || r.User != "" || len(r.Argv) != 0 || r.Shell != "" || r.WorkingDirectory != "" || len(r.Environment) != 0 || r.Timeout != "" || r.Overlap != "" || r.Persistent != nil {
		return fmt.Errorf("absent endpoint schedule may declare only name, backend, lifecycle, and shared metadata")
	}
	return nil
}

func (r EndpointScheduleResource) validateEnvironment() error {
	if len(r.Environment) > 64 {
		return fmt.Errorf("endpoint schedule environment exceeds 64 entries")
	}
	seen := make(map[string]struct{}, len(r.Environment))
	for _, variable := range r.Environment {
		if !scheduleEnvName.MatchString(variable.Name) {
			return fmt.Errorf("environment name %q is invalid", variable.Name)
		}
		if _, exists := seen[variable.Name]; exists {
			return fmt.Errorf("environment name %q is duplicated", variable.Name)
		}
		seen[variable.Name] = struct{}{}
		if (variable.Value == "") == (variable.SecretRef == "") {
			return fmt.Errorf("environment %q requires exactly one of value or secretRef", variable.Name)
		}
		if strings.ContainsAny(variable.Value, "\x00\r\n") {
			return fmt.Errorf("environment %q value must be one line", variable.Name)
		}
		if variable.SecretRef != "" {
			if err := secretref.Validate(variable.SecretRef); err != nil {
				return fmt.Errorf("environment %q secretRef: %w", variable.Name, err)
			}
		}
	}
	return nil
}
