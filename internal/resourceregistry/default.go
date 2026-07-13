package resourceregistry

import (
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/agentinstall"
	"github.com/DavidHoenisch/remotr/internal/applicators/aptkeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/aptrepositories"
	"github.com/DavidHoenisch/remotr/internal/applicators/authorizedkeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/bootstrap"
	"github.com/DavidHoenisch/remotr/internal/applicators/command"
	"github.com/DavidHoenisch/remotr/internal/applicators/directories"
	"github.com/DavidHoenisch/remotr/internal/applicators/downloads"
	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/applicators/firewall"
	"github.com/DavidHoenisch/remotr/internal/applicators/groups"
	"github.com/DavidHoenisch/remotr/internal/applicators/hostlocale"
	"github.com/DavidHoenisch/remotr/internal/applicators/hostname"
	"github.com/DavidHoenisch/remotr/internal/applicators/kernelmodules"
	"github.com/DavidHoenisch/remotr/internal/applicators/knownhosts"
	"github.com/DavidHoenisch/remotr/internal/applicators/links"
	pkgfactory "github.com/DavidHoenisch/remotr/internal/applicators/packages"
	"github.com/DavidHoenisch/remotr/internal/applicators/sudo"
	sysctlapp "github.com/DavidHoenisch/remotr/internal/applicators/sysctl"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemd"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
	"github.com/DavidHoenisch/remotr/internal/applicators/timesync"
	"github.com/DavidHoenisch/remotr/internal/applicators/userfiles"
	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// NewDefault constructs the registry for every currently implemented resource kind.
func NewDefault() (*Registry, error) {
	return New(
		definition(models.ResourceKindPackage, SensitivityPublic, models.RiskNormal, 0, []string{"package-database"},
			func(v *models.Package) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.Package { return pointers(c.Packages) },
			func(c *models.Configuration, v models.Package) { c.Packages = append(c.Packages, v) },
			func(v *models.Package, c FactoryContext) (executor.Handler, error) {
				return pkgfactory.SelectPackageApplicator(c.Facts.Distro, *v, c.Facts, c.Runner, c.PackageURLs)
			}, nil, nil),
		definition(models.ResourceKindAPTSigningKey, SensitivityPublic, models.RiskNormal, 0, []string{"apt-keyrings"},
			func(v *models.APTSigningKey) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.APTSigningKey { return pointers(c.APTSigningKeys) },
			func(c *models.Configuration, v models.APTSigningKey) { c.APTSigningKeys = append(c.APTSigningKeys, v) },
			func(v *models.APTSigningKey, c FactoryContext) (executor.Handler, error) {
				return aptkeys.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindAPTRepository, SensitivitySensitiveMetadata, models.RiskNormal, 1, []string{"apt-repositories"},
			func(v *models.APTRepository) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.APTRepository { return pointers(c.APTRepositories) },
			func(c *models.Configuration, v models.APTRepository) {
				c.APTRepositories = append(c.APTRepositories, v)
			},
			func(v *models.APTRepository, c FactoryContext) (executor.Handler, error) {
				return aptrepositories.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindSysctl, SensitivityPublic, models.RiskNormal, 2, []string{"kernel-sysctl"},
			func(v *models.SysctlResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.SysctlResource { return pointers(c.Sysctls) },
			func(c *models.Configuration, v models.SysctlResource) { c.Sysctls = append(c.Sysctls, v) },
			func(v *models.SysctlResource, c FactoryContext) (executor.Handler, error) {
				return sysctlapp.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindKernelModule, SensitivityPublic, models.RiskBoot, 3, []string{"kernel-modules"},
			func(v *models.KernelModuleResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.KernelModuleResource { return pointers(c.KernelModules) },
			func(c *models.Configuration, v models.KernelModuleResource) {
				c.KernelModules = append(c.KernelModules, v)
			},
			func(v *models.KernelModuleResource, c FactoryContext) (executor.Handler, error) {
				return kernelmodules.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindHostname, SensitivityPublic, models.RiskNormal, 2, []string{"hostname"},
			func(v *models.HostnameResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.HostnameResource { return pointers(c.Hostnames) },
			func(c *models.Configuration, v models.HostnameResource) { c.Hostnames = append(c.Hostnames, v) },
			func(v *models.HostnameResource, c FactoryContext) (executor.Handler, error) {
				return hostname.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindHostLocale, SensitivityPublic, models.RiskNormal, 2, []string{"host-locale"},
			func(v *models.HostLocaleResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.HostLocaleResource { return pointers(c.HostLocales) },
			func(c *models.Configuration, v models.HostLocaleResource) { c.HostLocales = append(c.HostLocales, v) },
			func(v *models.HostLocaleResource, c FactoryContext) (executor.Handler, error) {
				return hostlocale.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindTimeSync, SensitivityPublic, models.RiskNormal, 2, []string{"time-sync"},
			func(v *models.TimeSyncResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.TimeSyncResource { return pointers(c.TimeSync) },
			func(c *models.Configuration, v models.TimeSyncResource) { c.TimeSync = append(c.TimeSync, v) },
			func(v *models.TimeSyncResource, c FactoryContext) (executor.Handler, error) {
				return timesync.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindFile, SensitivityPublic, models.RiskNormal, 1, nil,
			func(v *models.File) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.File { return pointers(c.Files) },
			func(c *models.Configuration, v models.File) { c.Files = append(c.Files, v) },
			func(v *models.File, _ FactoryContext) (executor.Handler, error) { return files.New(*v), nil },
			func(v *models.File) models.RiskClass {
				if isCriticalFile(v) {
					return models.RiskAccess
				}
				return models.RiskNormal
			},
			func(v *models.File) int {
				if isCriticalFile(v) {
					return 3
				}
				return 1
			}),
		definition(models.ResourceKindDirectory, SensitivityPublic, models.RiskNormal, 1, nil,
			func(v *models.DirectoryResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.DirectoryResource { return pointers(c.Directories) },
			func(c *models.Configuration, v models.DirectoryResource) { c.Directories = append(c.Directories, v) },
			func(v *models.DirectoryResource, _ FactoryContext) (executor.Handler, error) {
				return directories.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindLink, SensitivityPublic, models.RiskNormal, 1, nil,
			func(v *models.LinkResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.LinkResource { return pointers(c.Links) },
			func(c *models.Configuration, v models.LinkResource) { c.Links = append(c.Links, v) },
			func(v *models.LinkResource, _ FactoryContext) (executor.Handler, error) {
				return links.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindGroup, SensitivitySensitiveMetadata, models.RiskAccess, 4, []string{"account-database"},
			func(v *models.GroupResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.GroupResource { return pointers(c.Groups) },
			func(c *models.Configuration, v models.GroupResource) { c.Groups = append(c.Groups, v) },
			func(v *models.GroupResource, c FactoryContext) (executor.Handler, error) {
				return groups.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindAuthorizedKey, SensitivitySensitiveMetadata, models.RiskAccess, 5, []string{"ssh-access"},
			func(v *models.AuthorizedKeyResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.AuthorizedKeyResource { return pointers(c.AuthorizedKeys) },
			func(c *models.Configuration, v models.AuthorizedKeyResource) {
				c.AuthorizedKeys = append(c.AuthorizedKeys, v)
			},
			func(v *models.AuthorizedKeyResource, _ FactoryContext) (executor.Handler, error) {
				return authorizedkeys.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindKnownHost, SensitivitySensitiveMetadata, models.RiskNormal, 5, []string{"ssh-access"},
			func(v *models.KnownHostResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.KnownHostResource { return pointers(c.KnownHosts) },
			func(c *models.Configuration, v models.KnownHostResource) { c.KnownHosts = append(c.KnownHosts, v) },
			func(v *models.KnownHostResource, _ FactoryContext) (executor.Handler, error) {
				return knownhosts.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindSudo, SensitivitySensitiveMetadata, models.RiskAccess, 6, []string{"sudo-policy"},
			func(v *models.SudoResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.SudoResource { return pointers(c.Sudo) },
			func(c *models.Configuration, v models.SudoResource) { c.Sudo = append(c.Sudo, v) },
			func(v *models.SudoResource, c FactoryContext) (executor.Handler, error) {
				return sudo.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindDownload, SensitivityPublic, models.RiskNormal, 2, nil,
			func(v *models.DownloadResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.DownloadResource { return pointers(c.Downloads) },
			func(c *models.Configuration, v models.DownloadResource) { c.Downloads = append(c.Downloads, v) },
			func(v *models.DownloadResource, c FactoryContext) (executor.Handler, error) {
				return downloads.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindUser, SensitivitySensitiveMetadata, models.RiskAccess, 4, []string{"account-database"},
			func(v *models.UserResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.UserResource { return pointers(c.Users) },
			func(c *models.Configuration, v models.UserResource) { c.Users = append(c.Users, v) },
			func(v *models.UserResource, _ FactoryContext) (executor.Handler, error) { return users.New(*v), nil }, nil, nil),
		definition(models.ResourceKindUserFile, SensitivityPublic, models.RiskNormal, 5, nil,
			func(v *models.UserFileResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.UserFileResource { return pointers(c.UserFiles) },
			func(c *models.Configuration, v models.UserFileResource) { c.UserFiles = append(c.UserFiles, v) },
			func(v *models.UserFileResource, _ FactoryContext) (executor.Handler, error) {
				return userfiles.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindFirewall, SensitivityPublic, models.RiskConnectivity, 6, []string{"firewall"},
			func(v *models.FirewallResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.FirewallResource { return pointers(c.Firewall) },
			func(c *models.Configuration, v models.FirewallResource) { c.Firewall = append(c.Firewall, v) },
			func(v *models.FirewallResource, c FactoryContext) (executor.Handler, error) {
				provider := firewall.New(*v, c.Runner)
				provider.SyncURL = c.SyncURL
				return provider, nil
			}, nil, nil),
		definition(models.ResourceKindSystemd, SensitivityPublic, models.RiskNormal, 7, nil,
			func(v *models.SystemdResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.SystemdResource { return pointers(c.Systemd) },
			func(c *models.Configuration, v models.SystemdResource) { c.Systemd = append(c.Systemd, v) },
			func(v *models.SystemdResource, c FactoryContext) (executor.Handler, error) {
				return systemd.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindSystemdUser, SensitivityPublic, models.RiskNormal, 8, nil,
			func(v *models.SystemdUserResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.SystemdUserResource { return pointers(c.SystemdUser) },
			func(c *models.Configuration, v models.SystemdUserResource) { c.SystemdUser = append(c.SystemdUser, v) },
			func(v *models.SystemdUserResource, c FactoryContext) (executor.Handler, error) {
				return systemduser.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindBootstrap, SensitivityPublic, models.RiskBoot, 9, nil,
			func(v *models.BootstrapResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.BootstrapResource { return pointers(c.Bootstrap) },
			func(c *models.Configuration, v models.BootstrapResource) { c.Bootstrap = append(c.Bootstrap, v) },
			func(v *models.BootstrapResource, c FactoryContext) (executor.Handler, error) {
				return bootstrap.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindAgentInstall, SensitivitySensitiveMetadata, models.RiskSensitive, 10, nil,
			func(v *models.AgentInstallResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.AgentInstallResource { return pointers(c.AgentInstall) },
			func(c *models.Configuration, v models.AgentInstallResource) {
				c.AgentInstall = append(c.AgentInstall, v)
			},
			func(v *models.AgentInstallResource, c FactoryContext) (executor.Handler, error) {
				return agentinstall.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindCommand, SensitivityPublic, models.RiskDestructive, 11, nil,
			func(v *models.CommandResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.CommandResource { return pointers(c.Commands) },
			func(c *models.Configuration, v models.CommandResource) { c.Commands = append(c.Commands, v) },
			func(v *models.CommandResource, c FactoryContext) (executor.Handler, error) {
				return command.New(*v, c.Runner), nil
			}, nil, nil),
	)
}

func definition[T any](
	kind models.ResourceKind,
	sensitivity Sensitivity,
	baseRisk models.RiskClass,
	baseTier int,
	baseLocks []string,
	metadata func(*T) (string, *models.ResourceMeta),
	list func(*models.Configuration) []*T,
	appendValue func(*models.Configuration, T),
	factory func(*T, FactoryContext) (executor.Handler, error),
	risk func(*T) models.RiskClass,
	tier func(*T) int,
) Definition {
	if risk == nil {
		risk = func(*T) models.RiskClass { return baseRisk }
	}
	if tier == nil {
		tier = func(*T) int { return baseTier }
	}
	cast := func(value any) (*T, error) {
		typed, ok := value.(*T)
		if !ok || typed == nil {
			return nil, fmt.Errorf("resource kind %q received value %T", kind, value)
		}
		return typed, nil
	}
	return Definition{
		Kind:        kind,
		Decode:      strictDecodeResource[T],
		Sensitivity: sensitivity,
		Metadata: func(value any) (string, *models.ResourceMeta, error) {
			typed, err := cast(value)
			if err != nil {
				return "", nil, err
			}
			name, meta := metadata(typed)
			return name, meta, nil
		},
		Validate: func(value any) error {
			typed, err := cast(value)
			if err != nil {
				return err
			}
			name, meta := metadata(typed)
			if err := validateMetadata(name, meta); err != nil {
				return err
			}
			if validator, ok := any(*typed).(interface{ Validate() error }); ok {
				return validator.Validate()
			}
			return nil
		},
		DefaultRisk: func(value any) models.RiskClass {
			if value == nil {
				return baseRisk
			}
			typed, err := cast(value)
			if err != nil {
				return baseRisk
			}
			return risk(typed)
		},
		ProviderFactory: func(value any, context FactoryContext) (executor.Handler, error) {
			typed, err := cast(value)
			if err != nil {
				return nil, err
			}
			return factory(typed, context)
		},
		OrderingTier: func(value any) int {
			if value == nil {
				return baseTier
			}
			typed, err := cast(value)
			if err != nil {
				return baseTier
			}
			return tier(typed)
		},
		LockDomains: func(value any) []string {
			if value == nil {
				return append([]string(nil), baseLocks...)
			}
			typed, err := cast(value)
			if err != nil {
				return append([]string(nil), baseLocks...)
			}
			_, meta := metadata(typed)
			return meta.EffectiveLockDomains(baseLocks...)
		},
		List: func(configuration *models.Configuration) []any {
			values := list(configuration)
			out := make([]any, 0, len(values))
			for _, value := range values {
				out = append(out, value)
			}
			return out
		},
		Append: func(configuration *models.Configuration, value any) error {
			typed, err := cast(value)
			if err != nil {
				return err
			}
			appendValue(configuration, *typed)
			return nil
		},
	}
}

func pointers[T any](values []T) []*T {
	out := make([]*T, 0, len(values))
	for i := range values {
		out = append(out, &values[i])
	}
	return out
}

func isCriticalFile(file *models.File) bool {
	return file != nil && (len(file.PreApplyValidation) > 0 || strings.HasPrefix(file.Path, "/etc/ssh"))
}
