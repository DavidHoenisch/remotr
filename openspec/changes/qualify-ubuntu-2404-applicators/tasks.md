## 1. Establish the Qualification Inventory and Traceability

- [x] 1.1 Register OS-AEC-093 through OS-AEC-103 in `test/traceability.yaml` and link them to configuration CLI, provider-contract, container, VM safety/recovery, capability-advertisement, and audit selectors.
- [ ] 1.2 Define the versioned Ubuntu qualification-manifest schema with exact capability ID, backend, contract revision, Ubuntu 24.04 amd64 tuple, environment, risk, accepted fields, composed address, governing IDs, selectors, disposition, and reason.
- [ ] 1.3 Populate the manifest from the current resource/provider registry with every non-package Ubuntu-targetable contract marked `blocked` or `unadvertised`; reference `complete-core-package-providers` for package/repository rows rather than duplicating them.
- [ ] 1.4 For OS-AEC-096, write and observe a red registry/manifest omission test, then implement completeness validation that rejects missing, duplicate, broad-only, stale-revision, or unknown contract rows.
- [ ] 1.5 Record explicit non-qualification reasons for generic `command`, one-shot `bootstrap`, demand-specific `agentInstall`, legacy compatibility forms, deferred backends, and every future-roadmap capability so none can be mistaken for typed Ubuntu support.
- [ ] 1.6 Add a per-row TDD record requiring the governing verification ID, approved public seam, independently known expected result, selected evidence layers, observed red failure, green result, broader checks, and final disposition before production code changes.

## 2. Add Public Ubuntu M1–M5 Composition Proof

- [ ] 2.1 Create `test/config-repos/ubuntu-2404-m1-m5/` with a fleet manifest and schema-1 modules organized by milestone/risk domain, deterministic test-only values, and no deployable placeholders or real secrets.
- [ ] 2.2 Add representative existing M1 file/download/user/firewall resources while leaving package/repository examples linked to the sibling package change.
- [ ] 2.3 Add representative M2 directory/link/group/user/authorized-key/known-host/sudo/user-file resources and their stable dependency graph.
- [ ] 2.4 Add representative M3 sysctl/kernel-module/hostname/host-locale/time-sync/mount/swap resources with high-risk enforcement disabled by default.
- [ ] 2.5 Add representative M4 endpoint-schedule/service/systemd-unit/reboot/hosts/DNS/route/firewall/network-profile resources with high-risk enforcement disabled by default.
- [ ] 2.6 Add representative M5 certificate/trust/AppArmor/audit/limits/login-policy/journald/logrotate/desktop/session/browser resources using only test references and inert policy values.
- [ ] 2.7 For OS-AEC-095, write and observe a red public CLI acceptance test, then validate, discover, and render the repository and semantically assert every expected address, field, dependency, ownership, policy, capability requirement, and activation signal.
- [ ] 2.8 Add deterministic repeated-render and repository-source guards proving generated `desired.yaml` and `crons.yaml` are neither required nor committed.
- [ ] 2.9 Add validation proving all access, connectivity, boot, destructive, and guarded sensitive examples remain report-only/non-enforcing unless an isolated qualification fixture supplies explicit authorization.

## 3. Make Provider Rows Exact and Executable

- [ ] 3.1 Extend matrix validation and claim matching to require exact capability ID, provider/backend, contract revision, Ubuntu 24.04, amd64, environment, and executable selectors.
- [ ] 3.2 For OS-AEC-093, write and observe red missing/untested/planned/skipped/failing/stale-revision row cases, then fail capability advertisement for every incomplete exact tuple.
- [ ] 3.3 For OS-AEC-094, write and observe a red broad-family overclaim case, then prevent one filesystem, identity, service, security, network, or desktop row from authorizing sibling contracts.
- [ ] 3.4 Split the existing Ubuntu discovery rows into exact non-advertising contract/backend rows and retain `untested` until each row's complete provider and environment evidence passes.
- [ ] 3.5 For OS-AEC-102, prove a passing row is recorded but remains unadvertised while its execution-contract, capability-delivery, testing-foundation, or package dependency is incomplete.
- [ ] 3.6 Validate that every advertised capability document entry resolves to one matching exact passing row and every exact row's selectors are runnable from a clean environment.

## 4. Qualify M1 Ordinary Filesystem and Download Behavior

- [ ] 4.1 For the `file` POSIX row, record the public provider seam/evidence, observe a focused red real-ownership or lifecycle case, correct only the exposed behavior, and pass content, metadata-only drift, absence, atomic replacement, no-follow traversal, validation, preservation, and second-Check evidence in Ubuntu 24.04.
- [ ] 4.2 For `download`, record the seam/evidence, observe a focused red checksum/signature/authentication/redirect or atomic-activation case, correct only the exposed behavior, and pass failure, redaction, cleanup, activation, and second-Check evidence in Ubuntu 24.04.
- [ ] 4.3 Add OS-AEC-097 coverage proving the exact ordinary POSIX rows pass their applicable provider-contract cases in the pinned Ubuntu container without implying access, service, or VM-qualified behavior.
- [ ] 4.4 Promote only the exact passing file/download rows and keep any unsupported accepted field represented as a blocked field/provider combination.

## 5. Qualify M2 Filesystem, Identity, and Access Contracts

- [ ] 5.1 Qualify `directory` on Ubuntu 24.04 for lifecycle, metadata, bounded authoritative cleanup, exclusion, cross-filesystem policy, no-follow traversal, failure recovery, and second Check.
- [ ] 5.2 Qualify symbolic and hard `link` behavior for lifecycle, type replacement policy, target identity, safe traversal, preservation, and second Check.
- [ ] 5.3 Qualify `group` and `user` through the Ubuntu VM account-database seam for UID/GID, class, primary/supplementary groups, home, shell, password-reference, lock, expiry, removal, native locking, protected identities, recovery, and second Check.
- [ ] 5.4 Qualify `authorizedKey` through Ubuntu VM access recovery for fingerprint, restrictions, merge/authoritative ownership, revocation, malicious-home symlink rejection, recovery-principal protection, and second Check.
- [ ] 5.5 Qualify `knownHost` for fingerprint, hashing policy, merge-safe editing, replacement, absence, preservation, and second Check at the public provider seam.
- [ ] 5.6 Qualify `sudo` through staged `visudo` validation and Ubuntu VM access recovery for compliant, drifted, Apply, second Check, invalid effective configuration, rollback, protected recovery principal, and redacted failure.
- [ ] 5.7 Qualify `userFile` for interactive-user resolution, per-user subresults, lifecycle, metadata, no-follow home traversal, one-user failure isolation, preservation, and second Check.
- [ ] 5.8 For every correction in 5.1–5.7, record the governing ID/seam/evidence and observed focused red failure before implementation; do not batch provider changes or substitute private-helper assertions.
- [ ] 5.9 For OS-AEC-098, prove identity/access rows remain blocked when container/unit coverage passes but the required Ubuntu VM login/access recovery selector is absent.

## 6. Qualify M3 Host, Kernel, and Storage Contracts

- [ ] 6.1 Qualify `sysctl` for independently managed runtime/persistent state, owned drop-ins, unsupported keys, reload/next-boot activation, preservation, and second Check in Ubuntu 24.04.
- [ ] 6.2 Qualify `kernelModule` in the Ubuntu VM for loaded/persistent/parameter/blacklist state, protected-device and network/root preflight, next-boot reporting, failed activation recovery, and second Check.
- [ ] 6.3 Qualify `hostname` for independent static/transient state, provider mismatch, activation outcome, preservation, and second Check.
- [ ] 6.4 Qualify `hostLocale` in the Ubuntu VM for independent timezone/locale/keymap omission semantics, native validation, logout/reboot activation, failure preservation, and second Check.
- [ ] 6.5 Qualify `timeSync` for the truthful `systemd-timesyncd` backend only, including enablement, server/pool configuration, configured/effective state, unavailable backend, activation, and second Check.
- [ ] 6.6 Qualify `mount` in the Ubuntu VM for runtime/persistent scopes, precise owned fstab entry, protected target/source preflight, force policy, failed-mount recovery, boot safety, and second Check.
- [ ] 6.7 Qualify `swap` in the Ubuntu VM for file/device identity, active/persistent lifecycle, priority, safe file creation/removal, protected capacity preflight, failure recovery, and second Check.
- [ ] 6.8 For every correction in 6.1–6.7, record the governing ID/seam/evidence and observed focused red failure before implementation, then run the applicable boot/storage VM recovery fixture.

## 7. Qualify M4 Schedules, Services, and Reboot

- [ ] 7.1 Qualify `endpointSchedule` cron/cron.d behavior for lifecycle, stable ownership, user, argv/shell mode, working directory, environment references, timeout/overlap policy, offline persistence, and second Check.
- [ ] 7.2 Qualify the `endpointSchedule` systemd-timer backend in the Ubuntu VM for paired-unit lifecycle, syntax validation, daemon reload, enablement, missed-run policy, removal, and second Check.
- [ ] 7.3 Qualify provider-neutral `service` on systemd for enablement, active state, masking, activation outcome, failure, preservation, and second Check; do not advertise OpenRC or SysV.
- [ ] 7.4 Audit legacy `systemd` and `systemdUser` compatibility forms separately and either prove their declared compatibility contract or record them unadvertised without allowing them to broaden the provider-neutral service claim.
- [ ] 7.5 Qualify `systemdUnit` for complete unit/drop-in lifecycle, staged syntax validation, atomic activation, daemon-reload/restart coalescing, failure preservation, removal, and second Check.
- [ ] 7.6 Qualify `reboot` in the Ubuntu VM for intent persistence, maintenance/inhibitor preflight, pre-reboot acknowledgement, durable attempt generation, boot-ID verification, timeout, no-loop behavior, and observable failure.
- [ ] 7.7 For every correction in 7.1–7.6, record the governing ID/seam/evidence and observed focused red failure before implementation; require actual Ubuntu systemd/boot evidence rather than container process mocks.

## 8. Qualify M4 Guarded Network Management

- [ ] 8.1 Qualify `hostsEntry` for stable marked lifecycle, preservation of unrelated `/etc/hosts` content, configured/effective observation, connectivity planning, and second Check.
- [ ] 8.2 Qualify `dnsResolver` for the exact NetworkManager backend, configured/effective state, ownership, unsupported backend, control-path preflight, checkpoint rollback, acknowledgement, and second Check.
- [ ] 8.3 Qualify `route` for the exact NetworkManager backend, stable route identity, configured/effective state, protected Remotr paths, checkpoint rollback, acknowledgement, and second Check.
- [ ] 8.4 Qualify `networkProfile` only for Ubuntu backends and fields whose complete activation/recovery contract passes; keep credential-bearing file-backed Netplan/networkd enforcement and unsupported networkd device classes unadvertised.
- [ ] 8.5 Qualify nftables `firewall` audit and guarded enforcement separately for named-rule/chain/zone/set ownership, bounded cleanup, protected destinations/ports, timed rollback, authenticated acknowledgement, and second Check; keep firewalld enforcement, UFW, and iptables unadvertised.
- [ ] 8.6 Run Ubuntu VM fixtures that intentionally break routes, DNS, firewall, and profiles and prove independent local rollback plus recovered authenticated Sync before promoting any enforcement row.
- [ ] 8.7 For every correction in 8.1–8.5, record the governing ID/seam/evidence and observed focused red failure before implementation; retain planned traceability for any recovery path that does not pass.

## 9. Qualify M5 Secrets, Security, Authentication, and Logging

- [ ] 9.1 Qualify `certificate` for matching certificate/private-key references, fingerprint/expiry/renewal observation, permissions, transactional activation, protected rollback, secret-canary safety, cleanup, and second Check.
- [ ] 9.2 Qualify `trustAnchor` for Ubuntu trust directories, fingerprint verification, lifecycle, coalesced trust-store refresh, preservation, failure recovery, and second Check.
- [ ] 9.3 Qualify `appArmorProfile` in the Ubuntu VM for staged parser validation, enforce/complain/disabled lifecycle, loaded/effective state, invalid-profile recovery, preservation, and second Check.
- [ ] 9.4 Qualify `auditRules` in the Ubuntu VM for structured fragment lifecycle, complete effective-ruleset validation, load state, immutable-mode handling, reboot reporting, failure recovery, and second Check.
- [ ] 9.5 Qualify `accountLimit` through the Ubuntu access/session seam for typed fragment lifecycle, full configuration validation, logout activation, preservation, recovery-principal behavior, and second Check.
- [ ] 9.6 Qualify `loginPolicy` through Ubuntu PAM and VM access recovery for password/history/lockout/last-login fields, complete stack validation, activation, protected principals, failed-login recovery, rollback, and second Check.
- [ ] 9.7 Qualify `journald` in the Ubuntu VM for storage/retention/disk/rate/local-forwarding fields, staged validation, reload/restart behavior, secret safety, failure preservation, and second Check without claiming remote delivery health.
- [ ] 9.8 Qualify `logrotate` for path/cadence/retention/compression/create/script fields, full-config validation, lifecycle, secret-safe diagnostics, failure preservation, and second Check.
- [ ] 9.9 For OS-AEC-099, run secret-canary evidence across desired state, agent output, argv, rollback data, Sync, persistence, API, CLI, and cleanup for every secret-bearing qualified row.
- [ ] 9.10 For every correction in 9.1–9.8, record the governing ID/seam/evidence and observed focused red failure before implementation; require Ubuntu VM evidence wherever kernel, PAM, effective service, or recovery state matters.

## 10. Qualify M5 Desktop, Session, and Browser Policy

- [ ] 10.1 Build a reproducible Ubuntu 24.04 desktop/session VM fixture with test interactive users, logged-in/logged-out execution, isolated homes, provider fact discovery, and snapshot recovery.
- [ ] 10.2 Qualify `desktopSetting` for every advertised schema/key/native type, mandatory/locked scope, logged-out persistence, lifecycle/cleanup, malicious-home symlink rejection, one-user failure aggregation, and second Check.
- [ ] 10.3 Qualify `sessionPolicy` only for supported lock/idle/proxy/restriction/default-application fields, merge-only semantics where required, logout/application activation, multi-user behavior, cleanup, and second Check.
- [ ] 10.4 Qualify `browserPolicy` separately for Chromium, Chrome, and Firefox paths, policy allowlist, native type, mandatory/recommended level where supported, lifecycle, unrelated-policy preservation, restart activation, and second Check.
- [ ] 10.5 Keep Firefox recommended policy, user-scope browser policy, Edge/other browsers, unknown policy names/types/levels, and authoritative default-application cleanup unadvertised with explicit roadmap dispositions.
- [ ] 10.6 Audit `systemdUser` and interactive-user selection behavior used by desktop/session resources without inferring desktop qualification from static file output alone.
- [ ] 10.7 For every correction in 10.2–10.6, record the governing ID/seam/evidence and observed focused red failure before implementation; require actual logged-in and logged-out Ubuntu VM evidence.

## 11. Reconcile Failures Without Weakening Requirements

- [ ] 11.1 For OS-AEC-100, add a harness regression that demonstrates a real Ubuntu fixture contradicting a focused test, then enforce that the exact row remains blocked until a focused public-seam red-green correction and required broader evidence pass.
- [ ] 11.2 Commit every new fuzz crash as a seed regression and run bounded parser/schema properties for all qualification-exposed inputs.
- [ ] 11.3 Record any unavoidable skip, quarantine, manual evidence, or equivalent mutant only as a reviewed expiring entry in `test/evidence-exceptions.yaml`; prove it cannot by itself promote a row.
- [ ] 11.4 Run focused mutation campaigns for safety, redaction, provider selection, ownership, validation, activation, rollback, and advertisement decisions with no unexplained relevant survivor.
- [ ] 11.5 Update the qualification manifest, matrix selector, traceability disposition, and documentation together after each exact row passes; never promote a broad family in advance of its rows.

## 12. Run the Exit Audit and Close the Qualification Gate

- [ ] 12.1 Run public validate/discover/render acceptance, manifest completeness, exact provider matrix, provider contracts, Ubuntu containers, Ubuntu VM safety/recovery, desktop/session VM, traceability, evidence-exception, capability-advertisement, mutation, and documentation validation.
- [ ] 12.2 Run the narrowest focused command after every red-green slice, then the affected package/provider suite and `make test`; run the relevant VM/container target whenever selected evidence requires it.
- [ ] 12.3 For OS-AEC-101, write and observe a red audit fixture containing one blocked/planned/missing/skipped/failing/untested row, then generate milestone and umbrella decisions that preserve the exact blocker.
- [ ] 12.4 For OS-AEC-103, generate the positive audit only when every non-optional Ubuntu target is qualified or explicitly descoped through an approved OpenSpec update and every dependent workstream is accepted.
- [ ] 12.5 Refresh `engineering/testing/applicator-m1-m5-gap-report.md` with exact composition coverage, qualified rows, blocked rows, unadvertised/deferred behavior, measured selectors, dependency status, and an evidence-derived archive decision.
- [ ] 12.6 Update the configuration reference and provider/support documentation to state only exact Ubuntu 24.04 amd64 capabilities whose rows pass and to link future CMMC/Hub gaps back to the roadmap.
- [ ] 12.7 Close umbrella task 14.10 only after this child, `complete-applicator-execution-contract`, `complete-capability-compatible-delivery`, `complete-core-package-providers`, and `establish-testing-and-performance-foundation` are accepted and the complete audit passes.
