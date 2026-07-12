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
	raw, err := io.ReadAll(r)
	if err != nil {
		return State{}, err
	}
	var version struct {
		SchemaVersion *int `yaml:"schemaVersion"`
	}
	if err := yaml.Unmarshal(raw, &version); err != nil {
		return State{}, err
	}
	if version.SchemaVersion == nil {
		// Legacy unversioned artifacts intentionally remain permissive until
		// their compatibility decoder is retired under the schema-0 policy.
		var state State
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		if err := dec.Decode(&state); err != nil {
			return State{}, err
		}
		return state, nil
	}
	if *version.SchemaVersion != 1 {
		return State{}, fmt.Errorf("unsupported desired-state schemaVersion %d (supported: 1; omit for legacy schema 0)", *version.SchemaVersion)
	}
	return parseCanonicalState(raw)
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
	Kind    string `yaml:"kind"`
	Package `yaml:",inline"`
}

type canonicalFile struct {
	Kind string `yaml:"kind"`
	File `yaml:",inline"`
}

type canonicalUserFile struct {
	Kind             string `yaml:"kind"`
	UserFileResource `yaml:",inline"`
}

type canonicalDownload struct {
	Kind             string `yaml:"kind"`
	DownloadResource `yaml:",inline"`
}

type canonicalUser struct {
	Kind         string `yaml:"kind"`
	UserResource `yaml:",inline"`
}

type canonicalSystemd struct {
	Kind            string `yaml:"kind"`
	SystemdResource `yaml:",inline"`
}

type canonicalSystemdUser struct {
	Kind                string `yaml:"kind"`
	SystemdUserResource `yaml:",inline"`
}

type canonicalBootstrap struct {
	Kind              string `yaml:"kind"`
	BootstrapResource `yaml:",inline"`
}

type canonicalAgentInstall struct {
	Kind                 string `yaml:"kind"`
	AgentInstallResource `yaml:",inline"`
}

type canonicalFirewall struct {
	Kind             string `yaml:"kind"`
	FirewallResource `yaml:",inline"`
}

type canonicalCommand struct {
	Kind            string `yaml:"kind"`
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
	var head struct {
		Kind string `yaml:"kind"`
		Name string `yaml:"name"`
	}
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
	case "package":
		var resource canonicalPackage
		err = decode(&resource)
		cfg.Packages = append(cfg.Packages, resource.Package)
	case "file":
		var resource canonicalFile
		err = decode(&resource)
		cfg.Files = append(cfg.Files, resource.File)
	case "userFile":
		var resource canonicalUserFile
		err = decode(&resource)
		cfg.UserFiles = append(cfg.UserFiles, resource.UserFileResource)
	case "download":
		var resource canonicalDownload
		err = decode(&resource)
		cfg.Downloads = append(cfg.Downloads, resource.DownloadResource)
	case "user":
		var resource canonicalUser
		err = decode(&resource)
		cfg.Users = append(cfg.Users, resource.UserResource)
	case "systemd":
		var resource canonicalSystemd
		err = decode(&resource)
		cfg.Systemd = append(cfg.Systemd, resource.SystemdResource)
	case "systemdUser":
		var resource canonicalSystemdUser
		err = decode(&resource)
		cfg.SystemdUser = append(cfg.SystemdUser, resource.SystemdUserResource)
	case "bootstrap":
		var resource canonicalBootstrap
		err = decode(&resource)
		cfg.Bootstrap = append(cfg.Bootstrap, resource.BootstrapResource)
	case "agentInstall":
		var resource canonicalAgentInstall
		err = decode(&resource)
		cfg.AgentInstall = append(cfg.AgentInstall, resource.AgentInstallResource)
	case "firewall":
		var resource canonicalFirewall
		err = decode(&resource)
		cfg.Firewall = append(cfg.Firewall, resource.FirewallResource)
	case "command":
		var resource canonicalCommand
		err = decode(&resource)
		cfg.Commands = append(cfg.Commands, resource.CommandResource)
	default:
		return fmt.Errorf("configuration %q resource %q: unknown resource kind %q", configName, address, head.Kind)
	}
	if err != nil {
		return fmt.Errorf("configuration %q resource %q: %w", configName, address, err)
	}
	return nil
}
