package resourceregistry

import (
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
)

func registeredPlanDescriptor(kind models.ResourceKind, value any, providerID string, metadata *models.ResourceMeta) (providercontract.PlanDescriptor, error) {
	descriptor := providercontract.PlanDescriptor{
		Effects:           []providercontract.PlanEffect{{Code: providercontract.EffectResourceUpdate}},
		RollbackClass:     providercontract.RollbackNone,
		ActivationTargets: notificationActivationTargets(metadata),
		BaselineEligible:  kind != models.ResourceKindReboot && kind != models.ResourceKindCommand,
	}
	if protectedRollbackKind(kind) {
		descriptor.RollbackClass = providercontract.RollbackTransactional
	}
	switch kind {
	case models.ResourceKindSudo:
		descriptor.Effects[0].Code = providercontract.EffectSudoPolicyReplace
	case models.ResourceKindBrowserPolicy:
		descriptor.RollbackClass = providercontract.RollbackTransactional
		if resource, ok := value.(*models.BrowserPolicyResource); ok {
			descriptor.ActivationTargets = []providercontract.ActivationTarget{{
				Kind: providercontract.ActivationApplicationRestart, Target: string(resource.Browser),
			}}
		}
	case models.ResourceKindNetworkProfile:
		if resource, ok := value.(*models.NetworkProfileResource); ok && !resource.IsAudit() {
			descriptor.RollbackClass = providercontract.RollbackTransactional
		}
	case models.ResourceKindFirewall:
		descriptor.Effects[0].Code = providercontract.EffectFirewallPolicyReplace
		if resource, ok := value.(*models.FirewallResource); ok && !resource.IsAudit() && providerID == "nftables" {
			descriptor.RollbackClass = providercontract.RollbackTransactional
		}
	case models.ResourceKindDNSResolver:
		descriptor.Effects[0].Code = providercontract.EffectNetworkDNSReplace
	case models.ResourceKindRoute:
		if resource, ok := value.(*models.RouteResource); ok &&
			(resource.Destination == "default" || resource.Destination == "0.0.0.0/0" || resource.Destination == "::/0") {
			descriptor.Effects[0].Code = providercontract.EffectDefaultRouteReplace
		}
	case models.ResourceKindSysctl:
		if resource, ok := value.(*models.SysctlResource); ok && resource.Activation == models.SysctlNextBoot {
			descriptor.ActivationTargets = append(descriptor.ActivationTargets, providercontract.ActivationTarget{Kind: providercontract.ActivationNextBoot})
		}
	case models.ResourceKindTimeSync:
		if resource, ok := value.(*models.TimeSyncResource); ok && (len(resource.Servers) > 0 || len(resource.Pools) > 0) {
			descriptor.ActivationTargets = append(descriptor.ActivationTargets, providercontract.ActivationTarget{
				Kind: providercontract.ActivationRestart, Target: "systemd-timesyncd.service",
			})
		}
	case models.ResourceKindSessionPolicy:
		descriptor.ActivationTargets = append(descriptor.ActivationTargets, providercontract.ActivationTarget{Kind: providercontract.ActivationLogoutRequired})
	case models.ResourceKindSystemdUnit:
		descriptor.ActivationTargets = append([]providercontract.ActivationTarget{{Kind: providercontract.ActivationDaemonReload}}, descriptor.ActivationTargets...)
	case models.ResourceKindAccountLimit:
		descriptor.ActivationTargets = append(descriptor.ActivationTargets, providercontract.ActivationTarget{Kind: providercontract.ActivationLogoutRequired})
	case models.ResourceKindJournald:
		descriptor.ActivationTargets = append(descriptor.ActivationTargets, providercontract.ActivationTarget{
			Kind: providercontract.ActivationRestart, Target: "systemd-journald.service",
		})
	case models.ResourceKindDownload:
		if resource, ok := value.(*models.DownloadResource); ok {
			switch {
			case len(resource.ReloadExec) > 0:
				activation, representable := downloadActivationTarget(resource.ReloadExec)
				if !representable {
					return providercontract.PlanDescriptor{}, fmt.Errorf("download %q has unrepresentable reloadExec plan evidence", resource.Name)
				}
				descriptor.ActivationTargets = append(descriptor.ActivationTargets, activation)
			case strings.TrimSpace(resource.NotifySystemd) != "":
				descriptor.ActivationTargets = append(descriptor.ActivationTargets, providercontract.ActivationTarget{
					Kind: providercontract.ActivationTryRestart, Target: resource.NotifySystemd,
				})
			}
		}
	case models.ResourceKindTrustAnchor:
		descriptor.ActivationTargets = append([]providercontract.ActivationTarget{{
			Kind: providercontract.ActivationTrustStoreRefresh, Target: providerID,
		}}, descriptor.ActivationTargets...)
	}
	return descriptor, nil
}

func downloadActivationTarget(argv []string) (providercontract.ActivationTarget, bool) {
	if len(argv) == 2 && argv[0] == "systemctl" && argv[1] == "daemon-reload" {
		return providercontract.ActivationTarget{Kind: providercontract.ActivationDaemonReload}, true
	}
	if len(argv) != 3 || argv[0] != "systemctl" {
		return providercontract.ActivationTarget{}, false
	}
	activation := providercontract.ActivationTarget{Target: argv[2]}
	switch argv[1] {
	case "reload":
		activation.Kind = providercontract.ActivationReload
	case "restart":
		activation.Kind = providercontract.ActivationRestart
	case "try-restart":
		activation.Kind = providercontract.ActivationTryRestart
	default:
		return providercontract.ActivationTarget{}, false
	}
	return activation, true
}

func protectedRollbackKind(kind models.ResourceKind) bool {
	switch kind {
	case models.ResourceKindAPTSigningKey, models.ResourceKindAPTRepository, models.ResourceKindFile,
		models.ResourceKindAuthorizedKey, models.ResourceKindKnownHost, models.ResourceKindSudo,
		models.ResourceKindDownload, models.ResourceKindBrowserPolicy, models.ResourceKindHostsEntry,
		models.ResourceKindCertificate, models.ResourceKindTrustAnchor, models.ResourceKindAccountLimit,
		models.ResourceKindLoginPolicy, models.ResourceKindJournald, models.ResourceKindLogrotate:
		return true
	default:
		return false
	}
}

func notificationActivationTargets(metadata *models.ResourceMeta) []providercontract.ActivationTarget {
	if metadata == nil {
		return nil
	}
	targets := make([]providercontract.ActivationTarget, 0, len(metadata.Notifications))
	for _, notification := range metadata.Notifications {
		activation := providercontract.ActivationTarget{Target: notification.Target}
		switch notification.Type {
		case models.NotificationDaemonReload:
			activation.Kind = providercontract.ActivationDaemonReload
		case models.NotificationReload:
			activation.Kind = providercontract.ActivationReload
		case models.NotificationTryRestart:
			activation.Kind = providercontract.ActivationTryRestart
		case models.NotificationRestart:
			activation.Kind = providercontract.ActivationRestart
		case models.NotificationLogout:
			activation.Kind = providercontract.ActivationLogoutRequired
		case models.NotificationNextBoot:
			activation.Kind = providercontract.ActivationNextBoot
		case models.NotificationReboot:
			activation.Kind = providercontract.ActivationRebootRequired
		}
		targets = append(targets, activation)
	}
	return targets
}
