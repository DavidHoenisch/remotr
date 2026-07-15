package models

import (
	"fmt"
	"regexp"
	"time"
)

type JournaldStorage string

const (
	JournaldStorageAuto       JournaldStorage = "auto"
	JournaldStorageVolatile   JournaldStorage = "volatile"
	JournaldStoragePersistent JournaldStorage = "persistent"
	JournaldStorageNone       JournaldStorage = "none"
)

var journaldName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)

type JournaldResource struct {
	ResourceMeta          `yaml:",inline"`
	Name                  string          `yaml:"name"`
	Storage               JournaldStorage `yaml:"storage,omitempty"`
	MaxRetention          string          `yaml:"maxRetention,omitempty"`
	SystemMaxUseBytes     *int64          `yaml:"systemMaxUseBytes,omitempty"`
	RuntimeMaxUseBytes    *int64          `yaml:"runtimeMaxUseBytes,omitempty"`
	RateLimitInterval     string          `yaml:"rateLimitInterval,omitempty"`
	RateLimitBurst        *int            `yaml:"rateLimitBurst,omitempty"`
	ForwardToSyslog       *bool           `yaml:"forwardToSyslog,omitempty"`
	ForwardToKernelBuffer *bool           `yaml:"forwardToKernelBuffer,omitempty"`
	ForwardToConsole      *bool           `yaml:"forwardToConsole,omitempty"`
	ForwardToWall         *bool           `yaml:"forwardToWall,omitempty"`
}

func (r JournaldResource) Validate() error {
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if !journaldName.MatchString(r.Name) {
		return fmt.Errorf("journald policy name must be a safe drop-in identifier")
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("journald lifecycle must be present or absent")
	}
	if lifecycle == LifecycleAbsent {
		if r.hasSettings() {
			return fmt.Errorf("absent journald policy must omit settings")
		}
		return nil
	}
	if !r.hasSettings() {
		return fmt.Errorf("journald policy requires at least one setting")
	}
	switch r.Storage {
	case "", JournaldStorageAuto, JournaldStorageVolatile, JournaldStoragePersistent, JournaldStorageNone:
	default:
		return fmt.Errorf("journald storage mode %q is invalid", r.Storage)
	}
	if err := validateNonNegativeDuration("maxRetention", r.MaxRetention); err != nil {
		return err
	}
	if err := validateNonNegativeDuration("rateLimitInterval", r.RateLimitInterval); err != nil {
		return err
	}
	for name, value := range map[string]*int64{
		"systemMaxUseBytes":  r.SystemMaxUseBytes,
		"runtimeMaxUseBytes": r.RuntimeMaxUseBytes,
	} {
		if value != nil && *value < 0 {
			return fmt.Errorf("journald %s must be non-negative", name)
		}
	}
	if r.RateLimitBurst != nil && *r.RateLimitBurst < 0 {
		return fmt.Errorf("journald rateLimitBurst must be non-negative")
	}
	return nil
}

func (r JournaldResource) hasSettings() bool {
	return r.Storage != "" || r.MaxRetention != "" || r.SystemMaxUseBytes != nil || r.RuntimeMaxUseBytes != nil ||
		r.RateLimitInterval != "" || r.RateLimitBurst != nil || r.ForwardToSyslog != nil ||
		r.ForwardToKernelBuffer != nil || r.ForwardToConsole != nil || r.ForwardToWall != nil
}

func validateNonNegativeDuration(field, value string) error {
	if value == "" {
		return nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return fmt.Errorf("journald %s must be a non-negative duration", field)
	}
	return nil
}
