package models

import (
	"time"

	"github.com/DavidHoenisch/remotr/internal/types"
)

// ResourceMeta holds dependency and validation metadata shared by resources.
type ResourceMeta struct {
	DependsOn          []string `yaml:"dependsOn,omitempty"`
	PreApplyValidation []string `yaml:"preApplyValidation,omitempty"`
}

type Package struct {
	ResourceMeta     `yaml:",inline"`
	Name             string               `yaml:"name"`
	Present          bool                 `yaml:"present"`
	Arch             types.Architecture   `yaml:"arch,omitempty"`
	PM               types.PackageManager `yaml:"packageManager,omitempty"`
}

type File struct {
	ResourceMeta   `yaml:",inline"`
	Name           string `yaml:"name"`
	Path           string `yaml:"path"`
	UpdateExisting bool   `yaml:"updateExisting,omitempty"`
	WithRegx       string `yaml:"withRegx,omitempty"`
	Content        string `yaml:"content,omitempty"`
	Mode           []int  `yaml:"mode,omitempty"`
}

// UserResource declares a local user account.
type UserResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Username     string `yaml:"username"`
	Present      bool   `yaml:"present"`
	UID          int    `yaml:"uid,omitempty"`
}

// SystemdResource declares systemd unit state.
type SystemdResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Unit         string `yaml:"unit"`
	Enabled      *bool  `yaml:"enabled,omitempty"`
	Active       *bool  `yaml:"active,omitempty"`
	Masked       *bool  `yaml:"masked,omitempty"`
}

// CommandResource is an escape hatch with explicit check/apply/revert argv.
type CommandResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string   `yaml:"name"`
	Check        []string `yaml:"check,omitempty"`
	Apply        []string `yaml:"apply,omitempty"`
	Revert       []string `yaml:"revert,omitempty"`
}

type Configuration struct {
	Name          string               `yaml:"name"`
	Description   string               `yaml:"description,omitempty"`
	LastUpdated   time.Time            `yaml:"lastUpdated,omitempty"`
	TargetDistros []types.Distro       `yaml:"targetDistros,omitempty"`
	TargetArch    []types.Architecture `yaml:"targetArch,omitempty"`
	Packages      []Package            `yaml:"packages,omitempty"`
	Files         []File               `yaml:"files,omitempty"`
	Users         []UserResource       `yaml:"users,omitempty"`
	Systemd       []SystemdResource    `yaml:"systemd,omitempty"`
	Commands      []CommandResource    `yaml:"commands,omitempty"`
}

type State struct {
	Configurations []Configuration `yaml:"configurations"`
}

// ResourceAddress returns configuration-name/resource-name.
func ResourceAddress(configName, resourceName string) string {
	return configName + "/" + resourceName
}
