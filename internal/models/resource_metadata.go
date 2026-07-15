package models

import (
	"fmt"
	"strings"
)

// ResourceKind identifies a portable desired-state resource contract.
type ResourceKind string

const (
	ResourceKindPackage          ResourceKind = "package"
	ResourceKindAPTSigningKey    ResourceKind = "aptSigningKey"
	ResourceKindAPTRepository    ResourceKind = "aptRepository"
	ResourceKindSysctl           ResourceKind = "sysctl"
	ResourceKindKernelModule     ResourceKind = "kernelModule"
	ResourceKindHostname         ResourceKind = "hostname"
	ResourceKindHostLocale       ResourceKind = "hostLocale"
	ResourceKindTimeSync         ResourceKind = "timeSync"
	ResourceKindMount            ResourceKind = "mount"
	ResourceKindSwap             ResourceKind = "swap"
	ResourceKindEndpointSchedule ResourceKind = "endpointSchedule"
	ResourceKindFile             ResourceKind = "file"
	ResourceKindDirectory        ResourceKind = "directory"
	ResourceKindLink             ResourceKind = "link"
	ResourceKindGroup            ResourceKind = "group"
	ResourceKindAuthorizedKey    ResourceKind = "authorizedKey"
	ResourceKindKnownHost        ResourceKind = "knownHost"
	ResourceKindSudo             ResourceKind = "sudo"
	ResourceKindUserFile         ResourceKind = "userFile"
	ResourceKindDownload         ResourceKind = "download"
	ResourceKindUser             ResourceKind = "user"
	ResourceKindSystemd          ResourceKind = "systemd"
	ResourceKindSystemdUser      ResourceKind = "systemdUser"
	ResourceKindService          ResourceKind = "service"
	ResourceKindSystemdUnit      ResourceKind = "systemdUnit"
	ResourceKindReboot           ResourceKind = "reboot"
	ResourceKindBootstrap        ResourceKind = "bootstrap"
	ResourceKindAgentInstall     ResourceKind = "agentInstall"
	ResourceKindFirewall         ResourceKind = "firewall"
	ResourceKindHostsEntry       ResourceKind = "hostsEntry"
	ResourceKindDNSResolver      ResourceKind = "dnsResolver"
	ResourceKindRoute            ResourceKind = "route"
	ResourceKindNetworkProfile   ResourceKind = "networkProfile"
	ResourceKindCertificate      ResourceKind = "certificate"
	ResourceKindTrustAnchor      ResourceKind = "trustAnchor"
	ResourceKindAppArmorProfile  ResourceKind = "appArmorProfile"
	ResourceKindAuditRules       ResourceKind = "auditRules"
	ResourceKindAccountLimit     ResourceKind = "accountLimit"
	ResourceKindCommand          ResourceKind = "command"
)

// Valid reports whether the kind belongs to the schema-1 resource vocabulary.
func (k ResourceKind) Valid() bool {
	switch k {
	case ResourceKindPackage, ResourceKindAPTSigningKey, ResourceKindAPTRepository, ResourceKindSysctl, ResourceKindKernelModule, ResourceKindHostname, ResourceKindHostLocale, ResourceKindTimeSync, ResourceKindMount, ResourceKindSwap, ResourceKindEndpointSchedule, ResourceKindFile, ResourceKindDirectory, ResourceKindLink, ResourceKindGroup, ResourceKindAuthorizedKey, ResourceKindKnownHost, ResourceKindSudo, ResourceKindUserFile,
		ResourceKindDownload, ResourceKindUser, ResourceKindSystemd,
		ResourceKindSystemdUser, ResourceKindService, ResourceKindSystemdUnit, ResourceKindReboot, ResourceKindBootstrap,
		ResourceKindAgentInstall, ResourceKindFirewall, ResourceKindHostsEntry, ResourceKindDNSResolver, ResourceKindRoute, ResourceKindNetworkProfile, ResourceKindCertificate, ResourceKindTrustAnchor, ResourceKindAppArmorProfile, ResourceKindAuditRules, ResourceKindAccountLimit, ResourceKindCommand:
		return true
	default:
		return false
	}
}

// ResourceHeader is the canonical identity shared by every resource kind.
type ResourceHeader struct {
	Kind ResourceKind `yaml:"kind"`
	Name string       `yaml:"name"`
}

// Lifecycle is the requested presence/lifecycle state. Individual resource
// definitions further restrict which values they can truthfully implement.
type Lifecycle string

const (
	LifecyclePresent  Lifecycle = "present"
	LifecycleAbsent   Lifecycle = "absent"
	LifecycleDisabled Lifecycle = "disabled"
	LifecyclePurged   Lifecycle = "purged"
)

func (l Lifecycle) Valid() bool {
	switch l {
	case LifecyclePresent, LifecycleAbsent, LifecycleDisabled, LifecyclePurged:
		return true
	default:
		return false
	}
}

// RemediationPolicy controls whether observed drift may be enforced.
type RemediationPolicy string

const (
	RemediationAuto   RemediationPolicy = "auto"
	RemediationReport RemediationPolicy = "report"
)

func (p RemediationPolicy) Valid() bool { return p == RemediationAuto || p == RemediationReport }

// OwnershipMode states the boundary within which a resource may mutate or clean up state.
type OwnershipMode string

const (
	OwnershipNamed         OwnershipMode = "named"
	OwnershipFragment      OwnershipMode = "fragment"
	OwnershipMerge         OwnershipMode = "merge"
	OwnershipAuthoritative OwnershipMode = "authoritative"
)

func (o OwnershipMode) Valid() bool {
	switch o {
	case OwnershipNamed, OwnershipFragment, OwnershipMerge, OwnershipAuthoritative:
		return true
	default:
		return false
	}
}

// ValidationRule declares an argv-based validation performed before activation.
type ValidationRule struct {
	Command []string `yaml:"command"`
}

// NotificationType identifies a structured post-change activation request.
type NotificationType string

const (
	NotificationDaemonReload NotificationType = "daemon-reload"
	NotificationReload       NotificationType = "reload"
	NotificationTryRestart   NotificationType = "try-restart"
	NotificationRestart      NotificationType = "restart"
	NotificationLogout       NotificationType = "logout-required"
	NotificationNextBoot     NotificationType = "next-boot"
	NotificationReboot       NotificationType = "reboot-required"
)

func (n NotificationType) Valid() bool {
	switch n {
	case NotificationDaemonReload, NotificationReload, NotificationTryRestart, NotificationRestart,
		NotificationLogout, NotificationNextBoot, NotificationReboot:
		return true
	default:
		return false
	}
}

// Notification is a structured activation request emitted after a successful change.
type Notification struct {
	Type   NotificationType `yaml:"type"`
	Target string           `yaml:"target,omitempty"`
}

// ValidateCanonical validates shared metadata independently of kind-specific fields.
func (m ResourceMeta) ValidateCanonical() error {
	if !m.Kind.Valid() {
		return fmt.Errorf("unknown resource kind %q", m.Kind)
	}
	if m.Lifecycle != "" && !m.Lifecycle.Valid() {
		return fmt.Errorf("unknown lifecycle %q", m.Lifecycle)
	}
	if m.Policy != "" && !m.Policy.Valid() {
		return fmt.Errorf("unknown remediation policy %q", m.Policy)
	}
	if m.Ownership != "" && !m.Ownership.Valid() {
		return fmt.Errorf("unknown ownership mode %q", m.Ownership)
	}
	if m.Risk != "" && !m.Risk.Valid() {
		return fmt.Errorf("unknown risk %q", m.Risk)
	}
	if strings.TrimSpace(m.AuthorizationGroup) != m.AuthorizationGroup {
		return fmt.Errorf("authorizationGroup must not have surrounding whitespace")
	}
	for provider := range m.ProviderOptions {
		if strings.TrimSpace(provider) == "" {
			return fmt.Errorf("providerOptions contains an empty provider name")
		}
	}
	for i, rule := range m.Validation {
		if len(rule.Command) == 0 || strings.TrimSpace(rule.Command[0]) == "" {
			return fmt.Errorf("validation rule %d requires non-empty command argv", i+1)
		}
	}
	for i, notification := range m.Notifications {
		if !notification.Type.Valid() {
			return fmt.Errorf("notification %d has unknown type %q", i+1, notification.Type)
		}
		if (notification.Type == NotificationReload || notification.Type == NotificationTryRestart || notification.Type == NotificationRestart) && strings.TrimSpace(notification.Target) == "" {
			return fmt.Errorf("notification %d type %q requires target", i+1, notification.Type)
		}
	}
	return nil
}
