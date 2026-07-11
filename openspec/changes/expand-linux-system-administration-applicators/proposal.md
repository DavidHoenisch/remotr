## Why

Remotr accepts several Linux desired-state fields that its agent does not fully check or enforce, and common endpoint-administration tasks still require generic commands or unmanaged file edits. The applicator roadmap needs an explicit, testable contract so contributors can deliver portable Linux management in a safe order without overstating convergence or expanding into an unbounded configuration-management catalog.

## What Changes

- Establish a common applicator contract for structured observed/desired state, unsupported-provider results, redaction, activation outcomes, rollback capability, audit/report behavior, validation, locking, and stable resource addresses.
- Complete convergence for current package, filesystem, user, systemd, download, and firewall resources before adding broad new surface area.
- Introduce provider-backed resources for repositories and signing keys, filesystem objects, groups and local accounts, SSH access, sudo policy, sysctl, kernel modules, and host identity.
- Add durable host-lifecycle resources for mounts and swap, endpoint-local schedules, richer services, controlled reboot, and guarded network/DNS/route management.
- Add security and workstation resources for SELinux/AppArmor/audit policy, certificates and secret material, authentication policy, resource limits, logging, and interactive-user desktop policy.
- Define optional post-foundation capabilities for archive/VCS deployment, storage provisioning, container workloads, alternatives, capabilities, and transient-path policy; these remain demand-gated milestones rather than prerequisites for the core roadmap.
- Extend repository validation, composition, agent resolution/ordering, drift telemetry, state reports, documentation, and provider test matrices for every introduced resource.
- Preserve server-dispatched cron jobs as a distinct capability from persistent endpoint schedules.
- **BREAKING**: reject desired-state fields and provider selections that the active agent cannot honor instead of silently accepting them or reporting perpetual drift.
- **BREAKING**: replace ambiguous package and filesystem behavior with explicit lifecycle and ownership semantics; compatibility parsing may be provided for existing YAML, but canonical authored configuration follows the new contracts.

## Capabilities

### New Capabilities

- `applicator-execution-contract`: Common check/apply/report, validation, safety, redaction, activation, rollback, locking, and provider-result semantics for all applicators.
- `package-and-repository-management`: Convergent package transactions, versions, lifecycle policy, provider selection, repositories, signing keys, and supported-distribution behavior.
- `filesystem-object-management`: Files, directories, links, ownership, modes, absence, atomic replacement, structured edits, metadata, and archive/source-tree deployment.
- `local-identity-and-access`: Groups, complete local-user lifecycle, group membership, SSH authorization/known hosts, sudo policy, authentication policy, and account limits.
- `kernel-and-host-baseline`: Sysctl, kernel modules, hostname, timezone, locale/keymap, and time-synchronization state.
- `mount-and-storage-management`: Mounts, swap, and demand-gated destructive storage provisioning with explicit safety controls.
- `endpoint-schedule-management`: Persistent endpoint cron entries and systemd timers, separate from server-dispatched scheduled work.
- `service-and-reboot-management`: Provider-neutral services, richer systemd unit/drop-in behavior, activation notifications, reboot-required reporting, and coordinated reboot.
- `network-and-firewall-management`: Firewall lifecycle/ownership plus guarded hosts, DNS, route, and network-profile management.
- `linux-security-and-secret-management`: Mandatory-access-control and audit policy, certificates, trust stores, secret material, logging policy, and confidential reporting.
- `interactive-user-policy`: Provider-aware desktop/session/browser policy applied safely to interactive users whether logged in or not.
- `optional-workload-management`: Demand-gated containers, alternatives, Linux capabilities, environment policy, and related host workload primitives.

### Modified Capabilities

None. This repository does not yet contain main OpenSpec capability specs; the change captures both corrected existing behavior and new behavior as the initial specification baseline.

## Impact

- Desired-state schema and validation in `internal/models`, `internal/configrepo`, configuration composition, and the published configuration reference.
- Agent facts, resolution, dependency ordering, execution, provider selection, applicators, rollback storage, and local operation locks.
- Sync telemetry, persisted state reports, operator CLI output, and any server API schemas that expose structured drift, activation, unsupported, deferred, or failure results.
- Linux provider implementations and integration environments for Debian/Ubuntu and Arch first, with Fedora/RHEL enabled only when facts and DNF behavior are complete; later providers are capability-gated.
- Security-sensitive local state including account databases, privilege policy, firewall/network paths, boot configuration, storage, and secrets.
- Backward compatibility for existing composed YAML and migration documentation for canonical resource forms.
