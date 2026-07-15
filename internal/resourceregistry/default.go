package resourceregistry

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/applicators/accountlimits"
	"github.com/DavidHoenisch/remotr/internal/applicators/agentinstall"
	"github.com/DavidHoenisch/remotr/internal/applicators/apparmor"
	"github.com/DavidHoenisch/remotr/internal/applicators/aptkeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/aptrepositories"
	"github.com/DavidHoenisch/remotr/internal/applicators/auditrules"
	"github.com/DavidHoenisch/remotr/internal/applicators/authorizedkeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/bootstrap"
	"github.com/DavidHoenisch/remotr/internal/applicators/browserpolicy"
	"github.com/DavidHoenisch/remotr/internal/applicators/certificates"
	"github.com/DavidHoenisch/remotr/internal/applicators/command"
	"github.com/DavidHoenisch/remotr/internal/applicators/desktopsettings"
	"github.com/DavidHoenisch/remotr/internal/applicators/directories"
	"github.com/DavidHoenisch/remotr/internal/applicators/downloads"
	endpointcron "github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/cron"
	"github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/systemdtimer"
	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/applicators/firewall"
	"github.com/DavidHoenisch/remotr/internal/applicators/groups"
	"github.com/DavidHoenisch/remotr/internal/applicators/hostlocale"
	"github.com/DavidHoenisch/remotr/internal/applicators/hostname"
	"github.com/DavidHoenisch/remotr/internal/applicators/hostsentries"
	"github.com/DavidHoenisch/remotr/internal/applicators/journald"
	"github.com/DavidHoenisch/remotr/internal/applicators/kernelmodules"
	"github.com/DavidHoenisch/remotr/internal/applicators/knownhosts"
	"github.com/DavidHoenisch/remotr/internal/applicators/links"
	"github.com/DavidHoenisch/remotr/internal/applicators/loginpolicy"
	"github.com/DavidHoenisch/remotr/internal/applicators/logrotate"
	"github.com/DavidHoenisch/remotr/internal/applicators/mounts"
	"github.com/DavidHoenisch/remotr/internal/applicators/networkfiles"
	"github.com/DavidHoenisch/remotr/internal/applicators/networkmanager"
	"github.com/DavidHoenisch/remotr/internal/applicators/networkresources"
	pkgfactory "github.com/DavidHoenisch/remotr/internal/applicators/packages"
	"github.com/DavidHoenisch/remotr/internal/applicators/reboots"
	servicecontracts "github.com/DavidHoenisch/remotr/internal/applicators/services"
	"github.com/DavidHoenisch/remotr/internal/applicators/sessionpolicy"
	"github.com/DavidHoenisch/remotr/internal/applicators/sudo"
	"github.com/DavidHoenisch/remotr/internal/applicators/swaps"
	sysctlapp "github.com/DavidHoenisch/remotr/internal/applicators/sysctl"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemd"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemdunits"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
	"github.com/DavidHoenisch/remotr/internal/applicators/timesync"
	"github.com/DavidHoenisch/remotr/internal/applicators/trustanchors"
	"github.com/DavidHoenisch/remotr/internal/applicators/userfiles"
	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
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
				provider := aptrepositories.New(*v, c.Runner)
				if c.SecretResolver != nil {
					provider.ResolveCredential = secretStringResolver(c, "repository-credential")
				}
				return provider, nil
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
		definition(models.ResourceKindMount, SensitivityPublic, models.RiskBoot, 3, []string{"mount-table", "fstab"},
			func(v *models.MountResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.MountResource { return pointers(c.Mounts) },
			func(c *models.Configuration, v models.MountResource) { c.Mounts = append(c.Mounts, v) },
			func(v *models.MountResource, c FactoryContext) (executor.Handler, error) {
				return mounts.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindSwap, SensitivityPublic, models.RiskBoot, 3, []string{"swap", "fstab"}, func(v *models.SwapResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta }, func(c *models.Configuration) []*models.SwapResource { return pointers(c.Swaps) }, func(c *models.Configuration, v models.SwapResource) { c.Swaps = append(c.Swaps, v) }, func(v *models.SwapResource, c FactoryContext) (executor.Handler, error) {
			return swaps.New(*v, c.Runner), nil
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
		definition(models.ResourceKindEndpointSchedule, SensitivitySensitiveMetadata, models.RiskNormal, 7, []string{"schedule-config"},
			func(v *models.EndpointScheduleResource) (string, *models.ResourceMeta) {
				return v.Name, &v.ResourceMeta
			},
			func(c *models.Configuration) []*models.EndpointScheduleResource { return pointers(c.EndpointSchedules) },
			func(c *models.Configuration, v models.EndpointScheduleResource) {
				c.EndpointSchedules = append(c.EndpointSchedules, v)
			},
			func(v *models.EndpointScheduleResource, c FactoryContext) (executor.Handler, error) {
				switch v.Backend {
				case models.ScheduleBackendCron:
					provider := endpointcron.New(*v)
					if c.SecretResolver != nil {
						provider.ResolveSecret = secretStringResolver(c, "schedule-environment")
					}
					return provider, nil
				case models.ScheduleBackendSystemdTimer:
					provider := systemdtimer.New(*v, c.Runner)
					if c.SecretResolver != nil {
						provider.ResolveSecret = secretStringResolver(c, "schedule-environment")
					}
					return provider, nil
				default:
					return nil, fmt.Errorf("endpoint schedule backend %q is invalid", v.Backend)
				}
			}, nil, nil),
		definition(models.ResourceKindUser, SensitivitySensitiveMetadata, models.RiskAccess, 4, []string{"account-database"},
			func(v *models.UserResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.UserResource { return pointers(c.Users) },
			func(c *models.Configuration, v models.UserResource) { c.Users = append(c.Users, v) },
			func(v *models.UserResource, c FactoryContext) (executor.Handler, error) {
				provider := users.New(*v)
				if c.SecretResolver != nil {
					provider.ResolveSecret = secretStringResolver(c, "password-hash")
				}
				return provider, nil
			}, nil, nil),
		definition(models.ResourceKindUserFile, SensitivityPublic, models.RiskNormal, 5, nil,
			func(v *models.UserFileResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.UserFileResource { return pointers(c.UserFiles) },
			func(c *models.Configuration, v models.UserFileResource) { c.UserFiles = append(c.UserFiles, v) },
			func(v *models.UserFileResource, _ FactoryContext) (executor.Handler, error) {
				return userfiles.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindDesktopSetting, SensitivityPublic, models.RiskNormal, 5, []string{"desktop-policy"},
			func(v *models.DesktopSettingResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.DesktopSettingResource { return pointers(c.DesktopSettings) },
			func(c *models.Configuration, v models.DesktopSettingResource) {
				c.DesktopSettings = append(c.DesktopSettings, v)
			},
			func(v *models.DesktopSettingResource, c FactoryContext) (executor.Handler, error) {
				return desktopsettings.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindSessionPolicy, SensitivityPublic, models.RiskNormal, 5, []string{"desktop-policy"},
			func(v *models.SessionPolicyResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.SessionPolicyResource { return pointers(c.SessionPolicies) },
			func(c *models.Configuration, v models.SessionPolicyResource) {
				c.SessionPolicies = append(c.SessionPolicies, v)
			},
			func(v *models.SessionPolicyResource, c FactoryContext) (executor.Handler, error) {
				return sessionpolicy.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindBrowserPolicy, SensitivityPublic, models.RiskNormal, 5, []string{"browser-policy"},
			func(v *models.BrowserPolicyResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.BrowserPolicyResource { return pointers(c.BrowserPolicies) },
			func(c *models.Configuration, v models.BrowserPolicyResource) {
				c.BrowserPolicies = append(c.BrowserPolicies, v)
			},
			func(v *models.BrowserPolicyResource, _ FactoryContext) (executor.Handler, error) {
				return browserpolicy.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindFirewall, SensitivityPublic, models.RiskConnectivity, 6, []string{"firewall"},
			func(v *models.FirewallResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.FirewallResource { return pointers(c.Firewall) },
			func(c *models.Configuration, v models.FirewallResource) { c.Firewall = append(c.Firewall, v) },
			func(v *models.FirewallResource, c FactoryContext) (executor.Handler, error) {
				provider := firewall.New(*v, c.Runner)
				provider.SyncURL = c.SyncURL
				provider.StateDir = c.StateDir
				return provider, nil
			}, nil, nil),
		definition(models.ResourceKindHostsEntry, SensitivityPublic, models.RiskConnectivity, 6, []string{"hosts-file"},
			func(v *models.HostsEntryResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.HostsEntryResource { return pointers(c.HostsEntries) },
			func(c *models.Configuration, v models.HostsEntryResource) {
				c.HostsEntries = append(c.HostsEntries, v)
			},
			func(v *models.HostsEntryResource, c FactoryContext) (executor.Handler, error) {
				provider := hostsentries.New(*v)
				provider.SyncURL = c.SyncURL
				return provider, nil
			}, nil, nil),
		definition(models.ResourceKindDNSResolver, SensitivityPublic, models.RiskConnectivity, 6, []string{"network-configuration"},
			func(v *models.DNSResolverResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.DNSResolverResource { return pointers(c.DNSResolvers) },
			func(c *models.Configuration, v models.DNSResolverResource) {
				c.DNSResolvers = append(c.DNSResolvers, v)
			},
			func(v *models.DNSResolverResource, c FactoryContext) (executor.Handler, error) {
				return networkresources.NewDNS(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindRoute, SensitivityPublic, models.RiskConnectivity, 6, []string{"network-configuration"},
			func(v *models.RouteResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.RouteResource { return pointers(c.Routes) },
			func(c *models.Configuration, v models.RouteResource) { c.Routes = append(c.Routes, v) },
			func(v *models.RouteResource, c FactoryContext) (executor.Handler, error) {
				return networkresources.NewRoute(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindNetworkProfile, SensitivitySensitiveMetadata, models.RiskConnectivity, 6, []string{"network-configuration"},
			func(v *models.NetworkProfileResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.NetworkProfileResource { return pointers(c.NetworkProfiles) },
			func(c *models.Configuration, v models.NetworkProfileResource) {
				c.NetworkProfiles = append(c.NetworkProfiles, v)
			},
			func(v *models.NetworkProfileResource, c FactoryContext) (executor.Handler, error) {
				if v.Provider != models.NetworkProviderNetworkManager {
					provider := networkfiles.New(*v, c.Runner)
					provider.StateDir = c.StateDir
					return provider, nil
				}
				provider := networkmanager.NewProfile(*v, c.Runner)
				provider.StateDir = c.StateDir
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
		definition(models.ResourceKindService, SensitivityPublic, models.RiskNormal, 7, []string{"service-manager"},
			func(v *models.ServiceResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.ServiceResource { return pointers(c.Services) },
			func(c *models.Configuration, v models.ServiceResource) { c.Services = append(c.Services, v) },
			func(v *models.ServiceResource, c FactoryContext) (executor.Handler, error) {
				contract, known := servicecontracts.ContractFor(v.Provider)
				if !known {
					return nil, fmt.Errorf("service %q selects unknown provider %q", v.Name, v.Provider)
				}
				if err := contract.RequireAdvertised(); err != nil {
					return nil, err
				}
				switch v.Scope {
				case models.ServiceScopeSystem:
					return systemd.New(models.SystemdResource{
						ResourceMeta: v.ResourceMeta, Name: v.Name, Unit: v.Service,
						Enabled: v.Enabled, Active: v.Active, Masked: v.Masked,
					}, c.Runner), nil
				case models.ServiceScopeUser:
					return systemduser.New(models.SystemdUserResource{
						ResourceMeta: v.ResourceMeta, Name: v.Name, Unit: v.Service, Users: v.Users, Linger: v.Linger,
						Enabled: v.Enabled, Active: v.Active, Masked: v.Masked,
					}, c.Runner), nil
				default:
					return nil, fmt.Errorf("service %q has unsupported scope %q", v.Name, v.Scope)
				}
			}, nil, nil),
		definition(models.ResourceKindSystemdUnit, SensitivityPublic, models.RiskNormal, 6, []string{"systemd-unit-files"},
			func(v *models.SystemdUnitResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.SystemdUnitResource { return pointers(c.SystemdUnits) },
			func(c *models.Configuration, v models.SystemdUnitResource) {
				c.SystemdUnits = append(c.SystemdUnits, v)
			},
			func(v *models.SystemdUnitResource, c FactoryContext) (executor.Handler, error) {
				return systemdunits.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindReboot, SensitivityPublic, models.RiskBoot, 9, []string{"reboot"},
			func(v *models.RebootResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.RebootResource { return pointers(c.Reboots) },
			func(c *models.Configuration, v models.RebootResource) { c.Reboots = append(c.Reboots, v) },
			func(v *models.RebootResource, c FactoryContext) (executor.Handler, error) {
				return reboots.New(*v, rebootstate.New(c.StateDir), reboots.SystemProbes{Runner: c.Runner}, nil), nil
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
		definition(models.ResourceKindCertificate, SensitivitySecret, models.RiskSensitive, 6, []string{"certificate-files"},
			func(v *models.CertificateResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.CertificateResource { return pointers(c.Certificates) },
			func(c *models.Configuration, v models.CertificateResource) {
				c.Certificates = append(c.Certificates, v)
			},
			func(v *models.CertificateResource, c FactoryContext) (executor.Handler, error) {
				provider := certificates.New(*v)
				if c.SecretResolver != nil {
					provider.ResolveWithPurpose = secretBytesPurposeResolver(c)
				}
				return provider, nil
			}, nil, nil),
		definition(models.ResourceKindTrustAnchor, SensitivityPublic, models.RiskSensitive, 6, []string{"ca-trust-store"},
			func(v *models.TrustAnchorResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.TrustAnchorResource { return pointers(c.TrustAnchors) },
			func(c *models.Configuration, v models.TrustAnchorResource) {
				c.TrustAnchors = append(c.TrustAnchors, v)
			},
			func(v *models.TrustAnchorResource, c FactoryContext) (executor.Handler, error) {
				provider, err := trustanchors.New(*v, c.Facts.Distro)
				if err != nil {
					return nil, err
				}
				if c.SecretResolver != nil {
					provider.Resolve = secretBytesResolver(c, "ca-trust-anchor")
				}
				return provider, nil
			}, nil, nil),
		definition(models.ResourceKindAppArmorProfile, SensitivitySensitiveMetadata, models.RiskSensitive, 6, []string{"apparmor-policy"},
			func(v *models.AppArmorProfileResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.AppArmorProfileResource { return pointers(c.AppArmorProfiles) },
			func(c *models.Configuration, v models.AppArmorProfileResource) {
				c.AppArmorProfiles = append(c.AppArmorProfiles, v)
			},
			func(v *models.AppArmorProfileResource, c FactoryContext) (executor.Handler, error) {
				return apparmor.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindAuditRules, SensitivitySensitiveMetadata, models.RiskSensitive, 6, []string{"audit-policy"},
			func(v *models.AuditRulesResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.AuditRulesResource { return pointers(c.AuditRules) },
			func(c *models.Configuration, v models.AuditRulesResource) { c.AuditRules = append(c.AuditRules, v) },
			func(v *models.AuditRulesResource, c FactoryContext) (executor.Handler, error) {
				return auditrules.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindAccountLimit, SensitivitySensitiveMetadata, models.RiskAccess, 6, []string{"account-limits"},
			func(v *models.AccountLimitResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.AccountLimitResource { return pointers(c.AccountLimits) },
			func(c *models.Configuration, v models.AccountLimitResource) {
				c.AccountLimits = append(c.AccountLimits, v)
			},
			func(v *models.AccountLimitResource, _ FactoryContext) (executor.Handler, error) {
				return accountlimits.New(*v), nil
			}, nil, nil),
		definition(models.ResourceKindLoginPolicy, SensitivitySensitiveMetadata, models.RiskAccess, 6, []string{"pam-policy"},
			func(v *models.LoginPolicyResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.LoginPolicyResource { return pointers(c.LoginPolicies) },
			func(c *models.Configuration, v models.LoginPolicyResource) {
				c.LoginPolicies = append(c.LoginPolicies, v)
			},
			func(v *models.LoginPolicyResource, c FactoryContext) (executor.Handler, error) {
				return loginpolicy.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindJournald, SensitivitySensitiveMetadata, models.RiskSensitive, 7, []string{"journald-policy"},
			func(v *models.JournaldResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.JournaldResource { return pointers(c.Journald) },
			func(c *models.Configuration, v models.JournaldResource) { c.Journald = append(c.Journald, v) },
			func(v *models.JournaldResource, c FactoryContext) (executor.Handler, error) {
				return journald.New(*v, c.Runner), nil
			}, nil, nil),
		definition(models.ResourceKindLogrotate, SensitivitySensitiveMetadata, models.RiskSensitive, 7, []string{"logrotate-policy"},
			func(v *models.LogrotateResource) (string, *models.ResourceMeta) { return v.Name, &v.ResourceMeta },
			func(c *models.Configuration) []*models.LogrotateResource { return pointers(c.Logrotate) },
			func(c *models.Configuration, v models.LogrotateResource) { c.Logrotate = append(c.Logrotate, v) },
			func(v *models.LogrotateResource, c FactoryContext) (executor.Handler, error) {
				return logrotate.New(*v, c.Runner), nil
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

func secretStringResolver(factoryContext FactoryContext, purpose string) func(context.Context, string) (string, error) {
	return func(ctx context.Context, reference string) (string, error) {
		resolved, err := factoryContext.SecretResolver.Resolve(ctx, secrets.ResolveRequest{
			Reference: reference, ArtifactDigest: factoryContext.ArtifactDigest,
			ResourceAddress: factoryContext.ResourceAddress, Purpose: purpose,
		})
		if err != nil {
			return "", secrets.RedactedResolutionError(err)
		}
		return string(resolved.Material), nil
	}
}

func secretBytesResolver(factoryContext FactoryContext, purpose string) func(context.Context, string) ([]byte, error) {
	return func(ctx context.Context, reference string) ([]byte, error) {
		resolved, err := factoryContext.SecretResolver.Resolve(ctx, secrets.ResolveRequest{
			Reference: reference, ArtifactDigest: factoryContext.ArtifactDigest,
			ResourceAddress: factoryContext.ResourceAddress, Purpose: purpose,
		})
		if err != nil {
			return nil, secrets.RedactedResolutionError(err)
		}
		return resolved.Material, nil
	}
}

func secretBytesPurposeResolver(factoryContext FactoryContext) func(context.Context, string, string) ([]byte, error) {
	return func(ctx context.Context, reference, purpose string) ([]byte, error) {
		return secretBytesResolver(factoryContext, purpose)(ctx, reference)
	}
}

func isCriticalFile(file *models.File) bool {
	return file != nil && (len(file.PreApplyValidation) > 0 || strings.HasPrefix(file.Path, "/etc/ssh"))
}
