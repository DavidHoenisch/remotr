package models

import (
	"time"

	"github.com/DavidHoenisch/remotr/internal/types"
)

// RiskClass classifies the safety impact of a resource mutation.
type RiskClass string

const (
	RiskNormal       RiskClass = "normal"
	RiskSensitive    RiskClass = "sensitive"
	RiskConnectivity RiskClass = "connectivity"
	RiskAccess       RiskClass = "access"
	RiskBoot         RiskClass = "boot"
	RiskDestructive  RiskClass = "destructive"
)

// Valid reports whether risk is a declared resource risk class.
func (r RiskClass) Valid() bool {
	switch r {
	case RiskNormal, RiskSensitive, RiskConnectivity, RiskAccess, RiskBoot, RiskDestructive:
		return true
	default:
		return false
	}
}

// RequiresPreflight reports whether a risk class requires explicit approval
// and a resource-specific safety preflight before Apply.
func (r RiskClass) RequiresPreflight() bool {
	return r != RiskNormal
}

// ResourceMeta holds dependency, validation, and safety metadata shared by resources.
type ResourceMeta struct {
	Kind               ResourceKind              `yaml:"-"`
	Lifecycle          Lifecycle                 `yaml:"lifecycle,omitempty"`
	DependsOn          []string                  `yaml:"dependsOn,omitempty"`
	ProviderOptions    map[string]map[string]any `yaml:"providerOptions,omitempty"`
	Policy             RemediationPolicy         `yaml:"policy,omitempty"`
	Ownership          OwnershipMode             `yaml:"ownership,omitempty"`
	Validation         []ValidationRule          `yaml:"validation,omitempty"`
	Notifications      []Notification            `yaml:"notifications,omitempty"`
	PreApplyValidation []string                  `yaml:"preApplyValidation,omitempty"`
	Risk               RiskClass                 `yaml:"risk,omitempty"`
	AuthorizationGroup string                    `yaml:"authorizationGroup,omitempty"`
	Enforce            *bool                     `yaml:"enforce,omitempty"`
	LockDomains        []string                  `yaml:"lockDomains,omitempty"`
}

// EffectiveRisk returns the author override when present or the resource's
// conservative default risk class otherwise.
func (m ResourceMeta) EffectiveRisk(defaultRisk RiskClass) RiskClass {
	if m.Risk != "" {
		return m.Risk
	}
	return defaultRisk
}

// EffectiveLockDomains combines a resource's mandatory and declared lock domains.
func (m ResourceMeta) EffectiveLockDomains(defaultDomains ...string) []string {
	domains := append([]string(nil), defaultDomains...)
	return append(domains, m.LockDomains...)
}

type Package struct {
	ResourceMeta       `yaml:",inline"`
	Name               string               `yaml:"name"`
	Present            bool                 `yaml:"present"`
	Version            string               `yaml:"version,omitempty"`
	AllowUpgrade       *bool                `yaml:"allowUpgrade,omitempty"`
	AllowDowngrade     *bool                `yaml:"allowDowngrade,omitempty"`
	Hold               *bool                `yaml:"hold,omitempty"`
	RefreshCache       bool                 `yaml:"refreshCache,omitempty"`
	RemoveDependencies bool                 `yaml:"removeDependencies,omitempty"`
	NonInteractive     *bool                `yaml:"nonInteractive,omitempty"`
	Arch               types.Architecture   `yaml:"arch,omitempty"`
	PM                 types.PackageManager `yaml:"packageManager,omitempty"`
	FlatpakRemote      string               `yaml:"flatpakRemote,omitempty"`
	FlatpakRemoteURL   string               `yaml:"flatpakRemoteURL,omitempty"`
	PWAURL             string               `yaml:"pwaURL,omitempty"`
	PWATitle           string               `yaml:"pwaTitle,omitempty"`
	PWAIcon            string               `yaml:"pwaIcon,omitempty"`
	PWABrowser         string               `yaml:"pwaBrowser,omitempty"`
	PWAUsers           string               `yaml:"pwaUsers,omitempty"`
}

// APTSigningKey manages one APT repository signing key in a dedicated,
// Remotr-owned keyring. Repository resources refer to this resource through
// normal stable dependencies; key material never belongs in a source fragment.
type APTSigningKey struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Source       string `yaml:"source,omitempty"`
	Fingerprint  string `yaml:"fingerprint,omitempty"`
}

// APTRepository owns the source, optional preference, and optional protected
// auth fragment for one named APT repository. CredentialRef identifies secret
// material kept outside desired state; it is never an embedded URL credential.
type APTRepository struct {
	ResourceMeta  `yaml:",inline"`
	Name          string   `yaml:"name"`
	URL           string   `yaml:"url,omitempty"`
	Suites        []string `yaml:"suites,omitempty"`
	Components    []string `yaml:"components,omitempty"`
	Architectures []string `yaml:"architectures,omitempty"`
	SigningKey    string   `yaml:"signingKey,omitempty"`
	Priority      int      `yaml:"priority,omitempty"`
	CredentialRef string   `yaml:"credentialRef,omitempty"`
}

// SysctlResource manages one Linux kernel sysctl independently at runtime and
// at boot through a named Remotr-owned /etc/sysctl.d drop-in.
type SysctlResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string           `yaml:"name"`
	Key          string           `yaml:"key,omitempty"`
	Value        string           `yaml:"value,omitempty"`
	Runtime      bool             `yaml:"runtime,omitempty"`
	Persistent   bool             `yaml:"persistent,omitempty"`
	Activation   SysctlActivation `yaml:"activation,omitempty"`
}

// HostnameResource manages the persistent static hostname and the active
// transient hostname as separate optional fields.
type HostnameResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string  `yaml:"name"`
	Static       *string `yaml:"static,omitempty"`
	Transient    *string `yaml:"transient,omitempty"`
}

// HostLocaleResource independently manages the timezone, system locale
// variables, and console keymap. Nil fields deliberately leave their scope
// unmanaged.
type HostLocaleResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string            `yaml:"name"`
	Timezone     *string           `yaml:"timezone,omitempty"`
	Locale       map[string]string `yaml:"locale,omitempty"`
	Keymap       *string           `yaml:"keymap,omitempty"`
}

// TimeSyncResource selects a time-synchronization provider and independently
// manages its enablement and optional server/pool configuration.
type TimeSyncResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string   `yaml:"name"`
	Provider     string   `yaml:"provider"`
	Enabled      *bool    `yaml:"enabled,omitempty"`
	Servers      []string `yaml:"servers,omitempty"`
	Pools        []string `yaml:"pools,omitempty"`
}

// MountResource manages runtime activation and a precisely owned fstab entry.
type MountResource struct {
	ResourceMeta   `yaml:",inline"`
	Name           string      `yaml:"name"`
	Source         string      `yaml:"source"`
	Target         string      `yaml:"target"`
	FilesystemType string      `yaml:"filesystemType"`
	Options        []string    `yaml:"options,omitempty"`
	Dump           int         `yaml:"dump,omitempty"`
	Pass           int         `yaml:"pass,omitempty"`
	Mounted        *bool       `yaml:"mounted,omitempty"`
	Persistent     *bool       `yaml:"persistent,omitempty"`
	UnmountMode    UnmountMode `yaml:"unmountMode,omitempty"`
}

// SwapResource manages a swap file or existing swap device at runtime and boot.
type SwapResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Path         string `yaml:"path"`
	Type         string `yaml:"type"`
	SizeBytes    int64  `yaml:"sizeBytes,omitempty"`
	Priority     int    `yaml:"priority,omitempty"`
	Active       *bool  `yaml:"active,omitempty"`
	Persistent   *bool  `yaml:"persistent,omitempty"`
	AllowRemove  bool   `yaml:"allowRemove,omitempty"`
}

// NormalizeLifecycle maps the schema-0 present boolean to the explicit package
// lifecycle and keeps Present populated for applicators during migration.
func (p *Package) NormalizeLifecycle() {
	if p.Lifecycle == "" {
		if p.Present {
			p.Lifecycle = LifecyclePresent
		} else {
			p.Lifecycle = LifecycleAbsent
		}
	}
	p.Present = p.Lifecycle == LifecyclePresent
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
	Owner          string `yaml:"owner,omitempty"`
	Group          string `yaml:"group,omitempty"`
}

// DirectoryResource manages one filesystem directory and its optional
// ownership and mode. Recursive contents are intentionally out of scope for
// this resource's initial contract.
type DirectoryResource struct {
	ResourceMeta         `yaml:",inline"`
	Name                 string   `yaml:"name"`
	Path                 string   `yaml:"path"`
	Mode                 []int    `yaml:"mode,omitempty"`
	Owner                string   `yaml:"owner,omitempty"`
	Group                string   `yaml:"group,omitempty"`
	AllowTypeReplacement bool     `yaml:"allowTypeReplacement,omitempty"`
	Recursive            bool     `yaml:"recursive,omitempty"`
	Purge                bool     `yaml:"purge,omitempty"`
	CrossFilesystem      bool     `yaml:"crossFilesystem,omitempty"`
	Exclusions           []string `yaml:"exclusions,omitempty"`
	MaxDepth             int      `yaml:"maxDepth,omitempty"`
	MaxEntries           int      `yaml:"maxEntries,omitempty"`
}

// UserFileResource applies file operations under each interactive user's home directory.
type UserFileResource struct {
	ResourceMeta   `yaml:",inline"`
	Name           string `yaml:"name"`
	Users          string `yaml:"users"`
	Path           string `yaml:"path"`
	UpdateExisting bool   `yaml:"updateExisting,omitempty"`
	WithRegx       string `yaml:"withRegx,omitempty"`
	ReplaceRegx    string `yaml:"replaceRegx,omitempty"`
	Content        string `yaml:"content,omitempty"`
	Mode           []int  `yaml:"mode,omitempty"`
}

// ToFile returns a system File with an absolute path for the files applicator.
func (u UserFileResource) ToFile(absPath string) File {
	return File{
		ResourceMeta:   u.ResourceMeta,
		Name:           u.Name,
		Path:           absPath,
		UpdateExisting: u.UpdateExisting,
		WithRegx:       u.WithRegx,
		ReplaceRegx:    u.ReplaceRegx,
		Content:        u.Content,
		Mode:           u.Mode,
	}
}

// DownloadResource fetches a remote file to a fixed destination path.
type DownloadResource struct {
	ResourceMeta      `yaml:",inline"`
	Name              string   `yaml:"name"`
	URL               string   `yaml:"url"`
	Dest              string   `yaml:"dest"`
	Mode              []int    `yaml:"mode,omitempty"`
	Owner             string   `yaml:"owner,omitempty"`
	Group             string   `yaml:"group,omitempty"`
	Checksum          string   `yaml:"checksum,omitempty"`
	Signature         string   `yaml:"signature,omitempty"`
	TrustedSigner     string   `yaml:"trustedSigner,omitempty"`
	AuthenticationRef string   `yaml:"authenticationRef,omitempty"`
	RedirectPolicy    string   `yaml:"redirectPolicy,omitempty"`
	Timeout           string   `yaml:"timeout,omitempty"`
	NotifySystemd     string   `yaml:"notifySystemd,omitempty"`
	ReloadExec        []string `yaml:"reloadExec,omitempty"`
}

// UserResource declares a local user account.
type UserResource struct {
	ResourceMeta            `yaml:",inline"`
	Name                    string              `yaml:"name"`
	Username                string              `yaml:"username"`
	Present                 bool                `yaml:"present"`
	UID                     int                 `yaml:"uid,omitempty"`
	AllowUIDReassignment    bool                `yaml:"allowUIDReassignment,omitempty"`
	PrimaryGroup            string              `yaml:"primaryGroup,omitempty"`
	SupplementaryGroups     []string            `yaml:"supplementaryGroups,omitempty"`
	SupplementaryGroupsMode GroupMembershipMode `yaml:"supplementaryGroupsMode,omitempty"`
	Home                    string              `yaml:"home,omitempty"`
	CreateHome              *bool               `yaml:"createHome,omitempty"`
	Shell                   string              `yaml:"shell,omitempty"`
	Comment                 string              `yaml:"comment,omitempty"`
	System                  *bool               `yaml:"system,omitempty"`
	PasswordHashRef         string              `yaml:"passwordHashRef,omitempty"`
	Locked                  *bool               `yaml:"locked,omitempty"`
	Expiry                  string              `yaml:"expiry,omitempty"`
	RemoveHome              bool                `yaml:"removeHome,omitempty"`
	ForceRemoval            bool                `yaml:"forceRemoval,omitempty"`
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
	Masked       *bool  `yaml:"masked,omitempty"`
	UnitPath     string `yaml:"unitPath,omitempty"`
}

// FirewallResource declares a firewall rule using a unified abstraction.
// Audit mode is default (audit=true) to prevent accidental lockouts.
type FirewallResource struct {
	ResourceMeta    `yaml:",inline"`
	Name            string         `yaml:"name"`
	Audit           *bool          `yaml:"audit,omitempty"`
	Action          string         `yaml:"action"`
	Protocol        string         `yaml:"protocol,omitempty"`
	Ports           []int          `yaml:"ports,omitempty"`
	Sources         []string       `yaml:"sources,omitempty"`
	Destinations    []string       `yaml:"destinations,omitempty"`
	Services        []string       `yaml:"services,omitempty"`
	Zones           []string       `yaml:"zones,omitempty"`
	Backend         string         `yaml:"backend,omitempty"`
	Table           string         `yaml:"table,omitempty"`
	Chain           string         `yaml:"chain,omitempty"`
	Family          string         `yaml:"family,omitempty"`
	Rule            string         `yaml:"rule,omitempty"`
	ProtectRemotr   *bool          `yaml:"protectRemotr,omitempty"`
	RollbackTimeout string         `yaml:"rollbackTimeout,omitempty"`
	CleanupLimit    int            `yaml:"cleanupLimit,omitempty"`
	Rules           []FirewallRule `yaml:"rules,omitempty"`
}

// FirewallRule is one member of an owned firewall chain, zone, or
// authoritative set. The parent FirewallResource supplies the provider and
// ownership boundary.
type FirewallRule struct {
	Name         string   `yaml:"name"`
	Action       string   `yaml:"action"`
	Protocol     string   `yaml:"protocol,omitempty"`
	Ports        []int    `yaml:"ports,omitempty"`
	Sources      []string `yaml:"sources,omitempty"`
	Destinations []string `yaml:"destinations,omitempty"`
	Services     []string `yaml:"services,omitempty"`
	Rule         string   `yaml:"rule,omitempty"`
}

// HostsEntryResource owns one stable marked entry in /etc/hosts.
type HostsEntryResource struct {
	ResourceMeta  `yaml:",inline"`
	Name          string   `yaml:"name"`
	Address       string   `yaml:"address,omitempty"`
	CanonicalHost string   `yaml:"canonicalHost,omitempty"`
	Aliases       []string `yaml:"aliases,omitempty"`
}

// IsAudit returns true when audit mode is enabled (default true).
func (f FirewallResource) IsAudit() bool {
	if f.Audit == nil {
		return true
	}
	return *f.Audit
}

// IsProtectRemotr returns true when remotr sync-path protection is enabled (default true).
func (f FirewallResource) IsProtectRemotr() bool {
	if f.ProtectRemotr == nil {
		return true
	}
	return *f.ProtectRemotr
}

// CommandResource is an escape hatch with explicit check/apply/revert argv.
type CommandResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string   `yaml:"name"`
	Check        []string `yaml:"check,omitempty"`
	Apply        []string `yaml:"apply,omitempty"`
	Revert       []string `yaml:"revert,omitempty"`
}

// RebootResource coordinates one explicitly generated reboot intent. Reusing
// a completed generation is compliant and cannot trigger another reboot.
type RebootResource struct {
	ResourceMeta       `yaml:",inline"`
	Name               string                   `yaml:"name"`
	Generation         string                   `yaml:"generation"`
	OnlyIfRequired     bool                     `yaml:"onlyIfRequired,omitempty"`
	Delay              string                   `yaml:"delay,omitempty"`
	Timeout            string                   `yaml:"timeout"`
	Deadline           string                   `yaml:"deadline,omitempty"`
	MaintenanceWindow  *RebootMaintenanceWindow `yaml:"maintenanceWindow,omitempty"`
	RequireACPower     bool                     `yaml:"requireACPower,omitempty"`
	UserInhibition     InhibitionPolicy         `yaml:"userInhibition,omitempty"`
	WorkloadInhibition InhibitionPolicy         `yaml:"workloadInhibition,omitempty"`
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
	Name              string                     `yaml:"name"`
	Description       string                     `yaml:"description,omitempty"`
	LastUpdated       time.Time                  `yaml:"lastUpdated,omitempty"`
	TargetDistros     []types.Distro             `yaml:"targetDistros,omitempty"`
	TargetArch        []types.Architecture       `yaml:"targetArch,omitempty"`
	Packages          []Package                  `yaml:"packages,omitempty"`
	APTSigningKeys    []APTSigningKey            `yaml:"aptSigningKeys,omitempty"`
	APTRepositories   []APTRepository            `yaml:"aptRepositories,omitempty"`
	Sysctls           []SysctlResource           `yaml:"sysctls,omitempty"`
	KernelModules     []KernelModuleResource     `yaml:"kernelModules,omitempty"`
	Hostnames         []HostnameResource         `yaml:"hostnames,omitempty"`
	HostLocales       []HostLocaleResource       `yaml:"hostLocales,omitempty"`
	TimeSync          []TimeSyncResource         `yaml:"timeSync,omitempty"`
	Mounts            []MountResource            `yaml:"mounts,omitempty"`
	Swaps             []SwapResource             `yaml:"swaps,omitempty"`
	EndpointSchedules []EndpointScheduleResource `yaml:"endpointSchedules,omitempty"`
	Files             []File                     `yaml:"files,omitempty"`
	Directories       []DirectoryResource        `yaml:"directories,omitempty"`
	Links             []LinkResource             `yaml:"links,omitempty"`
	Groups            []GroupResource            `yaml:"groups,omitempty"`
	AuthorizedKeys    []AuthorizedKeyResource    `yaml:"authorizedKeys,omitempty"`
	KnownHosts        []KnownHostResource        `yaml:"knownHosts,omitempty"`
	Sudo              []SudoResource             `yaml:"sudo,omitempty"`
	UserFiles         []UserFileResource         `yaml:"userFiles,omitempty"`
	Downloads         []DownloadResource         `yaml:"downloads,omitempty"`
	Users             []UserResource             `yaml:"users,omitempty"`
	Systemd           []SystemdResource          `yaml:"systemd,omitempty"`
	SystemdUser       []SystemdUserResource      `yaml:"systemdUser,omitempty"`
	Services          []ServiceResource          `yaml:"services,omitempty"`
	SystemdUnits      []SystemdUnitResource      `yaml:"systemdUnits,omitempty"`
	Reboots           []RebootResource           `yaml:"reboots,omitempty"`
	Bootstrap         []BootstrapResource        `yaml:"bootstrap,omitempty"`
	AgentInstall      []AgentInstallResource     `yaml:"agentInstall,omitempty"`
	Firewall          []FirewallResource         `yaml:"firewall,omitempty"`
	HostsEntries      []HostsEntryResource       `yaml:"hostsEntries,omitempty"`
	DNSResolvers      []DNSResolverResource      `yaml:"dnsResolvers,omitempty"`
	Routes            []RouteResource            `yaml:"routes,omitempty"`
	NetworkProfiles   []NetworkProfileResource   `yaml:"networkProfiles,omitempty"`
	Certificates      []CertificateResource      `yaml:"certificates,omitempty"`
	TrustAnchors      []TrustAnchorResource      `yaml:"trustAnchors,omitempty"`
	AppArmorProfiles  []AppArmorProfileResource  `yaml:"appArmorProfiles,omitempty"`
	AuditRules        []AuditRulesResource       `yaml:"auditRules,omitempty"`
	AccountLimits     []AccountLimitResource     `yaml:"accountLimits,omitempty"`
	LoginPolicies     []LoginPolicyResource      `yaml:"loginPolicies,omitempty"`
	Journald          []JournaldResource         `yaml:"journald,omitempty"`
	Logrotate         []LogrotateResource        `yaml:"logrotate,omitempty"`
	Commands          []CommandResource          `yaml:"commands,omitempty"`
}

type State struct {
	SchemaVersion  int             `yaml:"schemaVersion,omitempty"`
	Kind           types.Kind      `yaml:"kind,omitempty"`
	Configurations []Configuration `yaml:"configurations"`
	Diagnostics    []Diagnostic    `yaml:"-" json:"-"`
}

// ResourceAddress returns configuration-name/resource-name.
func ResourceAddress(configName, resourceName string) string {
	return configName + "/" + resourceName
}
