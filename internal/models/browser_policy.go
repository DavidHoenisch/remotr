package models

import (
	"fmt"
	"regexp"
	"strings"
)

type BrowserPolicyBrowser string

const (
	BrowserChromium     BrowserPolicyBrowser = "chromium"
	BrowserGoogleChrome BrowserPolicyBrowser = "google-chrome"
	BrowserFirefox      BrowserPolicyBrowser = "firefox"
)

type BrowserPolicyScope string

const BrowserPolicyScopeSystem BrowserPolicyScope = "system"

type BrowserPolicyLevel string

const (
	BrowserPolicyLevelMandatory   BrowserPolicyLevel = "mandatory"
	BrowserPolicyLevelRecommended BrowserPolicyLevel = "recommended"
)

type BrowserPolicyValueType string

const (
	BrowserValueBoolean    BrowserPolicyValueType = "boolean"
	BrowserValueString     BrowserPolicyValueType = "string"
	BrowserValueInteger    BrowserPolicyValueType = "integer"
	BrowserValueDouble     BrowserPolicyValueType = "double"
	BrowserValueStringList BrowserPolicyValueType = "string-list"
	BrowserValueObject     BrowserPolicyValueType = "object"
)

type BrowserPolicyValue struct {
	Type  BrowserPolicyValueType `yaml:"type"`
	Value any                    `yaml:"value"`
}

func (v BrowserPolicyValue) Validate() error {
	_, err := v.JSONValue()
	return err
}

// JSONValue returns a JSON-compatible value with the declared native type.
func (v BrowserPolicyValue) JSONValue() (any, error) {
	switch v.Type {
	case BrowserValueBoolean:
		value, ok := v.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("browser boolean value must be a YAML boolean")
		}
		return value, nil
	case BrowserValueString:
		value, ok := v.Value.(string)
		if !ok {
			return nil, fmt.Errorf("browser string value must be a YAML string")
		}
		return value, nil
	case BrowserValueInteger:
		value, ok := browserInteger(v.Value)
		if !ok {
			return nil, fmt.Errorf("browser integer value must be a YAML integer")
		}
		return value, nil
	case BrowserValueDouble:
		value, ok := v.Value.(float64)
		if !ok {
			return nil, fmt.Errorf("browser double value must be a YAML floating-point number")
		}
		return value, nil
	case BrowserValueStringList:
		switch values := v.Value.(type) {
		case []string:
			return append([]string(nil), values...), nil
		case []any:
			out := make([]string, len(values))
			for i, value := range values {
				text, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("browser string-list entries must be strings")
				}
				out[i] = text
			}
			return out, nil
		default:
			return nil, fmt.Errorf("browser string-list value must be a YAML sequence")
		}
	case BrowserValueObject:
		value, ok := v.Value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("browser object value must be a YAML mapping")
		}
		if err := validateBrowserJSON(value, 0); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("browser value type %q is invalid", v.Type)
	}
}

func browserInteger(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	default:
		return 0, false
	}
}

func validateBrowserJSON(value any, depth int) error {
	if depth > 16 {
		return fmt.Errorf("browser object value exceeds maximum nesting depth")
	}
	switch value := value.(type) {
	case nil, bool, string, int, int32, int64, float64:
		return nil
	case []any:
		for _, item := range value {
			if err := validateBrowserJSON(item, depth+1); err != nil {
				return err
			}
		}
		return nil
	case []string:
		return nil
	case map[string]any:
		for key, item := range value {
			if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\x00\r\n") {
				return fmt.Errorf("browser object key is invalid")
			}
			if err := validateBrowserJSON(item, depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("browser object contains unsupported value type %T", value)
	}
}

var (
	browserResourceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)
	browserPolicyName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,126}$`)
)

// BrowserPolicyResource manages one typed key in a supported browser's
// operating-system managed-policy location.
type BrowserPolicyResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string               `yaml:"name"`
	Browser      BrowserPolicyBrowser `yaml:"browser"`
	PolicyName   string               `yaml:"policyName"`
	Scope        BrowserPolicyScope   `yaml:"scope"`
	Level        BrowserPolicyLevel   `yaml:"level"`
	Value        *BrowserPolicyValue  `yaml:"value,omitempty"`
	TrustAnchors []string             `yaml:"trustAnchors,omitempty"`
}

func (r BrowserPolicyResource) Validate() error {
	if !browserResourceName.MatchString(r.Name) {
		return fmt.Errorf("browser policy name must be a safe identifier")
	}
	switch r.Browser {
	case BrowserChromium, BrowserGoogleChrome, BrowserFirefox:
	default:
		return fmt.Errorf("browser policy browser %q is invalid", r.Browser)
	}
	if !browserPolicyName.MatchString(r.PolicyName) {
		return fmt.Errorf("browser managed policy key %q is invalid", r.PolicyName)
	}
	if r.Scope != BrowserPolicyScopeSystem {
		return fmt.Errorf("browser policy scope %q is unsupported; use system", r.Scope)
	}
	if r.Level != BrowserPolicyLevelMandatory && r.Level != BrowserPolicyLevelRecommended {
		return fmt.Errorf("browser policy level %q is invalid", r.Level)
	}
	if err := ValidateTrustAnchorReferences(r.TrustAnchors); err != nil {
		return fmt.Errorf("browser policy trustAnchors: %w", err)
	}
	lifecycle := r.Lifecycle
	if lifecycle == "" {
		lifecycle = LifecyclePresent
	}
	if lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("browser policy lifecycle must be present or absent")
	}
	if lifecycle == LifecycleAbsent {
		if r.Value != nil {
			return fmt.Errorf("absent browser policy must omit value")
		}
		return nil
	}
	if r.Value == nil {
		return fmt.Errorf("present browser policy requires a typed value")
	}
	return r.Value.Validate()
}
