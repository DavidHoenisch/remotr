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
		for i := range state.Configurations {
			for j := range state.Configurations[i].Packages {
				state.Configurations[i].Packages[j].NormalizeLifecycle()
			}
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

type canonicalAPTSigningKey struct {
	Kind          ResourceKind `yaml:"kind"`
	APTSigningKey `yaml:",inline"`
}

type canonicalAPTRepository struct {
	Kind          ResourceKind `yaml:"kind"`
	APTRepository `yaml:",inline"`
}

type canonicalSysctl struct {
	Kind           ResourceKind `yaml:"kind"`
	SysctlResource `yaml:",inline"`
}

type canonicalFile struct {
	Kind ResourceKind `yaml:"kind"`
	File `yaml:",inline"`
}

type canonicalDirectory struct {
	Kind              ResourceKind `yaml:"kind"`
	DirectoryResource `yaml:",inline"`
}

type canonicalLink struct {
	Kind         ResourceKind `yaml:"kind"`
	LinkResource `yaml:",inline"`
}

type canonicalGroup struct {
	Kind          ResourceKind `yaml:"kind"`
	GroupResource `yaml:",inline"`
}

type canonicalAuthorizedKey struct {
	Kind                  ResourceKind `yaml:"kind"`
	AuthorizedKeyResource `yaml:",inline"`
}

type canonicalKnownHost struct {
	Kind              ResourceKind `yaml:"kind"`
	KnownHostResource `yaml:",inline"`
}

type canonicalSudo struct {
	Kind         ResourceKind `yaml:"kind"`
	SudoResource `yaml:",inline"`
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
			if resource.Lifecycle == "" {
				err = fmt.Errorf("package lifecycle is required in schema 1")
			} else if resource.Lifecycle == LifecycleDisabled {
				err = fmt.Errorf("package lifecycle %q is unsupported", resource.Lifecycle)
			} else if resource.Lifecycle == LifecyclePurged && resource.PM != types.Apt {
				err = fmt.Errorf("package lifecycle %q is unsupported by provider %q", resource.Lifecycle, resource.PM)
			}
		}
		if err == nil {
			resource.Package.NormalizeLifecycle()
			cfg.Packages = append(cfg.Packages, resource.Package)
		}
	case ResourceKindAPTSigningKey:
		var resource canonicalAPTSigningKey
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.APTSigningKey.Validate()
		}
		if err == nil {
			if resource.Lifecycle == "" {
				resource.Lifecycle = LifecyclePresent
			}
			cfg.APTSigningKeys = append(cfg.APTSigningKeys, resource.APTSigningKey)
		}
	case ResourceKindAPTRepository:
		var resource canonicalAPTRepository
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.APTRepository.Validate()
		}
		if err == nil {
			if resource.Lifecycle == "" {
				resource.Lifecycle = LifecyclePresent
			}
			cfg.APTRepositories = append(cfg.APTRepositories, resource.APTRepository)
		}
	case ResourceKindSysctl:
		var resource canonicalSysctl
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.SysctlResource.Validate()
		}
		if err == nil {
			if resource.Activation == "" {
				resource.Activation = SysctlSingleKey
			}
			cfg.Sysctls = append(cfg.Sysctls, resource.SysctlResource)
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
	case ResourceKindDirectory:
		var resource canonicalDirectory
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.DirectoryResource.Validate()
		}
		if err == nil {
			cfg.Directories = append(cfg.Directories, resource.DirectoryResource)
		}
	case ResourceKindLink:
		var resource canonicalLink
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.LinkResource.Validate()
		}
		if err == nil {
			cfg.Links = append(cfg.Links, resource.LinkResource)
		}
	case ResourceKindGroup:
		var resource canonicalGroup
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.GroupResource.Validate()
		}
		if err == nil {
			cfg.Groups = append(cfg.Groups, resource.GroupResource)
		}
	case ResourceKindAuthorizedKey:
		var resource canonicalAuthorizedKey
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.AuthorizedKeyResource.Validate()
		}
		if err == nil {
			cfg.AuthorizedKeys = append(cfg.AuthorizedKeys, resource.AuthorizedKeyResource)
		}
	case ResourceKindKnownHost:
		var resource canonicalKnownHost
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.KnownHostResource.Validate()
		}
		if err == nil {
			cfg.KnownHosts = append(cfg.KnownHosts, resource.KnownHostResource)
		}
	case ResourceKindSudo:
		var resource canonicalSudo
		err = decode(&resource)
		if err == nil {
			resource.ResourceMeta.Kind = head.Kind
			err = resource.ResourceMeta.ValidateCanonical()
		}
		if err == nil {
			err = resource.SudoResource.Validate()
		}
		if err == nil {
			cfg.Sudo = append(cfg.Sudo, resource.SudoResource)
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
			err = resource.DownloadResource.Validate()
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
			err = resource.UserResource.Validate()
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
