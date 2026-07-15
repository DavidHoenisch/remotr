package models

import (
	"fmt"
	"regexp"
	"strings"
)

type DesktopSettingProvider string

const (
	DesktopSettingProviderDconf     DesktopSettingProvider = "dconf"
	DesktopSettingProviderGSettings DesktopSettingProvider = "gsettings"
)

type DesktopSettingScope string

const (
	DesktopSettingScopeUser   DesktopSettingScope = "user"
	DesktopSettingScopeSystem DesktopSettingScope = "system"
)

type DesktopSettingLevel string

const (
	DesktopSettingLevelDefault   DesktopSettingLevel = "default"
	DesktopSettingLevelMandatory DesktopSettingLevel = "mandatory"
)

type DesktopValueType string

const (
	DesktopValueBoolean    DesktopValueType = "boolean"
	DesktopValueString     DesktopValueType = "string"
	DesktopValueInt32      DesktopValueType = "int32"
	DesktopValueInt64      DesktopValueType = "int64"
	DesktopValueUint32     DesktopValueType = "uint32"
	DesktopValueDouble     DesktopValueType = "double"
	DesktopValueStringList DesktopValueType = "string-list"
)

// DesktopSettingValue carries an explicit native type so YAML coercion cannot
// make a string look equivalent to a boolean or number.
type DesktopSettingValue struct {
	Type  DesktopValueType `yaml:"type"`
	Value any              `yaml:"value"`
}

func (v DesktopSettingValue) Validate() error {
	switch v.Type {
	case DesktopValueBoolean:
		if _, ok := v.Value.(bool); !ok {
			return fmt.Errorf("desktop boolean value must be a YAML boolean")
		}
	case DesktopValueString:
		if _, ok := v.Value.(string); !ok {
			return fmt.Errorf("desktop string value must be a YAML string")
		}
	case DesktopValueInt32:
		value, ok := desktopSigned(v.Value)
		if !ok || value < -(1<<31) || value > 1<<31-1 {
			return fmt.Errorf("desktop int32 value must be a 32-bit integer")
		}
	case DesktopValueInt64:
		if _, ok := desktopSigned(v.Value); !ok {
			return fmt.Errorf("desktop int64 value must be an integer")
		}
	case DesktopValueUint32:
		value, ok := desktopSigned(v.Value)
		if !ok || value < 0 || value > 1<<32-1 {
			return fmt.Errorf("desktop uint32 value must be an integer from 0 through 4294967295")
		}
	case DesktopValueDouble:
		if _, ok := v.Value.(float64); !ok {
			return fmt.Errorf("desktop double value must be a YAML floating-point number")
		}
	case DesktopValueStringList:
		switch values := v.Value.(type) {
		case []string:
		case []any:
			for _, value := range values {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("desktop string-list entries must be strings")
				}
			}
		default:
			return fmt.Errorf("desktop string-list value must be a YAML sequence")
		}
	default:
		return fmt.Errorf("desktop value type %q is invalid", v.Type)
	}
	return nil
}

func desktopSigned(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case int32:
		return int64(value), true
	default:
		return 0, false
	}
}

var (
	desktopSettingName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)
	dconfSettingPath   = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9_./-]*/[A-Za-z0-9][A-Za-z0-9_-]*$`)
	gsettingsSchema    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
	gsettingsKey       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
)

// DesktopSettingResource manages one typed dconf/GSettings key.
type DesktopSettingResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string                  `yaml:"name"`
	Provider     DesktopSettingProvider  `yaml:"provider"`
	Scope        DesktopSettingScope     `yaml:"scope"`
	Level        DesktopSettingLevel     `yaml:"level,omitempty"`
	Selector     InteractiveUserSelector `yaml:"selector"`
	Path         string                  `yaml:"path,omitempty"`
	Schema       string                  `yaml:"schema,omitempty"`
	Key          string                  `yaml:"key,omitempty"`
	Value        DesktopSettingValue     `yaml:"value"`
}

func (r DesktopSettingResource) Validate() error {
	if !desktopSettingName.MatchString(r.Name) {
		return fmt.Errorf("desktop setting name must be a safe identifier")
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent {
		return fmt.Errorf("desktop setting supports only present lifecycle")
	}
	if r.Level == "" {
		r.Level = DesktopSettingLevelDefault
	}
	if r.Level != DesktopSettingLevelDefault && r.Level != DesktopSettingLevelMandatory {
		return fmt.Errorf("desktop setting level %q is invalid", r.Level)
	}
	switch r.Scope {
	case DesktopSettingScopeUser:
		if r.Ownership != "" && r.Ownership != OwnershipMerge && r.Ownership != OwnershipAuthoritative {
			return fmt.Errorf("user desktop setting ownership must be merge or authoritative")
		}
		if err := r.Selector.Validate(); err != nil {
			return fmt.Errorf("desktop setting selector: %w", err)
		}
		if r.Level == DesktopSettingLevelMandatory {
			return fmt.Errorf("mandatory desktop settings require system dconf scope")
		}
	case DesktopSettingScopeSystem:
		if r.Ownership != "" {
			return fmt.Errorf("system desktop setting must not declare selector ownership")
		}
		if r.Provider != DesktopSettingProviderDconf {
			return fmt.Errorf("system desktop settings require dconf provider")
		}
		if r.Selector.Mode != InteractiveUserSelectionAll || len(r.Selector.Usernames) != 0 {
			return fmt.Errorf("system dconf scope requires all-interactive selector")
		}
	default:
		return fmt.Errorf("desktop setting scope %q is invalid", r.Scope)
	}
	switch r.Provider {
	case DesktopSettingProviderDconf:
		if !dconfSettingPath.MatchString(r.Path) || strings.Contains(r.Path, "//") || strings.Contains(r.Path, "..") {
			return fmt.Errorf("desktop dconf path %q is invalid", r.Path)
		}
		if r.Schema != "" || r.Key != "" {
			return fmt.Errorf("dconf desktop setting must not declare schema or key")
		}
	case DesktopSettingProviderGSettings:
		if r.Scope != DesktopSettingScopeUser {
			return fmt.Errorf("gsettings supports only user scope")
		}
		if !gsettingsSchema.MatchString(r.Schema) || !gsettingsKey.MatchString(r.Key) {
			return fmt.Errorf("gsettings schema and key are required and must be valid")
		}
		if r.Path != "" {
			return fmt.Errorf("gsettings desktop setting must not declare dconf path")
		}
	default:
		return fmt.Errorf("desktop setting provider %q is invalid", r.Provider)
	}
	return r.Value.Validate()
}

func (r DesktopSettingResource) EffectiveSelectorOwnership() OwnershipMode {
	if r.Ownership == "" {
		return OwnershipMerge
	}
	return r.Ownership
}
