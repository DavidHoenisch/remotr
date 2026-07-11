## Context

Remotr is a pull-based Linux MDM. The server composes kind-tagged Git configuration into an endpoint artifact; the agent parses that artifact, resolves it against local facts, builds a dependency-ordered list of handlers, checks state, optionally applies drift, and reports compliance on its next sync.

The current implementation is a useful skeleton but cannot support the roadmap safely without a contract change:

- `executor.Handler.State` returns `(any, bool)`, so probe errors, unsupported providers, and actual drift all collapse into `false`.
- Drift telemetry contains only address, name, and description; it cannot carry redacted actual/desired state, provider, reason, activation, or deferred maintenance.
- Several parsed fields are not convergent: native package versions are rejected or ignored, file mode is not checked, and user UID is not checked or applied.
- Provider discovery is narrow. Facts recognize Debian, Ubuntu, and Arch; `dnf` exists as a non-applying stub; `yay` routes to the pacman implementation.
- Applicators use inconsistent rollback and notification behavior. Many return `ErrNoOp`; file/download backups are local and resource-specific.
- Author-time validation covers only part of the schema, while runtime parsing accepts unknown YAML fields and can silently ignore intent.
- Resources are manually wired through the model, resolver, engine kind list, default tiers, validation, documentation, and telemetry. New breadth would multiply this coupling.
- High-risk resources can affect Remotr's own connection, login access, boot, privilege escalation, disks, or confidential material.

The source roadmap is `docs/plans/linux-system-administration-applicator-gaps.md`. This design turns it into a staged architecture. Operators, configuration authors, agent contributors, server/API contributors, security reviewers, and release engineers are stakeholders.

## Goals / Non-Goals

**Goals:**

- Make every accepted desired-state field either fully checked and applied, explicitly unmanaged when omitted, or rejected before release.
- Give all applicators one structured lifecycle and reporting contract.
- Separate portable resource intent from distro/backend providers.
- Make ownership, destructive behavior, validation, activation, rollback, redaction, and concurrency explicit.
- Preserve dependency ordering while supporting resource-specific locks and deferred lifecycle actions.
- Deliver the roadmap in independently releasable milestones with measurable exit criteria.
- Keep existing artifacts usable during a documented compatibility period.

**Non-Goals:**

- Reproduce the complete Ansible, Salt, Puppet, or Chef catalog.
- Support non-Linux endpoints in this change.
- Treat arbitrary commands as equivalent to first-class resources.
- Promise one implementation command across incompatible distributions.
- Implement cloud APIs, database/application orchestration, or network-device management.
- Enable destructive storage, network, reboot, or secret workflows before their safety prerequisites exist.
- Require every optional P2 provider before the core roadmap is complete.

## Decisions

### 1. Introduce a versioned canonical desired-state contract

Canonical composed artifacts will carry a schema version. Each resource has a stable kind, logical name, lifecycle state, resource-specific fields, and shared metadata. Omitted optional fields mean “unmanaged”; defaults are applied only when the specification names a default. Accepted fields must participate in validation and, when managed, in both Check and Apply.

Existing plural collections remain readable through a compatibility decoder during migration. Composition emits only the canonical form once its corresponding milestone is enabled. Unknown fields and invalid field combinations are rejected using strict decoding.

Why: Go zero values currently make omitted values indistinguishable from explicitly requested values, and permissive YAML parsing hides mistakes.

Alternative considered: extend the current structs indefinitely. Rejected because it preserves ambiguous booleans, manual wiring, and silent field loss.

### 2. Replace the boolean handler check with structured outcomes

The engine-facing contract will conceptually expose:

- resource identity and provider capabilities;
- `Check`, returning status (`compliant`, `drifted`, `unsupported`, `check_failed`, or `deferred`), redacted observed/desired summaries, and a stable reason code;
- `Apply`, returning changed state, activation signals, reboot requirement, rollback outcome, and redacted diagnostics;
- explicit rollback support (`transactional`, `best_effort`, or `none`).

`unsupported` and `check_failed` are never counted as ordinary drift or compliance. The engine will not call Apply for them. State reports aggregate them separately.

Why: a false boolean cannot safely drive auto-remediation or meaningful fleet reporting.

Alternative considered: retain `(any, bool)` and encode errors in `any`. Rejected because callers cannot enforce exhaustive handling.

### 3. Use a provider registry behind portable resource contracts

Resource contracts describe Linux intent; providers implement observation and mutation for a declared capability matrix. Selection uses normalized facts such as distro family/version, init system, package manager, firewall, network stack, security module, and desktop session support.

The initial guaranteed matrix is Debian/Ubuntu plus Arch for capabilities already working there. Fedora/RHEL becomes selectable only when facts and DNF are complete. Provider-specific fields live in an explicit provider options namespace and are rejected by other providers.

Author-time validation rejects impossible target/provider combinations known from the declared matrix. Runtime absence or mismatch is a structured `unsupported` result, not perpetual drift.

Why: portable intent is valuable, but pretending backends are interchangeable produces unsafe shell branching.

Alternative considered: one applicator with distro conditionals. Rejected because it mixes policy with execution and is difficult to test exhaustively.

### 4. Register resource kinds rather than hand-wire every engine path

A resource registry will associate each kind with decoding, validation, sensitivity, risk class, provider factory, default ordering tier, and lock domains. Resolution iterates registered resources and retains explicit `dependsOn` edges. Stable addresses remain `<configuration>/<resource-name>`; duplicate names within a configuration are rejected across all kinds so dependencies are unambiguous.

Why: the current model/resolver/engine/validator switches are easy to update inconsistently and already make omissions hard to spot.

Alternative considered: continue adding slices and loops. Rejected due to coupling and incomplete-path risk.

### 5. Make ownership and presence explicit

Resources declare whether they own a single named entry, a Remotr-owned fragment, or an authoritative set. Append/merge and authoritative behavior are never inferred. Presence is separate from configuration, normally `present` or `absent`; resources with richer lifecycle use a closed enum.

Remotr prefers named fragments/drop-ins under subsystem-supported directories. It does not rewrite distribution-owned monolithic files when a safe fragment mechanism exists.

Why: removal, revocation, and garbage collection are impossible to implement safely without an ownership boundary.

Alternative considered: infer ownership from filenames or resource omission. Rejected because omission must not unexpectedly delete state.

### 6. Centralize safety policy, planning, and activation

Every kind is classified as normal, sensitive, connectivity-risk, access-risk, boot-risk, or destructive. High-risk changes require a successful preflight and default to report/audit unless the resource explicitly enables enforcement. Network/firewall changes must preserve the active Remotr control path or use a timed rollback. Access-policy changes validate syntax and retain a recovery path. Boot/storage changes require maintenance coordination and post-change acknowledgement.

Applicators emit activation signals (`reload`, `restart`, `daemon-reload`, `logout-required`, `reboot-required`, or `next-boot`). The engine coalesces compatible signals and executes them after their producing resources and dependencies succeed. Immediate reboot is a separate coordinated resource, never an incidental package action.

Why: activation and risk are cross-cutting lifecycle concerns, not ad hoc fields on downloads or files.

Alternative considered: keep per-resource notify fields. Rejected because it duplicates behavior and cannot safely order or coalesce activation.

### 7. Use lock domains and durable transaction metadata

Resources declare lock domains such as package database, account database, firewall, network, storage, or reboot. The agent serializes conflicting operations and respects native provider locks/timeouts.

When rollback is supported, prior-state metadata is stored in an agent-owned root-only state directory with resource address, artifact digest, timestamp, sensitivity, and retention policy. Secret values are never placed in generic backups. A failed rollback is reported alongside the original apply failure rather than replacing it.

Why: package managers and host databases are shared mutable systems, and the current executor can obscure the primary error when revert fails.

Alternative considered: rely only on the engine's current sequential loop. Rejected because endpoint schedules and future parallel checks still need explicit exclusion, and provider processes may already hold locks.

### 8. Treat redaction as typed data-flow policy

Desired/observed fields are classified public, sensitive metadata, or secret at schema definition. Redaction happens before values enter logs, drift payloads, backups, diagnostics, or persistence. Reports include secret identifiers, versions/fingerprints where safe, and expiry/health, but never secret bytes, private keys, passwords, repository credentials, or command arguments containing resolved secrets.

Secret resources reference an external source by identifier. Git artifacts contain references only. Provider retrieval is pluggable; the first supported source may be a root-readable local file, but the contract does not expose its contents.

Why: string-scrubbing after logging is too late and incomplete.

Alternative considered: mark entire resources sensitive and omit all reporting. Rejected because operators still need safe compliance evidence.

### 9. Keep server jobs and endpoint schedules separate

Server-dispatched cron continues to mean “the server decides work is due and the next connected agent executes it.” Endpoint schedules manage persistent local cron entries or systemd timers that execute without server reachability. They use distinct schema kinds, reports, and guarantees.

Why: merging the models would misrepresent offline behavior and observability.

### 10. Stage delivery behind capability gates

The implementation order is:

1. Foundation: strict/versioned schema, structured execution/reporting, registry, provider facts, locks, redaction, and compatibility adapters.
2. M1 truthful convergence: package versions, file metadata/lifecycle, UID enforcement, distro/DNF consistency, firewall presence/removal.
3. M2 access baseline: groups/users, directories/links, authorized keys, sudo fragments.
4. M3 OS baseline: repositories/keys, sysctl/modules, host identity/time/locale, mounts/swap.
5. M4 durable operations: endpoint schedules, richer services, reboot coordination, network audit/apply.
6. M5 security/workstation: MAC/audit, certificates/secrets, logging, authentication/limits, desktop policy.
7. M6 optional breadth: archives/VCS, destructive storage, containers, alternatives/capabilities, only after demand and safety review.

Each gate includes schema, validation, composition, provider, engine wiring, unit tests, distro integration tests, telemetry, docs, migration examples, and feature advertisement. A field is not documented as supported until this vertical slice is complete.

Why: landing schemas far ahead of enforcement recreates the current trust gap.

Alternative considered: implement all schemas first and fill providers later. Rejected because accepted but unenforced configuration is the primary problem being fixed.

### 11. Test contracts and providers independently

Contract tests run every provider through compliant, drift, apply, idempotence, unsupported, probe-failure, validation-failure, lock-contention, redaction, and rollback cases. Provider integration tests run in containers when the kernel/system service behavior is meaningful there, and in VMs for reboot, network, mount, MAC, kernel-module, and destructive storage behavior. Negative tests cover loss of Remotr connectivity, SSH/sudo lockout, boot failure, secret leakage, and disk destruction.

Why: command-mock unit tests alone cannot prove system administration behavior.

Alternative considered: a single end-to-end matrix. Rejected because it is slower to diagnose and cannot cheaply exhaust contract edge cases.

## Risks / Trade-offs

- [The roadmap is too large for one implementation branch] → Treat this OpenSpec change as the umbrella contract and implement milestone-sized child changes; do not mark the umbrella complete until all non-optional requirements are delivered or explicitly descoped through an OpenSpec update.
- [Canonical schema migration disrupts existing config repositories] → Provide strict compatibility decoding, render-time deprecation warnings, an automated rewrite path, and at least one release where old input renders to canonical output.
- [Structured reports increase payload and storage size] → Use concise reason codes, redacted summaries, bounded output, digest-based unchanged suppression, and retention limits.
- [Provider matrices become expensive] → Guarantee only explicitly tested distro/backend combinations and report unsupported combinations honestly.
- [Rollback creates a false sense of safety] → Publish rollback capability per resource and require timed external recovery for connectivity/access changes.
- [Author-time validation cannot know endpoint-local runtime state] → Reject statically impossible combinations early and preserve `unsupported` as a first-class runtime outcome.
- [Root-level applicators enlarge attack surface] → Use argv execution, path-safe filesystem operations, strict field validation, least-privilege helpers where practical, and focused security review per high-risk kind.
- [Automatic remediation causes cascading host changes] → Preserve dependency ordering, stop dependent applies after failure, coalesce activation, and gate high-risk enforcement.
- [Optional breadth distracts from core convergence] → Keep M6 tasks gated on telemetry/customer demand and separate acceptance criteria.

## Migration Plan

1. Add structured execution/report types, resource/provider registries, strict canonical decoding, and adapters for all current handlers without changing their behavior.
2. Version sync/state-report payloads additively; update server persistence and CLI readers before agents begin sending new statuses.
3. Add compatibility decoding and deprecation diagnostics for current package/file/user/systemd/firewall YAML. Provide `config validate` and render output that identify fields not actually supported by the targeted agent capability matrix.
4. Deliver M1 vertical slices and make strict enforcement the default only after existing configuration repositories pass validation.
5. Roll out M2–M5 one capability at a time behind advertised agent capability/version gates. The server must not release artifacts requiring capabilities absent from their target fleet.
6. Retain rollback by downgrading the agent and rendering the prior schema during the compatibility window. High-risk resource state remains untouched on downgrade unless an explicit revert resource is deployed.
7. Remove compatibility input only in a separately announced breaking change after fleet telemetry shows no remaining use.

## Open Questions

- What canonical artifact version identifier and compatibility duration should be used?
- Should provider capability negotiation be stored per endpoint, derived solely from agent version/facts, or both?
- Which secret backends beyond root-readable local files are required for M5, and where is authorization enforced?
- What operator workflow authorizes destructive storage and enforced network transitions: signed artifact metadata, maintenance jobs, or a separate approval API?
- Which Fedora/RHEL versions and DNF generation form the first supported RPM-family matrix?
- Should rollback retention be count-based, time-based, or both, and how is sensitive rollback material encrypted at rest?
- Which optional M6 primitives have demonstrated fleet demand and should graduate into non-optional specs?
