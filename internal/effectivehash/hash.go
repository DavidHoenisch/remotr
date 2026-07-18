// Package effectivehash computes the versioned, canonical identity of an
// effective desired-state resource.
package effectivehash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// SchemaVersion identifies the canonical representation written by Canonical.
const SchemaVersion = 1

// Value is a closed set of canonical desired-state value types. Callers must
// choose whether a collection is an ordered List or an unordered Set.
type Value interface {
	canonicalValue()
}

type Object map[string]Value
type List []Value
type Set []Value
type String string
type Boolean bool
type Integer int64
type Unsigned uint64
type Float float64
type Null struct{}

func (Object) canonicalValue()   {}
func (List) canonicalValue()     {}
func (Set) canonicalValue()      {}
func (String) canonicalValue()   {}
func (Boolean) canonicalValue()  {}
func (Integer) canonicalValue()  {}
func (Unsigned) canonicalValue() {}
func (Float) canonicalValue()    {}
func (Null) canonicalValue()     {}

type ProviderIdentity struct {
	ID               string
	ContractRevision string
}

// SecretIdentity is the safe identity of resolved secret material. It
// intentionally has no field capable of carrying secret bytes.
type SecretIdentity struct {
	Path                 string
	Provider             string
	Name                 string
	Version              string
	ActivationGeneration uint64
	Purpose              string
}

type Input struct {
	ResourceAddress string
	ResourceKind    string
	Provider        ProviderIdentity
	Desired         Object
	// Defaults contains only schema-declared managed defaults. Fields absent
	// from both Desired and Defaults remain omitted and therefore unmanaged.
	Defaults Object
	Secrets  []SecretIdentity
}

// Canonical returns the schema-versioned canonical JSON representation.
func Canonical(input Input) ([]byte, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	desired, err := appendValue(nil, mergeDefaults(input.Defaults, input.Desired))
	if err != nil {
		return nil, err
	}
	secrets := append([]SecretIdentity(nil), input.Secrets...)
	sort.Slice(secrets, func(i, j int) bool { return secretKey(secrets[i]) < secretKey(secrets[j]) })

	output := []byte(`{"schemaVersion":1,"resource":{"address":`)
	output = appendQuoted(output, input.ResourceAddress)
	output = append(output, `,"kind":`...)
	output = appendQuoted(output, input.ResourceKind)
	output = append(output, `,"provider":{"id":`...)
	output = appendQuoted(output, input.Provider.ID)
	output = append(output, `,"contractRevision":`...)
	output = appendQuoted(output, input.Provider.ContractRevision)
	output = append(output, `},"desired":`...)
	output = append(output, desired...)
	output = append(output, `,"secrets":[`...)
	for index, secret := range secrets {
		if index > 0 {
			output = append(output, ',')
		}
		output = append(output, `{"path":`...)
		output = appendQuoted(output, secret.Path)
		output = append(output, `,"provider":`...)
		output = appendQuoted(output, secret.Provider)
		output = append(output, `,"name":`...)
		output = appendQuoted(output, secret.Name)
		output = append(output, `,"version":`...)
		output = appendQuoted(output, secret.Version)
		output = append(output, `,"activationGeneration":`...)
		output = strconv.AppendUint(output, secret.ActivationGeneration, 10)
		output = append(output, `,"purpose":`...)
		output = appendQuoted(output, secret.Purpose)
		output = append(output, '}')
	}
	return append(output, `]}}`...), nil
}

func mergeDefaults(defaults, desired Object) Object {
	effective := make(Object, len(defaults)+len(desired))
	for key, value := range defaults {
		effective[key] = value
	}
	for key, value := range desired {
		defaultObject, hasDefaultObject := effective[key].(Object)
		desiredObject, hasDesiredObject := value.(Object)
		if hasDefaultObject && hasDesiredObject {
			effective[key] = mergeDefaults(defaultObject, desiredObject)
			continue
		}
		effective[key] = value
	}
	return effective
}

// Sum returns the SHA-256 digest of Canonical with an explicit algorithm tag.
func Sum(input Input) (string, error) {
	canonical, err := Canonical(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func validateInput(input Input) error {
	for field, value := range map[string]string{
		"resource address":           input.ResourceAddress,
		"resource kind":              input.ResourceKind,
		"provider id":                input.Provider.ID,
		"provider contract revision": input.Provider.ContractRevision,
	} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("effective hash %s is required and must be trimmed", field)
		}
	}
	if input.Desired == nil {
		return fmt.Errorf("effective hash desired state is required")
	}
	for index, secret := range input.Secrets {
		for field, value := range map[string]string{
			"path": secret.Path, "provider": secret.Provider, "name": secret.Name,
			"version": secret.Version, "purpose": secret.Purpose,
		} {
			if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
				return fmt.Errorf("effective hash secret %d %s is required and must be trimmed", index, field)
			}
		}
	}
	return nil
}

func appendValue(output []byte, value Value) ([]byte, error) {
	switch value := value.(type) {
	case Object:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		output = append(output, '{')
		for index, key := range keys {
			if index > 0 {
				output = append(output, ',')
			}
			output = appendQuoted(output, key)
			output = append(output, ':')
			var err error
			output, err = appendValue(output, value[key])
			if err != nil {
				return nil, fmt.Errorf("canonical field %q: %w", key, err)
			}
		}
		return append(output, '}'), nil
	case List:
		output = append(output, '[')
		for index, item := range value {
			if index > 0 {
				output = append(output, ',')
			}
			var err error
			output, err = appendValue(output, item)
			if err != nil {
				return nil, fmt.Errorf("canonical list item %d: %w", index, err)
			}
		}
		return append(output, ']'), nil
	case Set:
		items := make([][]byte, len(value))
		for index, item := range value {
			canonical, err := appendValue(nil, item)
			if err != nil {
				return nil, fmt.Errorf("canonical set item %d: %w", index, err)
			}
			items[index] = canonical
		}
		sort.Slice(items, func(i, j int) bool { return string(items[i]) < string(items[j]) })
		output = append(output, `{"$set":[`...)
		for index, item := range items {
			if index > 0 {
				output = append(output, ',')
			}
			output = append(output, item...)
		}
		return append(output, `]}`...), nil
	case String:
		return appendQuoted(output, string(value)), nil
	case Boolean:
		return strconv.AppendBool(output, bool(value)), nil
	case Integer:
		return strconv.AppendInt(output, int64(value), 10), nil
	case Unsigned:
		return strconv.AppendUint(output, uint64(value), 10), nil
	case Float:
		floatValue := float64(value)
		if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return nil, fmt.Errorf("canonical float must be finite")
		}
		if floatValue == 0 {
			floatValue = 0
		}
		return strconv.AppendFloat(output, floatValue, 'g', -1, 64), nil
	case Null:
		return append(output, "null"...), nil
	case nil:
		return nil, fmt.Errorf("canonical value is nil")
	default:
		return nil, fmt.Errorf("unsupported canonical value %T", value)
	}
}

func appendQuoted(output []byte, value string) []byte {
	encoded, _ := json.Marshal(value)
	return append(output, encoded...)
}

func secretKey(secret SecretIdentity) string {
	return strings.Join([]string{
		secret.Path, secret.Provider, secret.Name, secret.Version,
		strconv.FormatUint(secret.ActivationGeneration, 10), secret.Purpose,
	}, "\x00")
}
