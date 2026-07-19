# Linux endpoint applicator gap analysis

- **Research date:** 2026-07-10
- **Repository baseline:** `5107514`
- **Question:** Which common Linux endpoint administration tasks should Remotr support next as first-class, convergent applicators?

## Executive summary

Remotr already has the outline of a useful Linux desired-state engine: package, file, download, user, systemd, per-user systemd, firewall, bootstrap, agent-install, command, and server-scheduled cron resources. The highest-value next step is not to copy Ansible's entire module catalog. It is to make a smaller set of endpoint primitives complete, portable, and honestly convergent.

The recommended order is:

1. **Close correctness gaps in existing resources**: enforce package versions, file metadata, requested UIDs, package-manager behavior, and firewall removal.
2. **Add foundational primitives**: groups and full local accounts, directories/links/ownership, repositories and signing keys, SSH authorization, sudo policy, and sysctl/kernel modules.
3. **Add host lifecycle primitives**: hostname/time/locale, mounts, persistent schedules, network/DNS, service backends, and controlled reboot.
4. **Add security and fleet-depth primitives**: SELinux/AppArmor, certificates and secrets, desktop policy, storage, containers, and structured archive/VCS deployment.

This ordering is a product judgment based on Remotr's pull-based MDM model, safety needs, and current implementation. The external inventories below are evidence that these are established configuration-management resource categories, not a claim that every listed task is equally common in every fleet.

## Delivery update — 2026-07-15

The analysis below remains the historical `5107514` baseline. The umbrella
OpenSpec implementation has since delivered the M1–M5 schema and applicator
surface, while the release-evidence audit found that provider advertisement
and real composed-repository coverage still lag implementation. Use the
[M1–M5 exit-criteria and gap report](../testing/applicator-m1-m5-gap-report.md)
for the current release decision.

### Delivered capability links

| Milestone | Delivered authoring and operating surface |
| --- | --- |
| **M1: truthful convergence** | [Packages](../reference/configuration-format.md#package-resources), [files/directories/links](../reference/configuration-format.md#file-resources), canonical lifecycle and provider validation, plus [guarded firewall ownership](../reference/configuration-format.md#firewall-resources) |
| **M2: access baseline** | [Expanded users, groups, authorized keys, known hosts, and sudo fragments](../reference/configuration-format.md#m2-local-access-resources), with the [local administrator workflow](../guides/local-administrator-access.md) |
| **M3: OS baseline** | [APT repositories and signing keys](../reference/configuration-format.md#package-resources), sysctl/kernel modules, hostname/locale/time, [mounts](../reference/configuration-format.md#mount-resources), and [swap](../reference/configuration-format.md#swap-resources) |
| **M4: durable operations** | [Endpoint cron/systemd-timer schedules](../reference/configuration-format.md#endpoint-schedule-resources), [provider-neutral services and coordinated reboot](../reference/configuration-format.md#service-resources), [systemd units](../reference/configuration-format.md#systemd-unit-resources), hosts/DNS/routes, and guarded [network profiles](../reference/configuration-format.md#network-profile-providers) |
| **M5: security and workstation policy** | [Typed desktop settings, session policy, and browser policy](../reference/configuration-format.md#typed-desktop-settings), plus certificates, trust anchors, AppArmor, audit rules, account limits, login policy, journald, and logrotate |

The [configuration validation workflow](../guides/config-validation.md) exposes
the canonical resource kinds and provider requirements before a release
advances. Full unit, contract, provider-process, VM-safety, migration,
mixed-version, end-to-end, and documentation validation passed on 2026-07-15.

### Measured demand and evidence

No production fleet-usage telemetry, administrator survey, or linked customer
request corpus is available in this repository. The roadmap therefore cannot
claim measured production frequency for any resource kind. The measurable
signals at this update are deliberately narrower:

- The two checked-in kind-tagged repositories both validate and render. Across
  their rendered desired state they contain four package resource instances;
  the Compose repository also references one server-dispatched cron. Neither
  repository currently expresses an M2–M5 resource.
- The provider matrix has 3 passing Debian 12 VM rows and 15 untested
  Debian/Ubuntu/Arch container rows. Passing container discovery does not
  promote an untested row to a support claim.
- At the 14.7 audit baseline, the umbrella traceability inventory had 123
  verified and 104 planned scenarios. Many planned selectors depended on the
  features now existing, but they still need deliberate public-seam assignment
  before release.
- No checked-in demand record meets the M6 graduation requirement for
  archives/VCS, destructive storage, containers, alternatives, Linux file
  capabilities, environment fragments, or transient paths.

This is evidence of current repository usage and validation depth, not a proxy
for production fleet demand. Until privacy-preserving fleet telemetry or a
reviewed request corpus exists, optional breadth remains demand-gated.

### Remaining gaps and reprioritization

Ubuntu security-control capabilities that cannot yet become safe Hub snippets
are tracked in the
[Ubuntu security-control capability roadmap](ubuntu-cmmc-capability-roadmap.md).

The roadmap priority is now evidence-first rather than catalog-first:

1. **P0 — close release evidence for M1–M5.** Add safe representative schema-1
   modules to a checked-in composed repository, assign the remaining
   traceability selectors, and promote provider rows only after their real
   contract suites pass.
2. **P1 — deepen the advertised provider matrix.** Finish Debian/Ubuntu/Arch
   package, filesystem, identity, service, repository, network, security,
   desktop, and logging rows before expanding to another distribution family.
3. **P2 — open demand-backed provider changes.** Keep RPM/DNF, SELinux,
   authselect, OpenRC/SysV, transactional firewalld, Chrony, and file-backed
   network credentials unadvertised until focused child changes carry their
   provider and safety evidence.
4. **P3 — retain M6 as optional breadth.** Archive/VCS deployment, destructive
   storage, containers, alternatives, Linux file capabilities, environment
   fragments, and transient paths remain deferred until a concrete fleet use
   case, security review, and maintenance owner are recorded.

The detailed unsupported field/provider inventory and milestone decisions are
maintained in the [current gap report](../testing/applicator-m1-m5-gap-report.md).

## Method and evidence

I compared Remotr's parsed model, engine wiring, and applicator implementations with primary-source resource inventories from mature configuration-management projects:

- [Ansible built-in modules](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html) include package/repository, file/template, cron, group/user, hostname, service, reboot, archive, and other host-management primitives.
- [Ansible POSIX modules](https://docs.ansible.com/projects/ansible/latest/collections/ansible/posix/index.html) add ACL, authorized keys, mounts, SELinux, and sysctl.
- [Salt state modules](https://docs.saltproject.io/en/latest/ref/states/all/index.html) independently cover groups, hosts, mounts, interfaces, NTP, repositories, SSH authorization and known hosts, SELinux, sysctl/sysfs, TLS, timezone, and user management.
- [Puppet's resource type reference](https://help.puppet.com/core/current/Content/PuppetCore/Markdown/type.htm) exposes detailed convergent resources for files, groups, users, packages, services, and adjacent types such as mounts, SSH keys, SELinux, and repositories.
- [Chef Infra's bundled resource catalog](https://docs.chef.io/client/19/resources/bundled/) independently includes directories, links, repositories, hostname/locale/timezone, kernel modules, mounts, reboot, routes, SELinux, sudo, sysctl, certificates, users and limits, and multiple Linux package managers.

The convergence of these independent catalogs is used as a demand signal. Linux and subsystem project documentation is used where the shape or safety of a resource matters. For example, Linux exposes runtime kernel settings through `/proc/sys` ([Linux kernel sysctl documentation](https://docs.kernel.org/admin-guide/sysctl/index.html)), and OpenSSH treats authorized keys as structured entries with per-key restrictions rather than an undifferentiated file blob ([OpenSSH `sshd(8)`, Authorized Keys File Format](https://man.openbsd.org/sshd.8#AUTHORIZED_KEYS_FILE_FORMAT)).

This document focuses on local Linux endpoint state. Cloud APIs, database schemas, application-specific orchestration, network-device automation, and Windows/macOS resources are intentionally out of scope.

### Limitations

- This is a primary-source capability comparison and static repository review, not a survey of Linux administrators or analysis of Remotr fleet telemetry. The priority order is therefore a reasoned product recommendation, not a measured frequency ranking.
- Mature-tool catalogs demonstrate stable categories and useful resource shapes, but their breadth includes niche and legacy modules. Catalog presence alone is not a reason to implement a Remotr applicator.
- The repository review distinguishes parsed schema from active Check/Apply behavior, but it did not exercise every applicator against live distributions.
- Package names, service names, configuration paths, and supported semantics differ across distributions. Each proposed portable contract still needs provider-specific validation and integration tests.

## Status vocabulary

- **Existing**: represented in desired state and checked/applied as a first-class resource.
- **Partial**: represented or advertised, but important desired fields, lifecycle operations, safety, or platform support are missing.
- **Missing**: only achievable through generic files/downloads/commands or not supported at all; there is no domain-specific check/apply contract.

Generic `commands` are a valuable escape hatch, but do not make a task first-class: authors must supply their own check, portability, idempotency, validation, rollback, and reporting.

## Current Remotr surface: advertised versus enforced

The public schema is summarized in the [configuration format reference](../reference/configuration-format.md), while the baseline resource model and engine wiring live in [`internal/models/models.go`](https://github.com/DavidHoenisch/remotr/blob/5107514/internal/models/models.go) and [`internal/agent/engine/engine.go`](https://github.com/DavidHoenisch/remotr/blob/5107514/internal/agent/engine/engine.go).

| Area | Status | What is actually enforced | Important gaps |
|---|---|---|---|
| Packages | **Partial** | apt and pacman install/remove by name; Flatpak, PWA, and Remotr catalog packages have dedicated handlers | apt/pacman checks only presence and ignore `version`; `yay` selects the pacman handler; DNF `Apply` always errors; no repository/key, hold/pin, cache/update, upgrade-policy, APK, Zypper, Snap, or rpm-ostree lifecycle |
| Distro support | **Partial** | Debian, Ubuntu, and Arch facts and targeting | `ReadDistro` recognizes only those families; Fedora/RHEL cannot reach the DNF stub; SUSE, Alpine, immutable/image-based variants, and non-systemd Linux are not modeled |
| Files | **Partial** | Whole-file content and line-regex edits; parent directories are created; optional mode is used on a content write; per-interactive-user files are chowned safely | `State` does not check mode; system files cannot declare owner/group; no absent state, standalone directory, symlink/hardlink, ACL, xattr/capability, recursive tree, template, managed block, or SELinux label |
| Downloads | **Existing, narrow** | HTTP(S) download to a path, optional SHA-256, mode, and post-apply unit/command notification | No archive extraction, signatures, authentication, directory sync, or artifact retention policy |
| Users | **Partial** | User presence/absence by username | Parsed `uid` is ignored by the active handler; no groups, primary/supplementary membership, GID, home, shell, gecos, system account, password/hash, lock/expiry, SSH keys, or account limits |
| systemd services | **Existing, systemd-only** | enabled/disabled, active/stopped, masked/unmasked for system and interactive-user units; optional lingering | No restart/reload state, drop-in/unit content resource, timer abstraction, or SysV/OpenRC backend |
| Firewall | **Partial** | Audited or enforced allow/deny/drop/reject rules for firewalld or nftables, with sync-path protection | No explicit present/absent lifecycle, authoritative rule-set ownership, policy/zone lifecycle, rollback timeout, or iptables/UFW backend |
| Scheduled work | **Partial, different semantic** | Server evaluates schedules and dispatches due work on the next agent check-in; execution state is reported | This does not create persistent cron entries or systemd timers on the endpoint, so it cannot model schedules that must run while the endpoint cannot reach Remotr |
| Bootstrap | **Existing, narrow** | Ordered exec/systemd steps gated by path existence | Conditions are path-only; no structured transaction, rich probe, or reusable handler semantics |
| Commands | **Existing escape hatch** | Explicit argv `check`, `apply`, and optional `revert` | Portability, idempotency, secrets handling, and structured drift/reporting remain author responsibilities |
| Agent/application install | **Existing, specialized** | Remotr catalog packages and a versioned fleet-agent tarball workflow | Not a general archive, installer, repository, certificate, or daemon resource |

The most consequential distinction is that a parsed field is not necessarily part of desired-state checking. Today `Package.Version`, `UserResource.UID`, and `File.Mode` can appear in YAML without being fully enforced as ongoing state.

## Deepen, add, or compose?

Not every gap should become a new resource kind. Use three delivery modes:

| Delivery mode | Use when | Candidates from this analysis |
|---|---|---|
| **Deepen an existing applicator** | The domain object already exists, but its declared fields, lifecycle, portability, or drift checking are incomplete | Package versions/backends, file metadata and absent state, full user attributes, systemd reload/restart, firewall removal/ownership, download authentication/signatures |
| **Add a new primitive applicator** | The task has distinct observed state, lifecycle, safety rules, or multiple providers that generic files/commands cannot report honestly | Groups, repositories/signing keys, authorized keys, sudo policy, sysctl, kernel modules, mounts, endpoint schedules, network profiles, reboot, SELinux/AppArmor, certificates/secrets |
| **Publish a composable module** | The outcome is a named policy or application assembled from existing primitives, not a new OS object | SSH hardening, auditd baseline, NTP daemon setup, developer workstation baseline, browser policy, CIS-style settings, log forwarding, application deployment |

The test is simple: build an applicator when Remotr must understand and compare the object's state; build a module when Remotr can safely compose already-correct primitives. For example, “an authorized SSH key is present with these restrictions” warrants a primitive, while “our company SSH baseline” should compose packages, files, authorized keys, firewall, and services.

## Prioritized applicator gaps

### P0 — complete the foundation

P0 items should precede broad catalog expansion because they affect trust in drift reports and underpin many later resources.

#### 1. Package transaction and repository applicators

**Current status: partial.** Split package management into a generic package contract and backend-specific providers. Make version constraints, install/remove/purge, upgrade/downgrade, hold/pin, and check behavior explicit. Finish DNF only after Fedora/RHEL fact support is real; route `yay` to a genuine AUR-aware provider or remove the advertised distinction. Add repository and trust-key resources for APT and DNF first, then Pacman, Zypper, APK, Snap, and image-based systems according to supported-distro strategy.

This is established configuration-management surface: Ansible has separate package, deb822 repository, RPM key, YUM repository, apt, DNF, and DNF5 modules ([Ansible built-ins](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html)); Salt exposes repositories across APT/DNF/YUM/Zypper ([Salt `pkgrepo`](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.pkgrepo.html)); and Chef separates package, repository, preference, update, Snap, Pacman, Zypper, and RPM-family resources ([Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

**Suggested first slice:** exact version convergence plus APT repository/key support on Debian/Ubuntu, with accurate drift reporting and redacted credentials.

#### 2. Filesystem object applicator family

**Current status: partial.** Evolve `files` into explicit `file`, `directory`, and `link` object kinds with `present/absent`, owner, group, mode, and atomic replacement. Ensure metadata-only drift is repaired even when content is already correct. Follow with recursive directory policy, ACLs, extended attributes/Linux capabilities, SELinux context, and managed block/template support.

Puppet's file resource manages files, directories, symlinks, ownership, permissions, recursion, and SELinux attributes ([Puppet file type](https://help.puppet.com/core/current/Content/PuppetCore/Markdown/type.htm#file)); Ansible separates file properties, templates, line editing, block editing, and assembly ([Ansible built-ins](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html)); and Ansible POSIX provides an ACL resource ([Ansible POSIX](https://docs.ansible.com/projects/ansible/latest/collections/ansible/posix/index.html)).

**Suggested first slice:** owner/group/mode checking, `state: absent|file|directory|symlink`, and atomic writes without changing the current line-edit semantics.

#### 3. Local identity, groups, SSH access, and sudo policy

**Current status: partial/missing.** Add a group resource, then deepen users with UID/GID, primary and supplementary groups, home creation/removal policy, shell, comment, system-account flag, password/hash/lock, expiry, and account limits. Add structured SSH `authorized_key` and `known_host` resources and a validated sudoers fragment resource.

Group membership and account attributes are first-class in Puppet rather than file edits ([Puppet group type](https://help.puppet.com/core/current/Content/PuppetCore/Markdown/type.htm#group), [Puppet user type](https://help.puppet.com/core/current/Content/PuppetCore/Markdown/type.htm#user)). Ansible POSIX and Salt both expose dedicated authorized-key resources ([Ansible POSIX](https://docs.ansible.com/projects/ansible/latest/collections/ansible/posix/index.html), [Salt `ssh_auth`](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.ssh_auth.html)). This matters because an OpenSSH authorized-key entry can carry restrictions such as forced commands, source restrictions, forwarding controls, expiry, and CA principals ([OpenSSH authorized-key format](https://man.openbsd.org/sshd.8#AUTHORIZED_KEYS_FILE_FORMAT)). Chef also treats sudo and user limits as distinct resources ([Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

**Suggested first slice:** group present/absent and user primary/supplementary membership, while fixing UID enforcement; then authorized keys with append-versus-authoritative-set semantics.

#### 4. Kernel parameters and kernel modules

**Current status: missing.** Add `sysctl` with runtime and persistent state, a target drop-in path, reload behavior, and a clear policy for unsupported keys. Add `kernelModule` with loaded/unloaded and boot persistence, plus optional module parameters and blacklist state.

The kernel defines sysctl as runtime configuration through `/proc/sys`, spanning filesystem, kernel, networking, user namespace, and VM settings ([Linux kernel sysctl documentation](https://docs.kernel.org/admin-guide/sysctl/index.html)). Ansible POSIX, Salt, and Chef all expose sysctl resources, while Chef also has a kernel-module resource ([Ansible POSIX](https://docs.ansible.com/projects/ansible/latest/collections/ansible/posix/index.html), [Salt state modules](https://docs.saltproject.io/en/latest/ref/states/all/index.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

**Suggested first slice:** one key/value with `runtime`, `persistent`, and `reload` controls, writing a Remotr-owned drop-in rather than editing the distribution's main file.

#### 5. Safe state transitions and validation contract

**Current status: partial cross-cutting behavior.** Standardize check/apply/revert semantics before adding risky resources. Each applicator should report actual and desired state structurally, distinguish unsupported from drift, validate before activation, and state whether rollback is real, best-effort, or unavailable. Add a common notification mechanism (`reload`, `restart`, `daemon-reload`, reboot-required) rather than resource-specific fields.

Chef exposes common notification, guard, retry, timeout, and sensitive-data behavior across resources ([Chef common resource functionality](https://docs.chef.io/resources/#common-functionality)). This is a useful model for a uniform Remotr contract even though Remotr's pull/check/apply lifecycle is different.

**Suggested first slice:** an applicator capability/result contract plus redaction, dry-run/audit support, and consistent post-apply notification.

### P1 — common host lifecycle management

#### 6. Host identity, time, and locale

**Current status: missing.** Add hostname, timezone, locale/keymap, and time-synchronization resources. Keep timezone selection separate from NTP server/provider configuration; different distributions use different daemons even when systemd tools are present.

Hostname is a built-in Ansible resource; Salt provides timezone and NTP states; Chef provides hostname, locale, and timezone resources ([Ansible built-ins](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html), [Salt state modules](https://docs.saltproject.io/en/latest/ref/states/all/index.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

#### 7. Mounts, swap, and persistent filesystem declarations

**Current status: missing.** Add mount resources that separately express persistent configuration and current mounted state, including source, target, filesystem type, options, dump/pass fields, and absent/unmounted semantics. Add swap-file/swap-device lifecycle before attempting partitions or LVM.

Ansible POSIX explicitly controls active and configured mount points; Salt has a filesystem mount state; Chef provides mount and swap-file resources ([Ansible POSIX](https://docs.ansible.com/projects/ansible/latest/collections/ansible/posix/index.html), [Salt `mount`](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.mount.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

#### 8. Persistent endpoint schedules

**Current status: partial.** Preserve server-dispatched crons for centrally observed work, but add a separate endpoint schedule resource for crontab/`cron.d` entries and systemd timers. The resource should own entries by stable marker/name, manage environment and user, validate schedules, and choose cron versus systemd timer explicitly.

Ansible has a dedicated cron resource, Salt manages its scheduler, and Chef includes cron, `cron_d`, and systemd-timer resources ([Ansible built-ins](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html), [Salt `schedule`](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.schedule.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

#### 9. Network, DNS, routes, and hosts entries

**Current status: missing.** Add small, backend-aware resources rather than a single universal network blob: hostname/hosts entries, DNS resolver/search domains, routes, and interface/profile configuration. Start with audit/report mode and NetworkManager on desktop/server distributions; preserve the Remotr control path with the same or stronger protections used by firewall changes. Support systemd-networkd/netplan only after the model is provider-neutral.

Salt separately exposes hosts, network-interface, NTP, and network configuration states, while Chef exposes interface and route resources ([Salt state modules](https://docs.saltproject.io/en/latest/ref/states/all/index.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)). The separation is a good signal that DNS, routes, and interface activation have different state and safety needs.

#### 10. Service provider abstraction and richer systemd units

**Current status: partial.** Add restart/reload operations, provider-discovered service facts, and first-class unit/drop-in content with validation and daemon reload. If cross-platform Linux includes Alpine or minimal systems, define a provider interface for OpenRC and SysV rather than overloading the systemd resource.

Ansible exposes a generic service resource, a systemd-specific resource, and a SysV resource ([Ansible built-ins](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html)). Salt and Chef likewise expose generic service resources ([Salt `service`](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.service.html), [Chef service resource](https://docs.chef.io/resources/service/)).

#### 11. Controlled reboot and maintenance coordination

**Current status: missing.** Add a reboot applicator/job with precondition, delay/deadline, maintenance-window policy, check-in acknowledgement, boot-ID verification, and timeout. Package and kernel changes should be able to report `rebootRequired` without immediately rebooting.

Ansible and Chef both provide dedicated reboot resources rather than treating reboot as a normal shell command ([Ansible reboot module](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/reboot_module.html), [Chef reboot resource](https://docs.chef.io/resources/reboot/)). Remotr additionally needs asynchronous acknowledgement because its agent disappears during the operation.

#### 12. Mandatory access control and audit policy

**Current status: missing.** Add SELinux state, booleans, file contexts, ports, and policy modules for Fedora/RHEL support; add AppArmor profile install/enforce/complain/disable for Ubuntu-family fleets. Treat auditd rule deployment/loading as its own validated resource rather than only a file plus reload command.

Ansible POSIX exposes SELinux state and booleans; Salt has an SELinux state; Chef has separate SELinux state, boolean, context, port, user, login, and module resources ([Ansible POSIX](https://docs.ansible.com/projects/ansible/latest/collections/ansible/posix/index.html), [Salt `selinux`](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.selinux.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

#### 13. Certificates, trust stores, and secret material

**Current status: partial infrastructure, missing desired-state resource.** Remotr already uses secrets for its own enrollment and can download files, but it lacks a general resource that safely installs private material. Add CA trust, certificate/key pair, permissions, expiry reporting, renewal hooks, and provider-backed secret retrieval. Secret values must be omitted from Git artifacts, diffs, logs, drift payloads, and backups.

Salt exposes TLS state, Chef includes trusted-certificate and X.509/key resources, and Chef's common resource behavior includes sensitive-data handling ([Salt `tls`](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.tls.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/), [Chef common functionality](https://docs.chef.io/resources/#common-functionality)).

#### 14. Desktop and interactive-user policy

**Current status: partial.** `userFiles`, PWA launchers, and systemd-user units are a useful base. Add provider-aware desktop settings, starting with dconf/GSettings, login/session policy, screensaver/lock, proxy, certificate/browser policy files, and default applications. Preserve per-user ownership and handle users who are not logged in.

Ansible's community catalog includes a dconf resource, and Salt exposes desktop-specific settings modules alongside per-user state ([Ansible `community.general.dconf`](https://docs.ansible.com/projects/ansible/latest/collections/community/general/dconf_module.html), [Salt state modules](https://docs.saltproject.io/en/latest/ref/states/all/index.html)). This category is especially relevant to Remotr's MDM positioning even though it is less central to server-only configuration managers.

#### 15. Authentication policy and resource limits

**Current status: missing.** Add PAM and login-policy support only through distro-aware providers, with syntax/preflight validation and recovery controls. Add user/service limit resources for file descriptors, processes, memory locking, and related `limits.d` settings. Do not treat PAM stacks as generic line-edit targets: Ubuntu/Debian, Fedora/RHEL with authselect, and other families have different ownership and regeneration rules.

The mature ecosystem models these separately: Ansible community general provides dedicated [PAM rule](https://docs.ansible.com/projects/ansible/latest/collections/community/general/pamd_module.html) and [PAM limits](https://docs.ansible.com/projects/ansible/latest/collections/community/general/pam_limits_module.html) modules, while Chef includes a [user limits resource](https://docs.chef.io/resources/user_ulimit/).

#### 16. Logging, rotation, and forwarding

**Current status: missing.** Add journald storage/retention/rate-limit/forwarding policy and logrotate resources, or first publish carefully validated modules if file/service primitives are sufficient. A first-class logging resource becomes worthwhile when Remotr must report effective retention, disk use limits, or forwarding state across providers.

Systemd documents persistent/volatile journal storage, retention and disk limits, rate limiting, and forwarding as distinct settings ([`journald.conf`](https://www.freedesktop.org/software/systemd/man/252/journald.conf.html)); Salt separately exposes a [logrotate state](https://docs.saltproject.io/en/latest/ref/states/all/salt.states.logrotate.html).

### P2 — valuable breadth after the core is trustworthy

#### 17. Archive and source-tree deployment

**Current status: missing/generalized only through commands.** Add archive extract/create with checksum, strip-components, ownership, atomic destination swap, and removal policy. Consider a separate Git checkout resource with revision pinning and clean/dirty policy; do not hide it inside downloads.

Ansible has Git and unarchive modules, and Chef has archive-file, Git, remote-directory, and remote-file resources ([Ansible built-ins](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

#### 18. Storage provisioning

**Current status: missing.** After mounts and swap are stable, consider filesystem formatting, partitions, LVM physical/volume/logical volumes, software RAID, encryption/crypttab, and quotas. These need explicit destructive-change policy, inventory preconditions, audit-first behavior, and normally human approval.

Salt exposes mdadm RAID and mount states; Ansible's broader collections include LVM and filesystem modules; Chef includes mdadm, mount, and swap resources ([Salt state modules](https://docs.saltproject.io/en/latest/ref/states/all/index.html), [Ansible community general modules](https://docs.ansible.com/projects/ansible/latest/collections/community/general/index.html), [Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/)).

#### 19. Container workload and image state

**Current status: missing.** Add only if endpoint use cases demand it: container image presence, container/service desired state, registry authentication, networks, volumes, and auto-update policy. Prefer a Podman/systemd-provider boundary over a generic shell wrapper.

Chef includes a container resource and Ansible maintains a dedicated containers collection, demonstrating that container state is usually separated from base OS package/service state ([Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/), [Ansible collections index](https://docs.ansible.com/projects/ansible/latest/collections/index.html)).

#### 20. Alternatives, capabilities, ulimits, and miscellaneous host policy

**Current status: missing.** Smaller but useful resources include alternatives/default program selection, Linux file capabilities, per-user/service limits, environment variables, `/etc/hosts`, and tmpfiles-style transient paths. Implement these after file, identity, and service primitives because many can share their provider and validation infrastructure.

Chef's bundled catalog includes alternatives, kernel modules, links, locale, sudo, sysctl, user limits, and related resources; Ansible community general includes Linux capabilities and alternatives ([Chef bundled resources](https://docs.chef.io/client/19/resources/bundled/), [Ansible community general](https://docs.ansible.com/projects/ansible/latest/collections/community/general/index.html)).

## Recommended delivery sequence

| Milestone | Scope | Exit criterion |
|---|---|---|
| **M1: truthful convergence** | Package version enforcement; file owner/group/mode drift; UID enforcement; DNF/facts mismatch resolved; firewall present/absent | Every advertised state field is either enforced, rejected during validation, or documented as informational |
| **M2: access baseline** | Groups, expanded users, directories/links, authorized keys, sudo fragments | A fleet can provision and revoke a local administrator without generic commands or hand-edited passwd/SSH/sudo files |
| **M3: OS baseline** | Repositories/keys, sysctl, kernel modules, hostname/time/locale, mounts/swap | A general Debian/Ubuntu or Fedora/RHEL operating-system baseline can be expressed with first-class resources |
| **M4: durable operations** | Endpoint schedules, richer services, reboot coordination, network audit/apply | Maintenance can run locally and risky connectivity/reboot changes have explicit safety and acknowledgement |
| **M5: security and workstation policy** | SELinux/AppArmor/audit, certificate/secret resource, dconf/browser/session policy | The same API can express server security controls and interactive Linux endpoint policy without leaking secret values |
| **M6: optional breadth** | Archives/VCS, storage, containers, alternatives/capabilities/limits | Add only against observed fleet demand and after destructive-change controls are proven |

## Applicator design requirements

Every new applicator should answer the same questions:

1. **Desired state:** Is presence separate from configuration? Are omitted fields unmanaged or defaulted?
2. **Observed state:** Can Check compare every accepted field, including metadata and version?
3. **Provider:** Which distro/init/network/security backend is active, and is unsupported state a validation error rather than perpetual drift?
4. **Ownership:** Does Remotr own one entry, a named fragment, or the complete set? Append and authoritative modes must be explicit for groups, keys, firewall rules, and repositories.
5. **Safety:** Can the change break Remotr connectivity, boot, login, privilege escalation, storage, or secret confidentiality? Risky resources need audit/dry-run and validation before activation.
6. **Activation:** Does a change require reload, restart, daemon reload, reboot, logout, or next boot? Return that outcome structurally.
7. **Rollback:** What prior state is captured, where is it stored, how are secrets protected, and when is rollback impossible?
8. **Reporting:** Report redacted actual/desired state and a stable resource address; distinguish drift, apply failure, unsupported provider, and deferred maintenance.
9. **Concurrency:** Serialize package-manager, account database, firewall, network, and reboot operations using provider-appropriate locks.
10. **Testing:** Require provider unit tests plus container/VM integration tests for every advertised distro and negative tests for lockout/destructive cases.

## Scope guardrails

- Do not define “cross-platform Linux” as a single shell command with distro branches. Use a stable resource contract with explicit providers.
- Do not advertise a YAML field until Check and Apply both honor it, or reject it as unsupported during validation.
- Prefer Remotr-owned drop-ins and named fragments over rewriting distribution-owned monolithic files.
- Keep server-dispatched jobs and persistent endpoint schedules as separate resource types; their availability and execution guarantees differ.
- Treat connectivity, privilege, boot, disk, and secret changes as high-risk applicators with audit-first workflows.
- Measure the roadmap against real fleet requests. Mature tools prove the categories exist; they do not prove Remotr needs every provider or application module.

## Primary sources

- [Ansible built-in collection](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/index.html)
- [Ansible POSIX collection](https://docs.ansible.com/projects/ansible/latest/collections/ansible/posix/index.html)
- [Ansible community general collection](https://docs.ansible.com/projects/ansible/latest/collections/community/general/index.html)
- [Puppet resource type reference](https://help.puppet.com/core/current/Content/PuppetCore/Markdown/type.htm)
- [Salt state module reference](https://docs.saltproject.io/en/latest/ref/states/all/index.html)
- [Chef Infra bundled resources](https://docs.chef.io/client/19/resources/bundled/)
- [Linux kernel sysctl documentation](https://docs.kernel.org/admin-guide/sysctl/index.html)
- [OpenSSH `sshd(8)`](https://man.openbsd.org/sshd.8)
