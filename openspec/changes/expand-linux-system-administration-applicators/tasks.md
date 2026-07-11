## 1. Resolve Foundation Decisions

- [ ] 1.1 Choose the canonical artifact schema identifier and compatibility-window length; record the decision and update migration examples.
- [ ] 1.2 Define the endpoint capability document, including facts-derived and agent-version-derived fields, and record how the server selects compatible artifacts.
- [ ] 1.3 Define rollback retention, protection, and encryption rules for public, sensitive, and secret-bearing resources.
- [ ] 1.4 Define the authorization and acknowledgement workflow required for enforced network, reboot, boot-risk, and destructive storage changes.
- [ ] 1.5 Select the initial Fedora/RHEL versions and DNF generation or explicitly defer RPM-family support from the first release gate.
- [ ] 1.6 Select the first secret provider and define endpoint/resource-scoped authorization plus the provider extension interface.
- [ ] 1.7 Review M6 demand evidence and mark each optional primitive accepted, deferred, or rejected without changing M1–M5 completion criteria.

## 2. Build the Applicator Execution Contract

- [ ] 2.1 Add typed check statuses, reason codes, redacted desired/observed summaries, and contract tests for exhaustive status handling.
- [ ] 2.2 Replace or adapt `executor.Handler.State(any, bool)` with structured Check while keeping current handlers behaviorally compatible during migration.
- [ ] 2.3 Add structured Apply results for changed state, activation signals, reboot requirement, deferred work, rollback class, and diagnostics.
- [ ] 2.4 Update executor failure handling so the original apply error and separate rollback outcome are both retained and tested.
- [ ] 2.5 Prevent Apply for compliant, unsupported, check-failed, deferred, report-only, dependency-blocked, or failed-preflight resources.
- [ ] 2.6 Add normal, sensitive, connectivity, access, boot, and destructive risk metadata with preflight hooks and safe default-policy tests.
- [ ] 2.7 Implement exclusive lock domains with bounded provider-native lock waits and lock-contention tests.
- [ ] 2.8 Implement activation collection, dependency-aware ordering, deduplication, and execution for daemon-reload, reload, restart, logout, next-boot, and reboot-required signals.
- [ ] 2.9 Add protected transaction metadata storage keyed by resource address and artifact digest, including retention and cleanup.
- [ ] 2.10 Add schema-driven sensitivity classification and prove via tests that secret values cannot enter logs, reports, diagnostics, or generic backups.

## 3. Version and Register Desired-State Resources

- [ ] 3.1 Add strict canonical artifact decoding with schema-version validation, unknown-field rejection, and precise resource-address diagnostics.
- [ ] 3.2 Define canonical shared metadata for kind, name, lifecycle, dependencies, provider options, policy, ownership, validation, notifications, and risk overrides.
- [ ] 3.3 Implement a resource registry covering decode, validate, sensitivity, risk, provider factory, ordering tier, and lock domains.
- [ ] 3.4 Refactor resolver and engine node construction to iterate registered resources and verify that no resource collection is dropped during resolution.
- [ ] 3.5 Enforce configuration-wide cross-kind name uniqueness and validate dependency existence/cycles against stable addresses.
- [ ] 3.6 Add provider registry and normalized facts for distro family/version, init, package, firewall, network, security, and desktop backends.
- [ ] 3.7 Implement capability-matrix validation that rejects statically impossible target/provider/field combinations and returns runtime unsupported for local mismatches.
- [ ] 3.8 Add legacy plural-collection compatibility decoding, canonical rendering, deprecation diagnostics, and golden migration fixtures.
- [ ] 3.9 Update `config discover`, `validate`, and `render` to understand canonical kinds and capability requirements without writing composed artifacts to source repos.
- [ ] 3.10 Update the configuration reference and examples only for resource fields whose vertical implementation slices are complete.

## 4. Expand Compliance Telemetry and Operator Reporting

- [ ] 4.1 Version drift/apply telemetry additively and encode all structured statuses, reason codes, provider identity, activation, rollback, and redacted summaries.
- [ ] 4.2 Update server persistence and endpoint/fleet state-report models for compliant, drifted, unsupported, check-failed, deferred, apply-failed, and no-report buckets.
- [ ] 4.3 Update operator CLI state-report output and JSON contracts to expose the new buckets and bounded per-resource diagnostics.
- [ ] 4.4 Preserve digest-based unchanged suppression and add payload-size bounds for expanded reports.
- [ ] 4.5 Add mixed-version server/agent compatibility tests covering legacy reports, new reports, and agent downgrade during the schema window.
- [ ] 4.6 Add redaction integration tests that trace secret-like canaries from desired state through agent logs, sync payloads, Postgres, APIs, and CLI output.

## 5. Establish Provider Test and Release Gates

- [ ] 5.1 Create reusable provider contract tests for compliant, drifted, apply, idempotence, unsupported, check failure, validation failure, lock contention, redaction, and rollback cases.
- [ ] 5.2 Define the supported distro/backend capability matrix in versioned test data and make feature advertisement derive from passing entries.
- [ ] 5.3 Add Debian, Ubuntu, and Arch container integration jobs for package, filesystem, identity, service, and repository providers as each lands.
- [ ] 5.4 Add VM test harnesses for network rollback, reboot acknowledgement, mounts, kernel modules/sysctl, MAC policy, and destructive storage safety.
- [ ] 5.5 Add negative recovery tests for Remotr connectivity loss, SSH/sudo lockout, boot-risk changes, secret leakage, and ambiguous/destructive devices.
- [ ] 5.6 Require schema, validation, composition, provider, engine, telemetry, documentation, migration fixture, and integration coverage before advertising any new field or provider.

## 6. Deliver M1 Package Truthful Convergence

- [ ] 6.1 Replace package `present` boolean ambiguity with canonical present/absent/provider-supported-purged lifecycle and compatibility mapping.
- [ ] 6.2 Implement native installed-version observation and exact-version convergence for APT, including explicit upgrade/downgrade policy and unavailable-version errors.
- [ ] 6.3 Implement native installed-version observation and exact-version convergence for Pacman with the same policy/result contract.
- [ ] 6.4 Separate Pacman and Yay/AUR providers; implement and test a truthful Yay provider or reject Yay as unsupported.
- [ ] 6.5 Implement hold/pin, cache refresh, dependency-removal, and noninteractive transaction fields only for providers that can check and apply them.
- [ ] 6.6 Resolve the DNF/facts mismatch by either completing selected Fedora/RHEL facts plus DNF Check/Apply or removing DNF from advertised schema for this gate.
- [ ] 6.7 Serialize package transactions, honor native locks/timeouts, sanitize environments, and return bounded provider diagnostics.
- [ ] 6.8 Detect and report service/reboot activation requirements from package transactions without implicit reboot.
- [ ] 6.9 Add migration, validation, unit, integration, and idempotence coverage proving every advertised package field converges.

## 7. Deliver M1 Filesystem, Download, User, and Firewall Correctness

- [ ] 7.1 Make system-file and user-file Check observe managed mode, owner, and group independently from content and repair metadata-only drift.
- [ ] 7.2 Add explicit file present/absent lifecycle and safe removal while retaining compatible whole-file and line-regex behavior.
- [ ] 7.3 Replace direct truncating writes with validated same-filesystem staging, fsync policy, atomic rename, and prior-state preservation.
- [ ] 7.4 Extend no-follow safe traversal to system filesystem objects and add traversal/symlink race tests.
- [ ] 7.5 Expand remote-file resources with lifecycle, checksum/signature verification, trusted signer, secret authentication reference, redirect/timeout policy, and atomic activation.
- [ ] 7.6 Migrate download `notifySystemd` and `reloadExec` behavior to shared structured activation signals with compatibility fixtures.
- [ ] 7.7 Make user Check and Apply enforce requested UID with explicit safe reassignment behavior and account-database locking.
- [ ] 7.8 Reject all user fields that are not yet supported by Check and Apply instead of parsing them early.
- [ ] 7.9 Add firewall present/absent lifecycle to firewalld and nftables with stable managed-rule identities and removal tests.
- [ ] 7.10 Replace firewall audit-log-as-compliance behavior with a structured plan/status while preserving audit-by-default.
- [ ] 7.11 Fix resource resolution and engine coverage tests so firewall and every registered current kind survive parse, compose, resolve, order, check, and report.
- [ ] 7.12 Add the M1 acceptance test proving every advertised package/file/download/user/firewall field is enforced or rejected.

## 8. Deliver M2 Filesystem and Local Access Baseline

- [ ] 8.1 Add canonical directory, symbolic-link, and hard-link resources with lifecycle, type replacement policy, ownership, mode, and safe traversal.
- [ ] 8.2 Add bounded recursive directory policy with explicit purge, cross-filesystem behavior, exclusions, and authoritative ownership tests.
- [ ] 8.3 Add group presence/GID/system-class provider and serialize it with all account-database changes.
- [ ] 8.4 Expand user providers for primary group, merge/authoritative supplementary groups, home policy, shell, comment, and system-account class.
- [ ] 8.5 Add password reference, lock/unlock, expiry, and safe removal fields with protected-account and Remotr-runtime safeguards.
- [ ] 8.6 Add structured authorized-key entries with restrictions, fingerprint, merge/authoritative modes, safe home traversal, and revocation tests.
- [ ] 8.7 Add structured known-host entries with fingerprint, hashing policy, merge-safe editing, and replacement controls.
- [ ] 8.8 Add named sudo fragments with complete effective `visudo` validation, atomic activation, rollback disclosure, and recovery-principal preflight.
- [ ] 8.9 Add the M2 integration flow that provisions and revokes a local administrator without generic commands or unmanaged passwd/SSH/sudo edits.
- [ ] 8.10 Publish M2 canonical YAML, compatibility guidance, provider matrix, and access-lockout recovery documentation.

## 9. Deliver M3 Repositories and Host Baseline

- [ ] 9.1 Add APT signing-key resources with declared fingerprints, scoped keyrings, atomic install/removal, and mismatch rejection.
- [ ] 9.2 Add APT repository resources with named fragments, enable/disable/absent lifecycle, architectures, suites, components, priority, and credential references.
- [ ] 9.3 Order signing keys, repositories, coalesced cache refresh, and dependent package resolution through explicit dependencies.
- [ ] 9.4 Add DNF repository/key support only after the selected Fedora/RHEL provider passes the complete M1 contract.
- [ ] 9.5 Add sysctl runtime/persistent scopes, Remotr-owned drop-ins, single-key/reload/next-boot activation, and unsupported-key results.
- [ ] 9.6 Add kernel-module loaded/persistent/parameters/blacklist state with network/root/boot protection for unload and blacklist.
- [ ] 9.7 Add static/transient hostname management separately from hosts-entry ownership.
- [ ] 9.8 Add independently optional timezone, locale, and keymap fields with logout/reboot activation reporting.
- [ ] 9.9 Add provider-neutral time synchronization with enablement and server/pool configuration capability checks.
- [ ] 9.10 Add mount runtime/persistent scopes, source/target/type/options/dump/pass fields, precise owned-entry removal, and protected-path preflight.
- [ ] 9.11 Add swap file/device active/persistent lifecycle, safe file creation, priority, and removal controls.
- [ ] 9.12 Add the M3 integration baseline proving supported Debian/Ubuntu and selected RPM-family hosts can be expressed without generic commands for these capabilities.

## 10. Deliver M4 Endpoint Schedules and Services

- [ ] 10.1 Add persistent endpoint-schedule models and validation distinct from existing server-dispatched cron models and APIs.
- [ ] 10.2 Implement cron/cron.d provider lifecycle with stable markers, user, argv/shell mode, working directory, environment references, timeout, and overlap policy.
- [ ] 10.3 Implement systemd timer/service paired-unit lifecycle, syntax validation, persistence/missed-run policy, daemon reload, and enablement.
- [ ] 10.4 Report schedule configuration compliance separately from optional execution-history telemetry and document offline guarantees.
- [ ] 10.5 Introduce provider-neutral service state and adapt existing systemd/systemd-user behavior without regressing enable/active/mask/linger semantics.
- [ ] 10.6 Add first-class systemd unit/drop-in content, validation, atomic replacement, absence, and activation signals.
- [ ] 10.7 Add provider capability contracts for OpenRC and SysV but advertise them only after full provider tests pass.
- [ ] 10.8 Add reusable service reload/restart/try-restart actions triggered by successful resource changes and verify coalescing/order.
- [ ] 10.9 Add reboot-required persistence and CLI/state-report visibility independent from reboot execution.
- [ ] 10.10 Add coordinated reboot intent, maintenance/inhibitor preflight, pre-reboot acknowledgement, durable attempt generation, boot-ID verification, timeout, and no-loop tests.

## 11. Deliver M4 Guarded Network Management

- [ ] 11.1 Add firewall individual-rule, owned-chain/zone, and authoritative-set ownership modes with bounded cleanup tests.
- [ ] 11.2 Add firewall transaction planning plus timed rollback/acknowledgement that protects resolved Remotr destinations, routes, DNS, ports, and established control traffic.
- [ ] 11.3 Add structured hosts-entry lifecycle that preserves unrelated `/etc/hosts` content.
- [ ] 11.4 Add separate DNS resolver/search-domain and route resource contracts with configured versus effective state.
- [ ] 11.5 Add NetworkManager profile audit/report provider with unambiguous interface matching and redacted credential references.
- [ ] 11.6 Add enforced NetworkManager activation only with checkpoints, timed rollback, explicit authorization, and server acknowledgement.
- [ ] 11.7 Add netplan/systemd-networkd providers only after they provide safety and reporting equivalent to the NetworkManager contract.
- [ ] 11.8 Run VM tests that intentionally break routes, DNS, firewall, and profiles and prove control-path recovery.

## 12. Deliver M5 Security and Secret Management

- [ ] 12.1 Implement the selected endpoint-scoped secret provider and protected retrieval API/path with reference-only Git validation.
- [ ] 12.2 Add certificate/key pair lifecycle, matching validation, safe fingerprint/expiry/renewal state, permissions, activation, and protected rollback.
- [ ] 12.3 Add named CA trust anchors with fingerprint verification, provider directories, absence, and coalesced trust-store refresh.
- [ ] 12.4 Add SELinux global mode, boolean, context, port, module, user, and login providers for the supported RPM-family matrix.
- [ ] 12.5 Add AppArmor profile content and enforce/complain/disabled lifecycle with staged parser validation for supported Ubuntu-family endpoints.
- [ ] 12.6 Add audit rule fragments, effective-ruleset validation, loading state, immutable-mode detection, and reboot-required reporting.
- [ ] 12.7 Add named account-limit fragments with logout-required reporting.
- [ ] 12.8 Add distro-aware PAM/login-policy providers only after full-stack validation and recovery-path tests exist.
- [ ] 12.9 Add structured journald policy with storage, retention, disk, rate, forwarding, validation, and activation.
- [ ] 12.10 Add structured logrotate fragments with path/cadence/retention/compression/create/script fields and full-config validation.
- [ ] 12.11 Run secret-canary and access-recovery integration suites across Apply, failure, rollback, diagnostics, sync, storage, API, and CLI paths.

## 13. Deliver M5 Interactive User Policy

- [ ] 13.1 Generalize interactive-user selection to documented all-interactive and explicit-user modes with unresolved-target reporting.
- [ ] 13.2 Add bounded per-user structured subresults while preserving safe ownership and no-follow home traversal.
- [ ] 13.3 Add dconf/GSettings provider with typed values, mandatory/locked scope, logged-out persistence, and native-type checks.
- [ ] 13.4 Add structured session policy for lock/idle settings, proxy, login/session restrictions, and default applications where providers support them.
- [ ] 13.5 Add Chromium-family and Firefox managed browser policy providers with typed values, mandatory/recommended scope, presence, and capability validation.
- [ ] 13.6 Compose browser/desktop trust policy through certificate resources without copying private material.
- [ ] 13.7 Add logout/application-restart activation reporting without implicit session termination.
- [ ] 13.8 Add authoritative versus merge cleanup semantics for users leaving a policy selector.
- [ ] 13.9 Run logged-in/logged-out multi-user integration tests including malicious home symlinks and one-user failure aggregation.

## 14. Gate M6 Optional Breadth and Complete the Umbrella Change

- [ ] 14.1 If accepted by the M6 demand review, add atomic archive deployment with checksum/signature, safe extraction, ownership, retention, and traversal tests; otherwise record deferral.
- [ ] 14.2 If accepted, add revision-pinned VCS deployment with credential references, clean/dirty policy, and atomic activation; otherwise record deferral.
- [ ] 14.3 If accepted, add approved destructive storage resources with stable device identity, inventory preconditions, audit plan, maintenance authorization, and VM recovery tests; otherwise record deferral.
- [ ] 14.4 If accepted, add provider-backed container lifecycle, digest policy, secret registry auth, data ownership, health, and replacement tests; otherwise record deferral.
- [ ] 14.5 If accepted, add alternatives, Linux file capabilities, environment fragments, and transient-path resources with full common-contract tests; otherwise record deferral.
- [ ] 14.6 Run full unit, contract, integration, VM safety, migration, mixed-version, and documentation validation across every advertised capability.
- [ ] 14.7 Verify the M1–M5 exit criteria against real composed repositories and produce a gap report for every unsupported or deferred field/provider.
- [ ] 14.8 Update the original gap-analysis roadmap with delivered capability links, measured fleet demand, remaining gaps, and any reprioritization.
- [ ] 14.9 Remove no legacy input or compatibility behavior until its separately approved breaking-change criteria and fleet-usage threshold are satisfied.
- [ ] 14.10 Archive the umbrella OpenSpec change only after all non-optional requirements are implemented or explicitly descoped through an approved OpenSpec update.
