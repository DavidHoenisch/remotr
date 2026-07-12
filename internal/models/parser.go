package models

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

// ParseState reads YAML deployable artifact bytes into State.
func ParseState(r io.Reader) (State, error) {
	state, _, err := ParseStateWithDiagnostics(r)
	state.Diagnostics = nil
	return state, err
}

// ParseStateWithDiagnostics reads an artifact and returns non-fatal migration notices.
func ParseStateWithDiagnostics(r io.Reader) (State, []Diagnostic, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return State{}, nil, err
	}
	var version struct {
		SchemaVersion *int `yaml:"schemaVersion"`
	}
	if err := yaml.Unmarshal(raw, &version); err != nil {
		return State{}, nil, err
	}
	if version.SchemaVersion == nil {
		// Legacy unversioned artifacts intentionally remain permissive until
		// their compatibility decoder is retired under the schema-0 policy.
		var state State
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		if err := dec.Decode(&state); err != nil {
			return State{}, nil, err
		}
		diagnostics := []Diagnostic{{
			Code:    DiagnosticLegacySchema,
			Message: "desired-state schema 0 is deprecated; render or author schemaVersion: 1 canonical resources",
		}}
		state.Diagnostics = append([]Diagnostic(nil), diagnostics...)
		return state, diagnostics, nil
	}
	if *version.SchemaVersion != 1 {
		return State{}, nil, fmt.Errorf("unsupported desired-state schemaVersion %d (supported: 1; omit for legacy schema 0)", *version.SchemaVersion)
	}
	state, err := parseCanonicalState(raw)
	return state, nil, err
}

type canonicalState struct {
	SchemaVersion  int                      `yaml:"schemaVersion"`
	Kind           types.Kind               `yaml:"kind,omitempty"`
	Configurations []canonicalConfiguration `yaml:"configurations"`
}

type canonicalConfiguration struct {
	Name          string               `yaml:"name"`
	Description   string               `yaml:"description,omitempty"`
	LastUpdated   time.Time            `yaml:"lastUpdated,omitempty"`
	TargetDistros []types.Distro       `yaml:"targetDistros,omitempty"`
	TargetArch    []types.Architecture `yaml:"targetArch,omitempty"`
	Resources     []yaml.Node          `yaml:"resources,omitempty"`
}

type canonicalPackage struct {
	Kind    ResourceKind `yaml:"kind"`
	Package `yaml:",inline"`
}

type canonicalFile struct {
	Kind ResourceKind `yaml:"kind"`
	File `yaml:",inline"`
}

type canonicalUserFile struct {
	Kind             ResourceKind `yaml:"kind"`
	UserFileResource `yaml:",inline"`
}

type canonicalDownload struct {
	Kind             ResourceKind `yaml:"kind"`
	DownloadResource `yaml:",inline"`
}

type canonicalUser struct {
	Kind         ResourceKind `yaml:"kind"`
	UserResource `yaml:",inline"`
}

type canonicalSystemd struct {
	Kind            ResourceKind `yaml:"kind"`
	SystemdResource `yaml:",inline"`
}

type canonicalSystemdUser struct {
	Kind                ResourceKind `yaml:"kind"`
	SystemdUserResource `yaml:",inline"`
}

type canonicalBootstrap struct {
	Kind              ResourceKind `yaml:"kind"`
	BootstrapResource `yaml:",inline"`
}

type canonicalAgentInstall struct {
	Kind                 ResourceKind `yaml:"kind"`
	AgentInstallResource `yaml:",inline"`
}

type canonicalFirewall struct {
	Kind             ResourceKind `yaml:"kind"`
	FirewallResource `yaml:",inline"`
}

type canonicalCommand struct {
	Kind            ResourceKind `yaml:"kind"`
	CommandResource `yaml:",inline"`
}

func parseCanonicalState(raw []byte) (State, error) {
	var document canonicalState
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&document); err != nil {
		return State{}, fmt.Errorf("decode schema 1 artifact: %w", err)
	}
	state := State{SchemaVersion: 1, Kind: document.Kind}
	for _, input := range document.Configurations {
		cfg := Configuration{
			Name: input.Name, Description: input.Description, LastUpdated: input.LastUpdated,
			TargetDistros: input.TargetDistros, TargetArch: input.TargetArch,
		}
		for i := range input.Resources {
			if err := decodeCanonicalResource(input.Name, &input.Resources[i], &cfg); err != nil {
				return State{}, err
			}
		}
		state.Configurations = append(state.Configurations, cfg)
	}
	return state, nil
}

func decodeCanonicalResource(configName string, node *yaml.Node, cfg *Configuration) error {
	var head ResourceHeader
	if err := node.Decode(&head); err != nil {
		return fmt.Errorf("configuration %q resource %q: %w", configName, ResourceAddress(configName, "<unknown>"), err)
	}
	address := ResourceAddress(configName, head.Name)
	if head.Name == "" {
		address = ResourceAddress(configName, "<unnamed>")
	}

	decode := func(dst any) error {
		raw, err := yaml.Marshal(node)
		if err != nil {
			return err
		}
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		return dec.Decode(dst)
	}

	var err error
	switch head.Kind {
	case ResourceKindPackage:
		var resource canonicalPackage
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Packages = append(cfg.Packages, resource.Package)
		}
	case ResourceKindFile:
		var resource canonicalFile
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Files = append(cfg.Files, resource.File)
		}
	case ResourceKindUserFile:
		var resource canonicalUserFile
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.UserFiles = append(cfg.UserFiles, resource.UserFileResource)
		}
	case ResourceKindDownload:
		var resource canonicalDownload
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Downloads = append(cfg.Downloads, resource.DownloadResource)
		}
	case ResourceKindUser:
		var resource canonicalUser
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Users = append(cfg.Users, resource.UserResource)
		}
	case ResourceKindSystemd:
		var resource canonicalSystemd
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Systemd = append(cfg.Systemd, resource.SystemdResource)
		}
	case ResourceKindSystemdUser:
		var resource canonicalSystemdUser
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.SystemdUser = append(cfg.SystemdUser, resource.SystemdUserResource)
		}
	case ResourceKindBootstrap:
		var resource canonicalBootstrap
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Bootstrap = append(cfg.Bootstrap, resource.BootstrapResource)
		}
	case ResourceKindAgentInstall:
		var resource canonicalAgentInstall
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.AgentInstall = append(cfg.AgentInstall, resource.AgentInstallResource)
		}
	case ResourceKindFirewall:
		var resource canonicalFirewall
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Firewall = append(cfg.Firewall, resource.FirewallResource)
		}
	case ResourceKindCommand:
		var resource canonicalCommand
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			cfg.Commands = append(cfg.Commands, resource.CommandResource)
		}
	default:
		return fmt.Errorf("configuration %q resource %q: unknown resource kind %q", configName, address, head.Kind)
	}
	if err != nil {
		return fmt.Errorf("configuration %q resource %q: %w", configName, address, err)
	}
	return nil
}
