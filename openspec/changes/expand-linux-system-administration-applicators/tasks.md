## 1. Resolve Foundation Decisions

- [x] 1.1 Use `schemaVersion: 1`, treat unversioned artifacts as schema `0`, and retain schema `0` for at least two minor releases and 90 days; decision and migration example recorded.
- [x] 1.2 Use a current-Sync endpoint capability document, conservative legacy fallback, bounded lossless schema variants, and visible per-endpoint capability blocking without freezing global Release ref advancement.
- [x] 1.3 Use centralized root-owned encrypted rollback storage with count, age, and disk bounds; short-lived secret payloads; retained Secret-version references; TPM preference; and explicit transactional/best-effort/none capability.
- [x] 1.4 Use fleet-bounded Change requests, configurable distinct-operator approvals, rollout/baseline authorization, endpoint Execution leases, risk-specific acknowledgement, and tightly scoped audited break glass.
- [x] 1.5 Defer Fedora/RHEL, DNF4/DNF5, RPM repositories, image-based RPM systems, and dependent SELinux/authselect providers to a future OpenSpec change; remove current DNF advertisement in M1.
- [x] 1.6 Provide local-file and encrypted Remotr providers, external KEK envelope encryption, exact endpoint/resource/purpose authorization, provider extension interfaces, and explicit pinned/active version rollout semantics.
- [x] 1.7 Defer every M6 provider to a demand-backed child OpenSpec change; retain contracts as roadmap guidance without affecting M1–M5 completion.
- [ ] 1.8 Complete and accept the `establish-testing-and-performance-foundation` change—including traceability, selective Godog, TDD/CI gates, provider conformance, fuzz/mutation decision, and initial controlled performance budgets—before beginning tasks 2–13.

## 2. Build the Applicator Execution Contract

- [x] 2.1 Add typed check statuses, reason codes, redacted desired/observed summaries, and contract tests for exhaustive status handling.
- [x] 2.2 Replace or adapt `executor.Handler.State(any, bool)` with structured Check while keeping current handlers behaviorally compatible during migration.
- [x] 2.3 Add structured Apply results for changed state, activation signals, reboot requirement, deferred work, rollback class, and diagnostics.
- [x] 2.4 Update executor failure handling so the original apply error and separate rollback outcome are both retained and tested.
- [x] 2.5 Prevent Apply for compliant, unsupported, check-failed, deferred, report-only, dependency-blocked, or failed-preflight resources.
- [x] 2.6 Add normal, sensitive, connectivity, access, boot, and destructive risk metadata with preflight hooks and safe default-policy tests.
- [x] 2.7 Implement exclusive lock domains with bounded provider-native lock waits and lock-contention tests.
- [x] 2.8 Implement activation collection, dependency-aware ordering, deduplication, and execution for daemon-reload, reload, restart, logout, next-boot, and reboot-required signals.
- [ ] 2.9 Add protected transaction metadata/payload storage keyed by resource address, artifact digest, and attempt, including count/age/disk bounds, atomic checksummed writes, encryption, TPM or root-key protection, reservation, and cleanup.
- [ ] 2.10 Add schema-driven sensitivity classification and prove via tests that secret values cannot enter logs, reports, diagnostics, or generic backups.
- [ ] 2.11 Compute stable desired-state hashes and non-enforcing high-risk preflight plans that normal dependency processing can block or bypass correctly.

## 3. Version and Register Desired-State Resources

- [x] 3.1 Add strict canonical artifact decoding with schema-version validation, unknown-field rejection, and precise resource-address diagnostics.
- [x] 3.2 Define canonical shared metadata for kind, name, lifecycle, dependencies, provider options, policy, ownership, validation, notifications, risk overrides, and authorization group.
- [x] 3.3 Implement a resource registry covering decode, validate, sensitivity, risk, provider factory, ordering tier, and lock domains.
- [x] 3.4 Refactor resolver and engine node construction to iterate registered resources and verify that no resource collection is dropped during resolution.
- [x] 3.5 Enforce configuration-wide cross-kind name uniqueness and validate dependency existence/cycles against stable addresses.
- [x] 3.6 Add provider registry and normalized facts for distro family/version, init, package, firewall, network, security, and desktop backends.
- [x] 3.7 Implement capability-matrix validation that rejects statically impossible target/provider/field combinations and returns runtime unsupported for local mismatches.
- [x] 3.8 Add legacy plural-collection compatibility decoding, canonical rendering, deprecation diagnostics, and golden migration fixtures.
- [x] 3.9 Update `config discover`, `validate`, and `render` to understand canonical kinds and capability requirements without writing composed artifacts to source repos.
- [x] 3.10 Update the configuration reference and examples only for resource fields whose vertical implementation slices are complete.

## 4. Expand Compliance Telemetry and Operator Reporting

- [x] 4.1 Version drift/apply telemetry additively and encode all structured statuses, reason codes, provider identity, activation, rollback, and redacted summaries.
- [x] 4.2 Update server persistence and endpoint/fleet state-report models for compliant, drifted, unsupported, check-failed, deferred, apply-failed, and no-report buckets.
- [x] 4.3 Update operator CLI state-report output and JSON contracts to expose the new buckets and bounded per-resource diagnostics.
- [x] 4.4 Preserve digest-based unchanged suppression and add payload-size bounds for expanded reports.
- [x] 4.5 Add mixed-version server/agent compatibility tests covering legacy reports, new reports, and agent downgrade during the schema window.
- [x] 4.6 Add redaction integration tests that trace secret-like canaries from desired state through agent logs, sync payloads, Postgres, APIs, and CLI output.
- [x] 4.7 Add server-registry Change requests with fleet-bounded explicit/dependency grouping, frozen rollout targets, resource hashes, risk/preflight evidence, authorization state, and audit history.
- [x] 4.8 Add validity-bounded Rollout authorizations with recurring execution windows and durable hash-bound Fleet baseline authorizations, including baseline eligibility and invalidation tests.
- [x] 4.9 Add Admin CLI list/show/authorize/watch/pause/resume/revoke and baseline-adoption workflows with human and JSON output.
- [x] 4.10 Add global and Fleet/risk Approval policies, distinct-operator counting, RBAC enforcement, multi-approval state, and persistent single-operator destructive-policy warnings.
- [x] 4.11 Add frozen-target outcome accounting, exception acknowledgement, manual baseline promotion defaults, and explicitly configured canary/evidence/failure gates for automatic promotion.
- [x] 4.12 Add endpoint-specific Execution lease scheduling with windows, concurrency, attempts, expiry, pause/revoke behavior, and authenticated Sync delivery.
- [x] 4.13 Add risk-specific progress and acknowledgement state, including network watchdog rollback, access canary gates, reboot boot-ID verification, and irreversible-storage postconditions.
- [x] 4.14 Add endpoint and fleet Break-glass authorization with dedicated RBAC, bounded scope/attempt/validity, non-bypassable safeguards, and prominent audit/SIEM events.

## 5. Establish Provider Test and Release Gates

- [x] 5.1 Map every umbrella scenario to immutable verification IDs and truthful planned/verified/deferred evidence in the foundation traceability manifest.
- [x] 5.2 Run every new or changed provider through the foundation conformance harness for compliant, drifted, apply, second-check idempotence, absence, unsupported, probe/check failure, validation failure, lock contention, cancellation, activation, redaction, and rollback cases.
- [x] 5.3 Extend the foundation's versioned Debian, Ubuntu, and Arch provider matrix and container jobs for package, filesystem, identity, service, and repository providers as each lands.
- [x] 5.4 Add required network rollback, reboot acknowledgement, mounts, kernel modules/sysctl, MAC policy, authentication recovery, and destructive-safety cases to the isolated foundation VM harness.
- [x] 5.5 Add risk-appropriate negative recovery evidence for Remotr connectivity loss, SSH/sudo lockout, boot-risk changes, secret leakage, and ambiguous/destructive devices.
- [x] 5.6 Require passing schema, validation, composition, provider, engine, telemetry, traceability, documentation, migration, integration, safety, mutation, and performance evidence before advertising any new field or provider.

## 6. Deliver M1 Package Truthful Convergence

- [x] 6.1 Replace package `present` boolean ambiguity with canonical present/absent/provider-supported-purged lifecycle and compatibility mapping.
- [x] 6.2 Implement native installed-version observation and exact-version convergence for APT, including explicit upgrade/downgrade policy and unavailable-version errors.
- [x] 6.3 Implement native installed-version observation and exact-version convergence for Pacman with the same policy/result contract.
- [x] 6.4 Separate Pacman and Yay/AUR providers; implement and test a truthful Yay provider or reject Yay as unsupported.
- [x] 6.5 Implement hold/pin, cache refresh, dependency-removal, and noninteractive transaction fields only for providers that can check and apply them.
- [x] 6.6 Remove DNF from the advertised schema/provider matrix, reject authored DNF configuration with a roadmap diagnostic, and remove or quarantine the non-applying stub.
- [x] 6.7 Serialize package transactions, honor native locks/timeouts, sanitize environments, and return bounded provider diagnostics.
- [x] 6.8 Detect and report service/reboot activation requirements from package transactions without implicit reboot.
- [x] 6.9 Add migration, validation, unit, integration, and idempotence coverage proving every advertised package field converges.

## 7. Deliver M1 Filesystem, Download, User, and Firewall Correctness

- [x] 7.1 Make system-file and user-file Check observe managed mode, owner, and group independently from content and repair metadata-only drift.
- [x] 7.2 Add explicit file present/absent lifecycle and safe removal while retaining compatible whole-file and line-regex behavior.
- [x] 7.3 Replace direct truncating writes with validated same-filesystem staging, fsync policy, atomic rename, and prior-state preservation.
- [x] 7.4 Extend no-follow safe traversal to system filesystem objects and add traversal/symlink race tests.
- [x] 7.5 Expand remote-file resources with lifecycle, checksum/signature verification, trusted signer, secret authentication reference, redirect/timeout policy, and atomic activation.
- [x] 7.6 Migrate download `notifySystemd` and `reloadExec` behavior to shared structured activation signals with compatibility fixtures.
- [x] 7.7 Make user Check and Apply enforce requested UID with explicit safe reassignment behavior and account-database locking.
- [x] 7.8 Reject all user fields that are not yet supported by Check and Apply instead of parsing them early.
- [x] 7.9 Add firewall present/absent lifecycle to firewalld and nftables with stable managed-rule identities and removal tests.
- [x] 7.10 Replace firewall audit-log-as-compliance behavior with a structured plan/status while preserving audit-by-default.
- [x] 7.11 Fix resource resolution and engine coverage tests so firewall and every registered current kind survive parse, compose, resolve, order, check, and report.
- [x] 7.12 Add the M1 acceptance test proving every advertised package/file/download/user/firewall field is enforced or rejected.

## 8. Deliver M2 Filesystem and Local Access Baseline

- [x] 8.1 Add canonical directory, symbolic-link, and hard-link resources with lifecycle, type replacement policy, ownership, mode, and safe traversal.
- [x] 8.2 Add bounded recursive directory policy with explicit purge, cross-filesystem behavior, exclusions, and authoritative ownership tests.
- [x] 8.3 Add group presence/GID/system-class provider and serialize it with all account-database changes.
- [x] 8.4 Expand user providers for primary group, merge/authoritative supplementary groups, home policy, shell, comment, and system-account class.
- [x] 8.5 Add password reference, lock/unlock, expiry, and safe removal fields with protected-account and Remotr-runtime safeguards.
- [x] 8.6 Add structured authorized-key entries with restrictions, fingerprint, merge/authoritative modes, safe home traversal, and revocation tests.
- [x] 8.7 Add structured known-host entries with fingerprint, hashing policy, merge-safe editing, and replacement controls.
- [x] 8.8 Add named sudo fragments with complete effective `visudo` validation, atomic activation, rollback disclosure, and recovery-principal preflight.
- [x] 8.9 Add the M2 integration flow that provisions and revokes a local administrator without generic commands or unmanaged passwd/SSH/sudo edits.
- [ ] 8.10 Publish M2 canonical YAML, compatibility guidance, provider matrix, and access-lockout recovery documentation.

## 9. Deliver M3 Repositories and Host Baseline

- [ ] 9.1 Add APT signing-key resources with declared fingerprints, scoped keyrings, atomic install/removal, and mismatch rejection.
- [ ] 9.2 Add APT repository resources with named fragments, enable/disable/absent lifecycle, architectures, suites, components, priority, and credential references.
- [ ] 9.3 Order signing keys, repositories, coalesced cache refresh, and dependent package resolution through explicit dependencies.
- [x] 9.4 Defer DNF repository/key support to the future RPM-family OpenSpec change.
- [ ] 9.5 Add sysctl runtime/persistent scopes, Remotr-owned drop-ins, single-key/reload/next-boot activation, and unsupported-key results.
- [ ] 9.6 Add kernel-module loaded/persistent/parameters/blacklist state with network/root/boot protection for unload and blacklist.
- [ ] 9.7 Add static/transient hostname management separately from hosts-entry ownership.
- [ ] 9.8 Add independently optional timezone, locale, and keymap fields with logout/reboot activation reporting.
- [ ] 9.9 Add provider-neutral time synchronization with enablement and server/pool configuration capability checks.
- [ ] 9.10 Add mount runtime/persistent scopes, source/target/type/options/dump/pass fields, precise owned-entry removal, and protected-path preflight.
- [ ] 9.11 Add swap file/device active/persistent lifecycle, safe file creation, priority, and removal controls.
- [ ] 9.12 Add the M3 integration baseline proving supported Debian/Ubuntu and Arch hosts can be expressed without generic commands for these capabilities.

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
- [ ] 12.2 Implement per-secret-version DEK envelope encryption, AES-256-GCM record formats, an external versioned KEK keyring, and fail-closed startup/recovery diagnostics.
- [ ] 12.3 Implement routine DEK rewrap, compromise full rekey, referenced-key removal protection, and key-coverage backup/restore checks.
- [ ] 12.4 Add a key-encryption-provider interface and contract tests for static-key and future KMS/HSM wrappers.
- [ ] 12.5 Add explicit pinned/active Secret reference validation, inactive upload, audited activation rollout, effective version hashing, and honest revocation reporting.
- [ ] 12.6 Add certificate/key pair lifecycle, matching validation, safe fingerprint/expiry/renewal state, permissions, activation, and protected rollback.
- [ ] 12.7 Add named CA trust anchors with fingerprint verification, provider directories, absence, and coalesced trust-store refresh.
- [x] 12.8 Defer SELinux providers to the future RPM-family OpenSpec change; keep the provider contract as roadmap guidance only.
- [ ] 12.9 Add AppArmor profile content and enforce/complain/disabled lifecycle with staged parser validation for supported Ubuntu-family endpoints.
- [ ] 12.10 Add audit rule fragments, effective-ruleset validation, loading state, immutable-mode detection, and reboot-required reporting.
- [ ] 12.11 Add named account-limit fragments with logout-required reporting.
- [ ] 12.12 Add Debian/Ubuntu PAM/login-policy providers only after full-stack validation and recovery-path tests exist; defer authselect to the RPM-family change.
- [ ] 12.13 Add structured journald policy with storage, retention, disk, rate, forwarding, validation, and activation.
- [ ] 12.14 Add structured logrotate fragments with path/cadence/retention/compression/create/script fields and full-config validation.
- [ ] 12.15 Run secret-canary and access-recovery integration suites across Apply, failure, rollback, diagnostics, sync, storage, API, and CLI paths.
- [ ] 12.16 Add prior Secret-version rollback references, referenced-version deletion protection, authorized abandonment, and short-lived encrypted offline recovery payload tests.

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

- [x] 14.1 Defer archive deployment to a demand-backed child OpenSpec change.
- [x] 14.2 Defer revision-pinned VCS deployment to a demand-backed child OpenSpec change.
- [x] 14.3 Defer destructive storage providers to a demand-backed child OpenSpec change.
- [x] 14.4 Defer container workload providers to a demand-backed child OpenSpec change.
- [x] 14.5 Defer alternatives, Linux file capabilities, environment fragments, and transient-path providers to demand-backed child OpenSpec changes.
- [ ] 14.6 Run full unit, contract, integration, VM safety, migration, mixed-version, and documentation validation across every advertised capability.
- [ ] 14.7 Verify the M1–M5 exit criteria against real composed repositories and produce a gap report for every unsupported or deferred field/provider.
- [ ] 14.8 Update the original gap-analysis roadmap with delivered capability links, measured fleet demand, remaining gaps, and any reprioritization.
- [ ] 14.9 Remove no legacy input or compatibility behavior until its separately approved breaking-change criteria and fleet-usage threshold are satisfied.
- [ ] 14.10 Archive the umbrella OpenSpec change only after all non-optional requirements are implemented or explicitly descoped through an approved OpenSpec update.
