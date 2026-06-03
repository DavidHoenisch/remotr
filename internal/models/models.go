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
	ReplaceRegx    string `yaml:"replaceRegx,omitempty"`
	Content        string `yaml:"content,omitempty"`
	Mode           []int  `yaml:"mode,omitempty"`
}

// DownloadResource fetches a remote file to a fixed destination path.
type DownloadResource struct {
	ResourceMeta   `yaml:",inline"`
	Name           string `yaml:"name"`
	URL            string `yaml:"url"`
	Dest           string `yaml:"dest"`
	Mode           []int  `yaml:"mode,omitempty"`
	Checksum       string `yaml:"checksum,omitempty"`
	NotifySystemd  string `yaml:"notifySystemd,omitempty"`
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

// SystemdUserResource declares per-user systemd --user unit state.
type SystemdUserResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Unit         string `yaml:"unit"`
	Users        string `yaml:"users"`
	Linger       bool   `yaml:"linger,omitempty"`
	Enabled      *bool  `yaml:"enabled,omitempty"`
	Active       *bool  `yaml:"active,omitempty"`
	UnitPath     string `yaml:"unitPath,omitempty"`
}

// CommandResource is an escape hatch with explicit check/apply/revert argv.
type CommandResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string   `yaml:"name"`
	Check        []string `yaml:"check,omitempty"`
	Apply        []string `yaml:"apply,omitempty"`
	Revert       []string `yaml:"revert,omitempty"`
}

// BootstrapWhen triggers one-shot orchestration when a path condition holds.
type BootstrapWhen struct {
	PathMissing string `yaml:"pathMissing,omitempty"`
	PathExists  string `yaml:"pathExists,omitempty"`
}

// BootstrapSystemdStep runs systemctl actions like the systemd applicator.
type BootstrapSystemdStep struct {
	Unit    string `yaml:"unit"`
	Enabled *bool  `yaml:"enabled,omitempty"`
	Active  *bool  `yaml:"active,omitempty"`
}

// BootstrapStep is exactly one of systemd or exec.
type BootstrapStep struct {
	Systemd *BootstrapSystemdStep `yaml:"systemd,omitempty"`
	Exec    []string              `yaml:"exec,omitempty"`
}

// BootstrapResource runs ordered steps once while When is unmet (e.g. DB file missing).
type BootstrapResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string          `yaml:"name"`
	When         BootstrapWhen   `yaml:"when"`
	Steps        []BootstrapStep `yaml:"steps"`
}

// AgentRunningCheck detects an installed agent process.
type AgentRunningCheck struct {
	Process string `yaml:"process,omitempty"`
}

// AgentInstallResource installs and enrolls a fleet agent from a tarball (e.g. Elastic Agent).
type AgentInstallResource struct {
	ResourceMeta          `yaml:",inline"`
	Name                  string            `yaml:"name"`
	Present               *bool             `yaml:"present,omitempty"`
	Version               string            `yaml:"version"`
	ArtifactURL           string            `yaml:"artifactURL"`
	ExtractDir            string            `yaml:"extractDir"`
	InstallBinary         string            `yaml:"installBinary,omitempty"`
	FleetURL              string            `yaml:"fleetURL"`
	EnrollmentTokenSecret string            `yaml:"enrollmentTokenSecret"`
	RunningCheck          AgentRunningCheck `yaml:"runningCheck"`
}

type Configuration struct {
	Name          string               `yaml:"name"`
	Description   string               `yaml:"description,omitempty"`
	LastUpdated   time.Time            `yaml:"lastUpdated,omitempty"`
	TargetDistros []types.Distro       `yaml:"targetDistros,omitempty"`
	TargetArch    []types.Architecture `yaml:"targetArch,omitempty"`
	Packages      []Package            `yaml:"packages,omitempty"`
	Files         []File               `yaml:"files,omitempty"`
	Downloads     []DownloadResource   `yaml:"downloads,omitempty"`
	Users         []UserResource       `yaml:"users,omitempty"`
	Systemd       []SystemdResource     `yaml:"systemd,omitempty"`
	SystemdUser   []SystemdUserResource `yaml:"systemdUser,omitempty"`
	Bootstrap     []BootstrapResource    `yaml:"bootstrap,omitempty"`
	AgentInstall  []AgentInstallResource `yaml:"agentInstall,omitempty"`
	Commands      []CommandResource      `yaml:"commands,omitempty"`
}

type State struct {
	Configurations []Configuration `yaml:"configurations"`
}

// ResourceAddress returns configuration-name/resource-name.
func ResourceAddress(configName, resourceName string) string {
	return configName + "/" + resourceName
}
