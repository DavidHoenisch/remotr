package models

import (
	"github.com/DavidHoenisch/remotr/internal/types"
)

// CronJob is one scheduled job inside a crons.yaml artifact.
type CronJob struct {
	Name           string                  `yaml:"name,omitempty"`
	Description    string                  `yaml:"description,omitempty"`
	Use            string                  `yaml:"use,omitempty"`
	Schedule       string                  `yaml:"schedule,omitempty"`
	Timezone       string                  `yaml:"timezone,omitempty"`
	TargetDistros  []types.Distro          `yaml:"targetDistros,omitempty"`
	TargetArch     []types.Architecture    `yaml:"targetArch,omitempty"`
	Packages       []Package               `yaml:"packages,omitempty"`
	Files          []File                  `yaml:"files,omitempty"`
	Directories    []DirectoryResource     `yaml:"directories,omitempty"`
	Links          []LinkResource          `yaml:"links,omitempty"`
	Groups         []GroupResource         `yaml:"groups,omitempty"`
	AuthorizedKeys []AuthorizedKeyResource `yaml:"authorizedKeys,omitempty"`
	UserFiles      []UserFileResource      `yaml:"userFiles,omitempty"`
	Downloads      []DownloadResource      `yaml:"downloads,omitempty"`
	Users          []UserResource          `yaml:"users,omitempty"`
	Systemd        []SystemdResource       `yaml:"systemd,omitempty"`
	SystemdUser    []SystemdUserResource   `yaml:"systemdUser,omitempty"`
	Bootstrap      []BootstrapResource     `yaml:"bootstrap,omitempty"`
	AgentInstall   []AgentInstallResource  `yaml:"agentInstall,omitempty"`
	Commands       []CommandResource       `yaml:"commands,omitempty"`
}

// CronState is the top-level crons.yaml document.
type CronState struct {
	Crons []CronJob `yaml:"crons"`
}

// ToConfiguration projects cron resources into a Configuration for shared validation and apply.
func (c CronJob) ToConfiguration() Configuration {
	return Configuration{
		Name:           c.Name,
		Description:    c.Description,
		TargetDistros:  c.TargetDistros,
		TargetArch:     c.TargetArch,
		Packages:       c.Packages,
		Files:          c.Files,
		Directories:    c.Directories,
		Links:          c.Links,
		Groups:         c.Groups,
		AuthorizedKeys: c.AuthorizedKeys,
		UserFiles:      c.UserFiles,
		Downloads:      c.Downloads,
		Users:          c.Users,
		Systemd:        c.Systemd,
		SystemdUser:    c.SystemdUser,
		Bootstrap:      c.Bootstrap,
		AgentInstall:   c.AgentInstall,
		Commands:       c.Commands,
	}
}

// HasResources reports whether the job defines any executable resources.
func (c CronJob) HasResources() bool {
	cfg := c.ToConfiguration()
	return len(cfg.Packages) > 0 ||
		len(cfg.Files) > 0 ||
		len(cfg.Directories) > 0 ||
		len(cfg.Links) > 0 ||
		len(cfg.Groups) > 0 ||
		len(cfg.AuthorizedKeys) > 0 ||
		len(cfg.UserFiles) > 0 ||
		len(cfg.Downloads) > 0 ||
		len(cfg.Users) > 0 ||
		len(cfg.Systemd) > 0 ||
		len(cfg.SystemdUser) > 0 ||
		len(cfg.Bootstrap) > 0 ||
		len(cfg.AgentInstall) > 0 ||
		len(cfg.Commands) > 0
}
