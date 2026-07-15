# Applicator M1–M5 exit-criteria and gap report

- **Audit date:** 2026-07-15
- **Validation baseline:** `244bda0`
- **OpenSpec change:** `expand-linux-system-administration-applicators`
- **Result:** M1–M5 implementation is broad, but none of the five milestone
  exit criteria is yet release-complete against the checked-in composed
  repositories and advertised provider evidence.

This report separates three claims that must not be conflated:

1. a resource kind or field has implementation and focused tests;
2. a checked-in configuration repository composes that resource through the
   public operator CLI; and
3. a provider/distribution row has passing real-environment evidence and may
   therefore be advertised.

## Real composed repository audit

The public `remotr config validate`, `discover`, and `render` seams were run
against both checked-in kind-tagged repositories.

| Repository and fleet | Validation and composition | Canonical surface actually exercised | Gap exposed |
| --- | --- | --- | --- |
| `compose/config-repo`, `test-fleet` | Passed validation, discovery, desired rendering, and cron rendering | A native package, a Remotr catalog package, and one server-dispatched cron | No M2–M5 applicator is represented |
| `deploy/fly/config-repo`, `default` | Passed validation, discovery, and rendering | APT and Pacman package resources | Its source module is schema 0 and correctly emits `legacy_schema_0`; no M2–M5 applicator is represented |

The rendered artifacts were canonical schema 1 and deterministic. This proves
the composition path used by deployed examples, but it does not turn absent
resource categories into provider evidence.

## Milestone exit decisions

| Milestone | Implemented evidence observed | Exit decision and remaining gap |
| --- | --- | --- |
| **M1: truthful convergence** | Canonical package lifecycle and version policy, file/directory/link metadata, UID handling, and bounded nftables enforcement exist. | **Not met.** The real repositories cover packages only. Package, filesystem, identity, service, and repository container rows remain `untested`; firewalld enforcement is unavailable; several M1 traceability scenarios remain planned. |
| **M2: access baseline** | Group, expanded user, authorized-key, known-host, and sudo resources exist; the local-administrator workflow and isolated login/user safety selectors pass. | **Not met.** Neither real repository composes the workflow, all Debian/Ubuntu/Arch identity rows remain `untested`, and `OS-LIA-013` still lacks an assigned passing selector. |
| **M3: OS baseline** | APT repositories/keys, sysctl, kernel modules, hostname/locale/time, mounts, and swap exist. Debian 12 VM rows pass for kernel modules, host locale, and time sync; the mount VM selector also passes. | **Not met.** No real repository composes an M3 baseline. APT repository rows remain `untested`, there are no advertised rows for sysctl, hostname, mounts, or swap, and RPM-family repository/security support is deferred. |
| **M4: durable operations** | Cron and systemd-timer endpoint schedules, systemd service/unit state, coordinated reboot, hosts/DNS/routes, network profiles, and guarded nftables/network activation exist; the relevant process and VM-safety selectors pass. | **Not met.** No real repository composes these resources. Service rows remain `untested`, no schedule/network/firewall provider row is advertised, firewalld enforcement is unavailable, and `OS-NFM-003`/`OS-NFM-004` remain planned. |
| **M5: security and workstation policy** | Certificates, trust anchors, AppArmor, audit rules, account limits, Debian/Ubuntu login policy, journald, logrotate, dconf/GSettings/session policy, and Chromium/Chrome/Firefox policy exist with focused redaction and safety tests. | **Not met.** No real repository composes this baseline and no security, authentication, logging, desktop, or browser provider row is advertised. SELinux and authselect remain deferred; `OS-LSM-001`/`OS-LSM-002` remain planned. |

## Provider evidence snapshot

`test/provider-matrix.yaml` has 18 rows:

- **Passing:** 3 Debian 12 VM rows — kernel module/kmod, host locale/systemd-localed,
  and time sync/systemd-timesyncd.
- **Untested:** 15 container rows — package, filesystem, identity, service, and
  repository across Debian 12, Ubuntu 24.04, and Arch 2026-07-06.

The container matrix command passed during this audit, but those selectors
only verify pinned image/backend discovery. The matrix explicitly says that
this is not a support claim, so the rows remain `untested`.

At the audit baseline, the umbrella traceability manifest classified 123 of
227 scenarios as `verified` and 104 as `planned`; none was merely `accepted`.
Its largest cross-cutting gap was the execution contract (11/75 verified).
The separate testing-foundation change had 3 of 50 scenarios verified and 47
planned, including provider conformance (3/10), test-quality gates (0/15),
specification traceability (0/11), and performance/scale assurance (0/14).
Feature implementation was a prerequisite for many of these selectors, but
the missing selectors still prevent a release-evidence claim.

## Unsupported and deferred field/provider inventory

This table is the exhaustive set of explicit field/provider limitations found
in the canonical configuration reference, capability matrix, resource
registry, and umbrella specifications at the audit baseline. A runtime backend
mismatch returns `unsupported`; an unadvertised roadmap provider is rejected
during validation or has no resource kind.

| Area | Unsupported or restricted now | Deferred/unadvertised |
| --- | --- | --- |
| Distribution and native packages | Native targeting is APT on Debian/Ubuntu and Pacman on Arch. `lifecycle: purged` and `hold` are APT-only; interactive transactions are rejected; `yay` is rejected because no truthful AUR provider is advertised. | Fedora/RHEL and DNF4/DNF5, RPM repositories, image-based RPM systems, APK, Zypper, Snap, and other immutable-image providers. |
| Repository trust | Only APT repository and scoped signing-key resources are registered, and they reject non-Debian/Ubuntu targets. | Pacman/AUR repository trust and every RPM/APK/Zypper/Snap repository provider. |
| Filesystem and deployment | ACL and extended-attribute fields are not advertised; SELinux labels are unavailable. | Linux file capabilities, archive extraction/deployment, and revision-pinned VCS deployment. |
| Host and time | Time sync advertises only `systemd-timesyncd`; non-systemd host locale/time backends are unavailable. | Chrony and other NTP providers, plus non-systemd hostname/locale providers. |
| Storage | The core surface is mounts plus swap files/devices; a force unmount requires explicit enforcement and swap removal requires `allowRemove`. | Partitioning, filesystems, LVM, RAID, encryption/crypttab, quotas, and other destructive storage providers. |
| Services and schedules | `systemd` is the only advertised service provider. OpenRC/SysV authoring is rejected; their lack of masking/user-scope/linger is never approximated. Endpoint schedules support only cron and systemd timers. | Passing OpenRC/SysV implementations and real-environment rows; any additional scheduler backend. |
| Firewall | Enforced mutation is nftables-only. Firewalld is audit/report-only; UFW and iptables are not selectable. | Transactional firewalld enforcement and provider-contract implementations for UFW/iptables. |
| DNS, routes, and profiles | DNS and route resources are NetworkManager-only. Network profile providers are NetworkManager, netplan, and systemd-networkd; networkd is Ethernet-only. File-backed netplan/networkd profiles with credential references cannot be enforced and remain audit-only. | Other network owners and protected credential activation for file-backed profiles. |
| Mandatory access control and login | AppArmor is Ubuntu/AppArmor-only. Login policy is `pam-auth-update` on Debian/Ubuntu only. | SELinux mode/boolean/context/port/module/user/login resources and RPM-family `authselect`. |
| Trust and secret resolution | Trust-anchor refresh is implemented only for Debian/Ubuntu and Arch. Secret references resolve only through `local-file` and the Remotr registry; a provider is never a fallback for another provider. | RPM-family trust stores and additional external secret-provider integrations. |
| Desktop/session | dconf and GSettings require matching endpoint capabilities. `sessionPolicy.defaultApplications` supports merge ownership only because authoritative cleanup is not portable. | Other desktop/session providers and authoritative default-application cleanup. |
| Browser policy | Scope is system-only. Chromium/Chrome support mandatory and recommended policy locations; Firefox recommended policy is unsupported. Unknown policy names, types, and levels report `unsupported`. | Other browsers, user-scope browser policy, and unsupported provider policy catalogs. |
| M6 optional breadth | No optional M6 resource is advertised. | Containers, alternatives, Linux file capabilities, environment fragments, transient paths, archive/VCS deployment, and destructive storage all require demand-backed child OpenSpec changes. |

## Release follow-up

Before an M1–M5 milestone is advertised as complete:

1. add representative schema-1 modules to a checked-in composed repository
   without enabling risky enforcement by default;
2. promote each claimed provider/distribution row from `untested` to `passing`
   only after its real contract selector passes;
3. assign and pass the remaining public-seam traceability selectors; and
4. re-run this audit so the milestone decision is based on repository,
   provider, and traceability evidence together.

## Archive gate decision — 2026-07-15

The umbrella change is **not eligible for archive** and was not archived.
Task 1.8 is treated as sequencing debt rather than an implementation blocker,
but the following independent conditions still fail task 14.10:

- non-optional tasks 2.9, 2.10, and 2.11 remain unchecked and have not been
  explicitly descoped through an approved OpenSpec update;
- the M1–M5 exit audit above found no milestone release-complete against both
  real composed repositories and provider evidence; and
- after the 14.9 compatibility guard was verified, the umbrella manifest still
  has 124 verified and 103 planned scenarios.

Re-evaluate 14.10 only after the remaining non-optional tasks are accepted or
explicitly descoped and the gap report's release conditions are closed. Until
then, leaving 14.10 unchecked is the policy-preserving outcome.
