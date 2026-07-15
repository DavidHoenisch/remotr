package models

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type LogrotateCadence string

const (
	LogrotateHourly  LogrotateCadence = "hourly"
	LogrotateDaily   LogrotateCadence = "daily"
	LogrotateWeekly  LogrotateCadence = "weekly"
	LogrotateMonthly LogrotateCadence = "monthly"
	LogrotateYearly  LogrotateCadence = "yearly"
)

var (
	logrotateName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)
	logrotateMode      = regexp.MustCompile(`^0?[0-7]{3}$`)
	logrotatePrincipal = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
)

type LogrotateCreate struct {
	Mode  string `yaml:"mode"`
	Owner string `yaml:"owner"`
	Group string `yaml:"group"`
}

type LogrotateScript struct {
	Command []string `yaml:"command"`
}

type LogrotateResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string           `yaml:"name"`
	Paths        []string         `yaml:"paths,omitempty"`
	Cadence      LogrotateCadence `yaml:"cadence,omitempty"`
	Retention    *int             `yaml:"retention,omitempty"`
	Compress     *bool            `yaml:"compress,omitempty"`
	Create       *LogrotateCreate `yaml:"create,omitempty"`
	Shared       *bool            `yaml:"sharedScripts,omitempty"`
	PreRotate    *LogrotateScript `yaml:"preRotate,omitempty"`
	PostRotate   *LogrotateScript `yaml:"postRotate,omitempty"`
	FirstAction  *LogrotateScript `yaml:"firstAction,omitempty"`
	LastAction   *LogrotateScript `yaml:"lastAction,omitempty"`
}

func (r LogrotateResource) Validate() error {
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if !logrotateName.MatchString(r.Name) {
		return fmt.Errorf("logrotate name must be a safe named-fragment identifier")
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("logrotate lifecycle must be present or absent")
	}
	if lifecycle == LifecycleAbsent {
		if r.hasSettings() {
			return fmt.Errorf("absent logrotate fragment must omit settings")
		}
		return nil
	}
	if len(r.Paths) == 0 {
		return fmt.Errorf("logrotate fragment requires at least one path")
	}
	seenPaths := make(map[string]struct{}, len(r.Paths))
	for _, path := range r.Paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, " \t\r\n\x00{}") {
			return fmt.Errorf("logrotate path %q must be clean, absolute, and config-safe", path)
		}
		if _, exists := seenPaths[path]; exists {
			return fmt.Errorf("logrotate path %q is duplicated", path)
		}
		seenPaths[path] = struct{}{}
	}
	switch r.Cadence {
	case "", LogrotateHourly, LogrotateDaily, LogrotateWeekly, LogrotateMonthly, LogrotateYearly:
	default:
		return fmt.Errorf("logrotate cadence %q is invalid", r.Cadence)
	}
	if r.Retention != nil && (*r.Retention < 0 || *r.Retention > 10000) {
		return fmt.Errorf("logrotate retention must be between 0 and 10000")
	}
	if r.Cadence == "" || r.Retention == nil {
		return fmt.Errorf("logrotate fragment requires cadence and retention")
	}
	if r.Create != nil {
		if !logrotateMode.MatchString(r.Create.Mode) {
			return fmt.Errorf("logrotate create mode must be a three- or four-digit octal string")
		}
		if !logrotatePrincipal.MatchString(r.Create.Owner) || !logrotatePrincipal.MatchString(r.Create.Group) {
			return fmt.Errorf("logrotate create owner and group must be safe account names")
		}
	}
	for name, script := range map[string]*LogrotateScript{
		"preRotate": r.PreRotate, "postRotate": r.PostRotate, "firstAction": r.FirstAction, "lastAction": r.LastAction,
	} {
		if err := validateLogrotateScript(name, script); err != nil {
			return err
		}
	}
	return nil
}

func (r LogrotateResource) hasSettings() bool {
	return len(r.Paths) > 0 || r.Cadence != "" || r.Retention != nil || r.Compress != nil || r.Create != nil ||
		r.Shared != nil || r.hasScripts()
}

func (r LogrotateResource) hasScripts() bool {
	return r.PreRotate != nil || r.PostRotate != nil || r.FirstAction != nil || r.LastAction != nil
}

func validateLogrotateScript(name string, script *LogrotateScript) error {
	if script == nil {
		return nil
	}
	if len(script.Command) == 0 || len(script.Command) > 64 || !filepath.IsAbs(script.Command[0]) {
		return fmt.Errorf("logrotate %s command requires an absolute executable and at most 64 argv entries", name)
	}
	length := 0
	for _, argument := range script.Command {
		length += len(argument)
		if argument == "" || strings.ContainsAny(argument, "\r\n\x00") {
			return fmt.Errorf("logrotate %s command contains an invalid argv entry", name)
		}
	}
	if length > 4096 {
		return fmt.Errorf("logrotate %s command exceeds 4096 argv bytes", name)
	}
	return nil
}
