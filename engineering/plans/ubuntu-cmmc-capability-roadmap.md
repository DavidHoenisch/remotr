# Ubuntu security-control capability roadmap

- **Created:** 2026-07-17
- **Audience:** Remotr maintainers, provider authors, and security reviewers
- **Status:** Proposed future feature roadmap
- **Scope:** Ubuntu endpoint controls that cannot yet become safe,
  ready-to-import Hub snippets

## Purpose

This roadmap records the product and evidence gaps exposed while reviewing a
candidate Ubuntu hardening catalog for environments that may handle Controlled
Unclassified Information. It is not a compliance profile and does not claim
that implementing any item satisfies CMMC or NIST requirements.

The roadmap exists to prevent two unsafe shortcuts:

1. publishing a Hub snippet because the YAML parses even though its provider,
   recovery path, or real Ubuntu evidence is incomplete; and
2. substituting a generic `file` or `command` resource when Remotr lacks a
   typed ownership, observation, redaction, and rollback contract.

Controls that are already safely expressible should be handled in the Hub
snippet workstream. This document contains only controls that need a product
capability, provider, safety foundation, evidence gate, or organization-bound
authoring mechanism first.

The current implementation baseline is described by the
[M1-M5 applicator gap report](../testing/applicator-m1-m5-gap-report.md). The
canonical vocabulary is indexed by the
[resource-kind reference](../../docs/reference/resource-kinds.md).

## What "cannot be safely expressed" means

An item belongs in this roadmap when at least one of these conditions applies:

| Classification | Meaning |
| --- | --- |
| **Missing contract** | No typed resource describes the desired and observed state, ownership boundary, lifecycle, activation, or rollback. |
| **Provider gap** | A resource contract exists, but the required Ubuntu backend or policy catalog is absent or rejected. |
| **Evidence gap** | Code exists, but its provider/distribution row is not advertised with the required public-seam, provider, and recovery evidence. |
| **Safety gap** | Enforcement can affect access, connectivity, boot, storage, or secrets without an accepted preflight and recovery contract. |
| **Organization binding** | A reusable entry needs operator-supplied values that must not ship as deployable `CHANGEME` placeholders. |
| **Non-endpoint control** | The outcome depends on policy, people, assessment, external services, or evidence handling rather than endpoint desired state alone. |

Schema acceptance is not sufficient. A capability graduates only when a real
Ubuntu provider is advertised and the applicable evidence in
[public test seams](../testing/public-seams.md) passes.

## Priority definitions

| Priority | Meaning |
| --- | --- |
| **P0** | Shared prerequisite for publishing any high-risk Ubuntu hardening entry. |
| **P1** | High-value endpoint control that closes a common identity, policy, repository, or inspection gap. |
| **P2** | Host, device, cryptographic, or security-tool breadth with greater platform coupling. |
| **P3** | External integration, assessment, and evidence capability built after core endpoint controls. |

## Qualified foundations do not imply Hub or compliance coverage

The exact Ubuntu 24.04 rows for the contracts below are now qualified. That
proves their declared provider behavior and recovery boundary; it does not make
an organization-neutral Hub module safe or establish a CMMC control. The
remaining gaps are narrower product, catalog, applicability, and authoring
requirements.

| Area | Qualified foundation | Remaining Hub or roadmap boundary |
| --- | --- | --- |
| PAM password, history, lockout, and last-login policy | `loginPolicy/pam-auth-update` | Password aging is UHF-100; MFA/smart-card stacks are UHF-101; organization-selected values require UHF-001. |
| Audit events | `auditRules/auditd` | A concrete Hub baseline still needs reviewed control intent and organization mappings; endpoint rules alone do not satisfy or export compliance evidence. |
| Custom AppArmor profiles | `appArmorProfile/apparmor-parser` | One named Remotr-owned profile is not the system-wide posture in UHF-208. |
| GNOME lock, desktop, and browser policy | `desktopSetting` and `sessionPolicy` for dconf/GSettings; mandatory `browserPolicy` for Chromium, Chrome, and Firefox | GDM is UHF-103; broader browser catalogs, Edge, user scope, and Firefox recommended policy are UHF-104; organization values require UHF-001. |
| Kernel-module USB storage policy | `kernelModule/kmod` | Module state is not the USB device/HID inventory and authorization model in UHF-203. |
| Enforced firewall and network policy | NetworkManager DNS/route/profile and exact nftables/firewalld rows listed in the support reference | A reusable service-reduction module still needs conditional applicability from UHF-002; netplan/networkd activation and firewalld enforcement remain unadvertised. |
| Service reduction for printing and remote access | `package/apt`, `service/systemd`, and qualified desktop settings | Optional installed facilities still require UHF-002 so absence is not misreported as drift or unsupported. |

Do not create parallel `command`-based Hub entries to bypass these boundaries.
Use the qualified typed row where it fits and retain the roadmap gap where it
does not.

## P0 shared foundations

### UHF-000 — Complete the high-risk execution foundation

**Desired outcome:** Access-, connectivity-, boot-, destructive-, and
secret-bearing resources can be reviewed, authorized, applied, acknowledged,
and recovered without relying on process-local or fictional rollback state.

**Current limitation:** The shared execution workstream and Ubuntu
qualification are accepted, including protected transaction storage,
schema-driven sensitivity, stable desired-state hashing, non-enforcing
high-risk preflight, and risk-selected recovery evidence. Future Hub entries
must still use those mechanisms; qualification does not waive them.

**Required capability:** No duplicate foundation is needed. Each future
high-risk capability must integrate with the accepted execution contract and
add its own exact provider, authorization, recovery, and real-environment
evidence.

**Safety and evidence:** Secret-canary tests, durable rollback reservation and
cleanup, stable plan hashes, authorization expiry, failed acknowledgement,
recovery after agent/server interruption, and the relevant Ubuntu VM fixtures.

**Graduation:** The shared foundation is complete. A high-risk Hub entry
graduates only after its capability-specific contract and evidence also pass.

### UHF-001 — Add typed Hub import parameters and review

**Desired outcome:** Operators can safely supply organization-defined values
such as warning text, idle timeouts, NTP servers, proxy settings, logging
destinations, approved repositories, firewall flows, and backup destinations.

**Current limitation:** Hub import copies a source verbatim. Deployable
placeholder values can be overlooked, while publishing one entry per numeric
or organization-specific value creates misleading duplicates.

**Required capability:** Add typed, bounded parameter declarations to catalog
entries and an import flow that:

- requires every parameter without a safe default;
- validates type, range, URI/host/path grammar, and secret-reference class;
- renders a deterministic preview before writing;
- records the selected values without putting secret material in the catalog,
  process arguments, logs, or generated comments; and
- produces ordinary kind-tagged repository sources after substitution, with no
  runtime template language in deployable artifacts.

**Safety and evidence:** Malformed, omitted, oversized, traversal, injection,
secret-canary, non-interactive, and desktop/CLI parity cases at the Hub import
seam.

**Graduation:** Parameterized entries contain no deployable `CHANGEME`, example
organization, example credential, or silently selected security value.

### UHF-002 — Add conditional applicability for optional host facilities

**Desired outcome:** A module can manage an optional installed facility without
failing endpoints where the package, service unit, desktop schema, hardware,
or backend is legitimately absent.

**Current limitation:** Distro and architecture targeting are too coarse for
"disable this service if installed" or "apply this policy only when this
desktop schema exists." Treating absence as drift or unsupported makes a
generic service-reduction baseline dishonest.

**Required capability:** Add a bounded applicability contract based on
authenticated endpoint capabilities and observed external facts such as
installed package, service unit, desktop schema, device class, or selected
configuration owner. Conditions must not run arbitrary shell code or silently
remove required resources from a composed artifact.

**Safety and evidence:** Deterministic author-time validation, explicit runtime
`not_applicable` reporting, no Apply for an unresolved condition, capability
document integrity, and mixed-fleet tests.

**Graduation:** Optional CUPS, Avahi, Bluetooth, Wi-Fi, SMB/NFS, RDP/VNC, and
desktop controls can distinguish absent, applicable-compliant,
applicable-drifted, and unsupported state.

## P1 policy and governance capabilities

### UHF-100 — Local password-aging policy

**Desired outcome:** Manage password minimum age, maximum age, warning period,
inactive period, and default policy for new accounts independently from account
expiration.

**Current limitation:** `user.expiry` expresses account expiration, not shadow
password-aging fields or distribution defaults. PAM complexity and history do
not replace per-account aging.

**Required capability:** Add structured, pointer-valued password-aging fields
or a dedicated account-aging resource with explicit existing-user and
new-user-default ownership.

**Safety and evidence:** Account-database locking, omission semantics,
non-password/system account handling, recovery-principal protection, exact
`chage`/shadow boundary assertions, and Ubuntu VM login recovery.

**Graduation:** Check reports every managed aging field independently and a
second Check is compliant without changing password material.

### UHF-101 — MFA and smart-card authentication providers

**Desired outcome:** Express organization-selected MFA and smart-card login
stacks without embedding tokens, certificates, PINs, or vendor-specific shell
installers in desired state.

**Current limitation:** `loginPolicy` can compose validated PAM rules, but
Remotr has no provider contract for token enrollment, smart-card middleware,
certificate mapping, offline behavior, break-glass access, or factor health.

**Required capability:** Define demand-backed providers for selected MFA and
smart-card ecosystems, including package prerequisites, PAM integration,
certificate trust, protected references, enrollment state, and recovery
principals.

**Safety and evidence:** Secret redaction, disconnected and expired-factor
behavior, failed middleware activation rollback, local-console and SSH
recovery, and real hardware or faithful isolated integration evidence.

**Graduation:** A provider advertises a specific supported stack and version;
there is no generic claim that arbitrary PAM modules constitute working MFA.

### UHF-102 — Automatic logout and session termination policy

**Desired outcome:** Manage policy-defined inactivity logout and termination
conditions separately from screen lock.

**Current limitation:** `sessionPolicy` manages GNOME idle lock and selected
lockdown settings. It does not observe or converge shell, graphical, SSH, and
systemd-logind session termination as one portable policy.

**Required capability:** Define provider-specific session classes and
termination semantics, including whether a setting affects future sessions or
actively terminates existing ones. Do not approximate a graphical logout with
shell `TMOUT` alone.

**Safety and evidence:** Logged-in and logged-out users, unsaved-session
warnings, administrative and service-account exclusions, activation reporting,
and deterministic no-wall-clock tests plus Ubuntu desktop/VM evidence.

**Graduation:** Reports distinguish screen lock, shell inactivity, remote
session timeout, future-session activation, and actual termination.

### UHF-103 — First-class GDM policy

**Desired outcome:** Manage the pre-login warning, user-list visibility, guest
login, automatic login, timed login, selected appearance, and mandatory GDM
settings.

**Current limitation:** Interactive-user dconf/GSettings resources do not own
the GDM system database and profile. A managed MOTD is not a graphical or SSH
consent banner, and organization branding cannot be safely hardcoded.

**Required capability:** Add a typed GDM provider with named ownership of its
dconf profile/database, supported keys, warning text, and asset references.
Use UHF-001 for organization text and branding values.

**Safety and evidence:** Staged dconf compilation, preservation of unrelated
databases, automatic-login access-risk review, display-manager restart
reporting without implicit termination, and Ubuntu graphical VM evidence.

**Graduation:** The provider can observe the effective greeter state after a
restart and recover from an invalid database or inaccessible branding asset.

### UHF-104 — Expand browser policy catalogs and providers

**Desired outcome:** Manage Safe Browsing, developer tools, extension allow and
block lists, update policy, certificate selection, homepage/proxy policy, and
insecure-protocol restrictions for explicitly supported browsers.

**Current limitation:** `browserPolicy` intentionally accepts only an allowlist
of Chromium/Chrome and Firefox policies. Microsoft Edge, user scope, Firefox
recommended policy, and many requested keys are unsupported.

**Required capability:** Version policy catalogs independently from the
resource schema; add native type and scope validation per browser/version; add
an Edge provider only after its Linux paths and policy behavior are tested.

**Safety and evidence:** Unknown-policy rejection, bounded object values,
preservation of unrelated keys, install/upgrade compatibility, browser restart
activation, and real packaged-browser evidence for each advertised policy.

**Graduation:** Catalog support is explicit per browser, policy, native type,
level, scope, and tested version range; package update behavior is not
misrepresented as a browser policy.

### UHF-105 — Authoritative repository governance

**Desired outcome:** Inventory approved Ubuntu repositories, detect or remove
unapproved PPAs and source fragments within a declared scope, require approved
signing keys, and report the effective package origin policy.

**Current limitation:** `aptRepository` and `aptSigningKey` own named Remotr
fragments. They intentionally preserve unrelated distribution and operator
sources and cannot claim an authoritative allowlist.

**Required capability:** Add an audited repository-set resource with bounded
authoritative ownership, protected distribution-source categories, PPA
classification, effective-origin observation, cleanup limits, cache-refresh
planning, and rollback.

**Safety and evidence:** Never remove the only viable Remotr or base operating
system source; validate signed metadata; preserve protected sources; restore
prior source/preference state when refresh fails; and run real Ubuntu APT
fixtures.

**Graduation:** Reports distinguish owned, approved external, protected base,
unapproved, unsigned, unreachable, and removed repositories.

### UHF-106 — Structured remote log forwarding

**Desired outcome:** Configure an approved remote logging destination and
report both local forwarding configuration and delivery health.

**Current limitation:** `journald` can forward locally to syslog, but Remotr
does not own a provider-neutral rsyslog/syslog-ng remote destination,
certificate trust, queue, retry, or effective delivery contract. A raw file
cannot prove that logs reached the collector.

**Required capability:** Add a structured remote-log provider with
organization-bound endpoints through UHF-001, TLS trust references, local queue
policy, bounded retry, service activation, and safe delivery telemetry.

**Safety and evidence:** Unreachable collector behavior, disk-queue bounds,
certificate mismatch, secret redaction, no log loss during reload, local
retention independence, and controlled collector integration evidence.

**Graduation:** Configuration compliance and recent delivery health are
reported separately; neither is claimed from the other.

### UHF-107 — Chrony time-synchronization provider

**Desired outcome:** Manage approved Chrony servers/pools and the provider's
enabled, configured, and effective synchronization state.

**Current limitation:** The advertised `timeSync` provider is
`systemd-timesyncd` only. Chrony configuration cannot be selected truthfully.

**Required capability:** Implement the existing provider-neutral contract for
Chrony, including named configuration ownership, source options that can be
represented safely, service activation, and effective synchronization probes.

**Safety and evidence:** Invalid or unreachable sources, large clock offset
handling, conflicting time providers, configuration validation, Apply and
second Check, and real Ubuntu VM evidence.

**Graduation:** Chrony is an advertised provider rather than a file-and-service
approximation, and reports configured state separately from synchronization
health.

## P2 host, device, and cryptographic capabilities

### UHF-200 — Dynamic filesystem posture and bounded remediation

**Desired outcome:** Assess home-directory permissions, world-writable paths,
missing sticky bits, SUID/SGID files, and other discovered filesystem posture;
optionally remediate only an explicitly authorized and bounded set.

**Current limitation:** `file` and `directory` manage explicit paths. They do
not perform fleet-wide discovery, express query scope, preserve assessment
evidence, or bound remediation of dynamically discovered objects.

**Required capability:** Add an audit-first filesystem-policy resource with
mount boundaries, include/exclude roots, file-type filters, maximum depth and
entry counts, ownership rules, result truncation, and separate authorization
for each remediation class.

**Safety and evidence:** Symlink and mount escape, special files, package-owned
files, concurrent mutation, huge trees, sensitive path redaction, bounded
results, and destructive VM fixtures for remediation.

**Graduation:** The provider can prove complete coverage within the declared
bounds and never turns a partial or truncated scan into a compliance claim.

### UHF-201 — ACL, extended-attribute, and Linux capability resources

**Desired outcome:** Manage POSIX ACLs, selected extended attributes, and Linux
file capabilities without opaque shell commands.

**Current limitation:** These fields are explicitly unadvertised. File mode and
ownership cannot represent them, and recursive command execution lacks bounded
ownership and safe removal semantics.

**Required capability:** Add separate typed resources or an explicitly scoped
filesystem metadata contract with named entries, merge/authoritative modes,
filesystem capability detection, and precise absence semantics.

**Safety and evidence:** Unsupported filesystems, symlinks, capability-induced
privilege, recursive bounds, exact process argv, provider conformance, and VM
security review.

**Graduation:** Check observes the kernel/filesystem effective metadata and
removes only state owned by the resource.

### UHF-202 — System and interactive-user umask policy

**Desired outcome:** Manage default file-creation masks for supported login,
shell, service, and graphical session classes.

**Current limitation:** A PAM rule or shell fragment covers only part of the
problem. Remotr has no contract that identifies which session classes are
managed or observes their effective defaults.

**Required capability:** Add a structured default-permission policy with
provider-specific targets, explicit exclusions, named fragment ownership, and
activation reporting.

**Safety and evidence:** Existing sessions versus future sessions, system and
service accounts, shells that ignore common profiles, conflicting fragments,
and real file-creation tests for each advertised session class.

**Graduation:** Reports identify the effective mask per managed class instead
of claiming that one edited file controls every process.

### UHF-203 — USB device and HID policy

**Desired outcome:** Move beyond USB-storage blacklisting to device-class and
device-identity allow/deny policy, optional HID restrictions, insertion
events, and recoverable local administration.

**Current limitation:** `kernelModule` can control `usb-storage` but cannot
identify physical devices or safely distinguish keyboards, security tokens,
docks, storage, and other USB classes. A blanket HID block can remove the only
local recovery path.

**Required capability:** Add a USBGuard-backed or equivalent provider with
stable device attributes, explicit default policy, learning/report mode,
console-recovery protection, and bounded event telemetry.

**Safety and evidence:** Boot devices, keyboards, smart cards, composite and
spoofed devices, hotplug races, agent disconnect, policy rollback, and real
hardware or USB-passthrough VM evidence.

**Graduation:** Enforcement follows a reviewed observed-device inventory and
cannot remove all declared recovery devices.

### UHF-204 — Destructive storage and LUKS lifecycle

**Desired outcome:** Observe and manage approved encrypted storage, including
LUKS formatting/enrollment, crypttab ownership, unlock methods, and recovery
metadata.

**Current limitation:** Remotr manages mounts and swap only. Partitioning,
filesystems, LVM, RAID, encryption, and crypttab are explicitly deferred.
Retrofitting full-disk encryption is destructive and cannot be made safe with
a bootstrap command.

**Required capability:** Create a demand-backed destructive-storage change
with stable device identity, discovery-only planning, maintenance windows,
irreversible-operation acknowledgement, external recovery material, and
post-boot verification.

**Safety and evidence:** Ambiguous devices, root and Remotr-state protection,
power interruption, partial format, missing recovery key, failed initramfs,
reboot loop prevention, and disposable VM recovery fixtures.

**Graduation:** Discovery and planning ship before mutation; no generic Hub
snippet formats an existing device.

### UHF-205 — TPM enrollment and key-protection providers

**Desired outcome:** Use supported TPM hardware for LUKS enrollment, measured
unlock, protected rollback keys, and other explicitly scoped key-protection
purposes.

**Current limitation:** Endpoint inventory reports TPM presence, and the
rollback design prefers TPM sealing, but there is no advertised TPM provider
or enrollment lifecycle.

**Required capability:** Define separate TPM purposes, PCR policy, enrollment
identity, recovery-key requirements, reseal behavior, ownership, and safe
replacement. TPM presence must never imply that a requested purpose is
supported.

**Safety and evidence:** Firmware/boot changes, PCR drift, cleared TPM,
multiple enrolled slots, unsupported TPM versions, recovery-key use, and VM
vTPM plus reviewed real-hardware evidence.

**Graduation:** Reports distinguish present, healthy, enrolled for a declared
purpose, locked out, recovery-required, and unsupported state without exposing
key material.

### UHF-206 — Ubuntu FIPS lifecycle

**Desired outcome:** Observe and manage an explicitly supported Ubuntu FIPS
mode, including entitlement prerequisites, package/kernel state, reboot, and
post-boot verification.

**Current limitation:** FIPS mode is distribution-, release-, kernel-, and
subscription-specific. Package installation or a generic command cannot prove
that the running kernel and cryptographic modules are the approved FIPS set.

**Required capability:** Add an Ubuntu-specific provider with declared support
matrix, protected entitlement references, preflight, package/channel
selection, coordinated reboot, boot verification, rollback limits, and an
honest disable/migration contract.

**Safety and evidence:** Unsupported release, missing entitlement, package
failure, incompatible applications, failed FIPS kernel boot, prior-kernel
recovery, secret redaction, and real Ubuntu VM evidence.

**Graduation:** The provider reports configured and running FIPS state
separately and never equates installed packages with active mode.

### UHF-207 — Scoped TLS and cryptographic policy

**Desired outcome:** Express approved protocol, cipher, trust, and key
requirements for a declared consumer rather than promising one universal Linux
TLS setting.

**Current limitation:** Certificates and trust anchors are first-class, but
protocol and cipher policy remains application-specific. Ubuntu has no single
setting that safely controls OpenSSH, browsers, reverse proxies, Java, and
arbitrary application libraries together.

**Required capability:** Define consumer-scoped providers or a proven system
crypto-policy backend. Every provider must identify its configuration owner,
supported versions, validation command, activation, compatibility impact, and
rollback.

**Safety and evidence:** Known client compatibility fixtures, invalid policy,
service recovery, certificate/trust dependencies, protocol negotiation tests,
and no cross-application compliance inference.

**Graduation:** Each policy claim names the affected consumer and independently
observes its effective protocol/cipher behavior.

### UHF-208 — System-wide AppArmor posture

**Desired outcome:** Inventory enabled, enforcing, complain, disabled,
unconfined, and missing expected profiles, including distribution-owned
profiles, without claiming ownership of all profile content.

**Current limitation:** `appArmorProfile` manages one named Remotr-owned
profile. It cannot define an authoritative expected set across distribution and
application packages or safely force every existing profile to enforce mode.

**Required capability:** Add an audit-first AppArmor posture resource with
expected-profile selectors, ownership classification, bounded results, and
explicit per-profile enforcement authorization.

**Safety and evidence:** Parser/load failures, package upgrades, unconfined
processes, deleted profiles, essential-service denial, complain-to-enforce
rollout, and Ubuntu VM recovery.

**Graduation:** Reports distinguish owned and external profiles and never
rewrite package-owned content to satisfy an expected-set policy.

## P3 integrations, assessment, and evidence

### UHF-300 — Security-tool health and finding telemetry

**Desired outcome:** Report whether endpoint security tools are installed,
running, current, completing scheduled work, and producing actionable findings.

**Current limitation:** Packages, services, and endpoint schedules can install
and run tools such as ClamAV, but schedule configuration compliance does not
prove signature freshness, successful scans, detections, quarantine, or alert
delivery.

**Required capability:** Add a bounded security-tool result contract with tool
identity/version, health, definition age, run identity, completion state,
finding counts, safe summaries, and explicit artifact references. Provider
adapters must remain tool-specific.

**Safety and evidence:** Untrusted output parsing, huge result sets, malicious
filenames, CUI leakage, stale results, interrupted scans, quarantine ownership,
and redaction/fuzz tests.

**Graduation:** Tool configuration, runtime health, and findings are three
separate report dimensions.

### UHF-301 — Falco runtime-monitoring integration

**Desired outcome:** Install a pinned Falco stack, manage supported rule
sources and service state, and report health and bounded security events.

**Current limitation:** Generic repository/package/service resources can
approximate installation but cannot validate kernel-driver/eBPF compatibility,
rule schema, event transport, or runtime detection health.

**Required capability:** Build a versioned Falco provider or application
integration with pinned repository trust, driver capability detection,
validated named rule ownership, output routing, and UHF-300 telemetry.

**Safety and evidence:** Kernel compatibility, driver load failure, resource
overhead, invalid rules, event storms, sensitive event fields, upgrades, and
real Ubuntu kernel evidence.

**Graduation:** The integration advertises a bounded version/kernel matrix and
does not equate a running service with functioning detection.

### UHF-302 — Backup and restore provider contract

**Desired outcome:** Express protected sources, destination, schedule,
retention, encryption, credential references, recent backup health, and
periodic restore verification.

**Current limitation:** `endpointSchedule` can launch an external backup
program, but Remotr cannot infer backup completeness, encryption, retention,
remote durability, or restore success from an exit code or installed cron
entry.

**Required capability:** Define demand-backed providers for selected backup
tools and destinations, with typed source/retention policy, protected secrets,
snapshot identity, manifest/hash verification, restore-test isolation, and
bounded telemetry.

**Safety and evidence:** CUI redaction, excluded paths, partial uploads,
credential rotation, retention deletion bounds, ransomware/untrusted backup
content, offline behavior, and destructive restore isolation.

**Graduation:** Configuration, last backup, retained recovery points, and last
successful restore test are reported separately.

### UHF-303 — Compliance scanner execution and result ingestion

**Desired outcome:** Run explicitly supported OpenSCAP, Lynis, Ubuntu Security
Guide, or similar profiles and ingest versioned, bounded findings.

**Current limitation:** A package plus scheduled command can start a scanner,
but it cannot pin profile identity, validate applicability, collect results,
distinguish tool errors from failed checks, or preserve assessment provenance.

**Required capability:** Add a scanner provider contract covering tool and
profile version, applicability, invocation, result schema, artifact digest,
finding severity/identity, exceptions, retention, and safe upload.

**Safety and evidence:** Unsupported profiles, parser fuzzing, huge or corrupt
reports, sensitive evidence, timeout, partial completion, result signing, and
representative Ubuntu fixtures.

**Graduation:** Scanner findings are observations, not Remotr compliance
decisions, and every result retains its tool/profile/version provenance.

### UHF-304 — Control mapping and evidence export

**Desired outcome:** Correlate selected Remotr desired state, provider results,
scanner findings, exceptions, and operational evidence with organization-owned
control statements and export an assessment package.

**Current limitation:** Drift and activity reports are product telemetry, not a
complete CMMC evidence system. They do not include policies, procedures,
interviews, assessor judgment, system scope, or every external safeguard.

**Required capability:** Add an organization-owned mapping layer with versioned
control references, scoped assets, evidence-object links, collection times,
source identity, retention, exception/POA&M references, redaction, and signed
export manifests. Keep mappings independent from resource implementation.

**Safety and evidence:** Tenant/role authorization, stale evidence, deleted or
superseded artifacts, secret/CUI handling, tamper detection, export size, and
negative tests proving that absent evidence cannot become a satisfied claim.

**Graduation:** Remotr can export traceable technical evidence while explicitly
leaving requirement satisfaction and assessment conclusions to authorized
people and processes.

## Organization-bound controls that must not become fixed defaults

The following values may be represented only after UHF-001 or inside an
operator-owned configuration repository. Their absence from a universal Hub
baseline is intentional:

- legal warning and consent language;
- organization logo, wallpaper, help-desk contact, and other branding;
- lock, inactivity, automatic-logout, password-age, history, and lockout
  values;
- approved NTP, DNS, proxy, syslog, update, repository, certificate, and secret
  endpoints;
- firewall flows, remote-access methods, Wi-Fi policy, and IPv6 policy;
- unused services and unnecessary package lists;
- approved browser extensions, blocked URLs, homepages, and internal trust;
- audit retention and event-selection policy beyond a documented technical
  minimum;
- backup sources, exclusions, destinations, retention, recovery objectives,
  encryption ownership, and restore cadence; and
- application-specific TLS protocols, cipher suites, and compatibility
  exceptions.

These are organization decisions, not missing universal constants. A future
feature may make them safer to author, but Remotr must not choose them on an
operator's behalf.

## Explicit non-goals

- Do not create a monolithic `cmmc-compliant` module.
- Do not treat a passing parser or import test as provider support.
- Do not use `command` merely to bypass a missing typed resource.
- Do not treat an installed package or active service as proof of effective
  security behavior.
- Do not silently skip unsupported state to make an endpoint look compliant.
- Do not publish access, network, boot, storage, or secret enforcement without
  its required recovery evidence.
- Do not encode legal, contractual, assessor, or organizational decisions as
  product defaults.
- Do not claim that technical endpoint evidence replaces system scope,
  policies, procedures, interviews, or assessor judgment.

## Proposed feature waves

### Wave 0 — Safe authoring and enforcement foundations

1. UHF-000 high-risk execution foundation.
2. UHF-001 typed Hub parameters and import review.
3. UHF-002 conditional applicability.
4. Promote the required Ubuntu provider rows only after real evidence passes.
5. Add representative, non-enforcing schema-1 modules to a checked-in composed
   repository.

### Wave 1 — Identity, desktop, repository, and logging policy

1. UHF-100 password aging.
2. UHF-101 MFA/smart-card providers selected by demonstrated demand.
3. UHF-102 session termination.
4. UHF-103 GDM policy.
5. UHF-104 browser policy expansion.
6. UHF-105 authoritative repository governance.
7. UHF-106 remote log forwarding.
8. UHF-107 Chrony.

### Wave 2 — Filesystem, device, storage, and cryptographic lifecycle

1. UHF-200 filesystem posture.
2. UHF-201 ACLs, attributes, and capabilities.
3. UHF-202 umask policy.
4. UHF-203 USB/HID policy.
5. UHF-204 LUKS and destructive storage.
6. UHF-205 TPM providers.
7. UHF-206 Ubuntu FIPS lifecycle.
8. UHF-207 scoped TLS policy.
9. UHF-208 system-wide AppArmor posture.

### Wave 3 — Security integrations and evidence

1. UHF-300 security-tool telemetry.
2. UHF-301 Falco integration.
3. UHF-302 backup and restore providers.
4. UHF-303 compliance scanner ingestion.
5. UHF-304 control mapping and evidence export.

Ordering within a wave is demand-backed. A concrete fleet use case, owner,
supported-version matrix, and test environment are required before opening an
implementation change.

## Graduation into the Hub

A roadmap item may produce or unblock a Hub entry only after all applicable
conditions below are true:

1. An accepted OpenSpec change defines observable behavior, unsupported state,
   ownership, risk, activation, rollback, and reporting.
2. The feature is implemented through a typed resource or provider; any use of
   `command` has an explicit, reviewed exception and independently tested
   idempotency and recovery.
3. Author-time validation rejects impossible targets and runtime mismatches
   report `unsupported` or `not_applicable`, never ordinary compliance.
4. A focused public-seam test was written first, observed red, and made green
   by the minimum implementation.
5. Provider contract cases cover compliant, drifted, Apply, second Check,
   absence, unsupported, probe failure, activation, cancellation, locking,
   redaction, and rollback where applicable.
6. Every advertised Ubuntu release/provider row passes in the environment
   appropriate to the claim. Access, connectivity, boot, storage, kernel, and
   authentication behavior includes VM recovery evidence.
7. Secret-bearing behavior includes negative, canary, persistence, cleanup,
   and mutation evidence where required.
8. A checked-in kind-tagged repository imports, validates, discovers, and
   renders the resource through the public configuration CLI.
9. The catalog description states prerequisites, organization-defined values,
   risk and activation effects, supported releases/providers, and known
   limitations without making a compliance claim.
10. The focused suite and `make test` pass before handoff. Higher-risk provider,
    VM, performance, load, or end-to-end checks run according to the repository
    evidence policy.

The [Hub contribution guide](../../hub/README.md) remains the final publication
contract. This roadmap does not relax its requirement that every catalog
source be complete, importable, registered, and valid.
