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

Canonical deployable artifacts carry the top-level integer `schemaVersion: 1`; unversioned artifacts are interpreted as legacy schema `0`. Artifact schema versions are independent of Remotr release versions and increase only for incompatible artifact-contract changes. The canonical envelope begins:

```yaml
schemaVersion: 1
configurations: []
```

Each resource has a stable kind, logical name, lifecycle state, resource-specific fields, and shared metadata. Omitted optional fields mean “unmanaged”; defaults are applied only when the specification names a default. Accepted fields must participate in validation and, when managed, in both Check and Apply.

Existing unversioned plural collections remain readable as schema `0` through a compatibility decoder. Composition always produces canonical schema `1` and, during migration, may additionally produce a schema-`0` compatibility variant only when it can do so without losing behavior. Schema `0` support remains for at least two minor Remotr releases and 90 days after schema `1` ships, whichever is longer. Removal additionally requires fleet telemetry showing no schema-`0` endpoints and an announced breaking release. Unknown fields and invalid field combinations are rejected using strict decoding for schema `1`.

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

Every agent reports an authenticated endpoint capability document during every Sync. The document contains supported artifact schema versions; stable capability IDs and contract revisions; normalized provider facts; agent version; and a document digest. The server records its own receive time and stores the latest document in the server registry for readiness previews and operator reporting. Artifact selection uses only the capability evidence in the current authenticated Sync; persisted evidence has visible age but no arbitrary delivery TTL because an offline endpoint cannot receive an artifact and reports again before its next selection.

Agent version is metadata rather than proof of runtime provider support. Existing agents known not to implement capability reporting receive a conservative schema-`0` profile from a server-maintained version mapping. A modern agent that omits or sends an invalid capability document keeps its last active artifact and becomes capability-blocked; unknown versions do not receive capabilities beyond the legacy baseline.

Composition computes an explicit artifact requirement set and caches bounded schema variants rather than arbitrary per-endpoint builds: canonical schema `1` plus a lossless schema-`0` compatibility variant when possible. On Sync, the server compares the endpoint capability document to each variant and serves the highest compatible one. It never removes unsupported resources or fields to manufacture compatibility.

If the current global Release ref has no compatible artifact, the Release ref still advances for the system. An endpoint with a previously processed artifact continues checking that last compatible artifact and receives a `capability_blocked` delivery status containing the unavailable Release ref and missing capabilities; a new endpoint without a prior artifact remains explicitly unmanaged and blocked. Operator reporting distinguishes the global Release ref from each endpoint's active artifact Release ref. A compatible agent-upgrade instruction may accompany the blocked response.

The implementation matrix for this change is Debian/Ubuntu plus Arch. Fedora, RHEL, DNF4/DNF5, RPM repositories, image-based RPM systems, and RPM-dependent SELinux/authselect providers remain roadmap capabilities but are explicitly deferred. M1 removes `dnf` from advertised authored configuration and rejects it with a roadmap diagnostic rather than retaining a selectable stub. A future OpenSpec change must define facts, mutable-versus-image system boundaries, DNF generation detection, provider contracts, supported versions/architectures, and real distribution integration tests before any RPM-family capability is advertised. Provider-specific fields live in an explicit provider options namespace and are rejected by other providers.

Author-time validation rejects impossible target/provider combinations known from the declared matrix. Runtime absence or mismatch is a structured `unsupported` result, not perpetual drift.

Why: portable intent is valuable, but pretending backends are interchangeable produces unsafe shell branching.

Alternatives considered: derive all support from agent version, trust persisted capability evidence for a wall-clock TTL, block global Release ref advancement until every endpoint is compatible, compile arbitrary per-endpoint artifacts, silently omit unsupported resources, or use one applicator with distro conditionals. Version alone cannot represent endpoint-local backends; a TTL is arbitrary when current evidence is available at delivery; fleet-wide blocking lets one stale endpoint freeze unrelated work; per-endpoint builds create unbounded variants; omission corrupts desired-state meaning; and distro conditionals mix policy with execution.

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

When a merged Release ref contains related high-risk resource changes, the server creates one or more Change requests in the server registry. A request never spans Fleets or endpoint overrides. Authors can assign related resources an `authorizationGroup`; absent that metadata, the server groups high-risk resources by dependency-connected component, including shared activation edges. Independent components become separate requests. Normal prerequisites appear in the plan but do not become high-risk solely through grouping, while a group containing multiple risk classes inherits the strictest authorization and rollout policy. Grouping coordinates review and rollout; it does not promise atomic mutation across endpoints.

Agents evaluate the grouped resources without enforcing them and return preflight evidence; each request freezes its evaluated endpoint set before authorization. Normal resources may continue converging unless they depend on a pending high-risk resource. The request records the Release ref, artifact and resource hashes, authorization group, risk classes, target set, compatibility, predicted effects, rollback capability, and preflight results. Automation may approve multiple requests together, but each retains independent evidence, authorization, and outcome.

Authorization has two lifetimes:

- A Rollout authorization is short-lived and permits the exact high-risk resource hashes to run against the frozen endpoint set under a maintenance window, expiry, attempt limit, and concurrency policy.
- A Fleet baseline authorization is durable permission for future members of the named fleet to converge to the same resource addresses and desired-state hashes. It can be promoted only after a successful verified rollout and only when explicitly requested. Each future endpoint must still pass its own capability and safety preflight.

Approval thresholds are server-registry policy configured globally with per-Fleet and per-risk-class overrides. Connectivity, access/privilege, boot policy, coordinated reboot, and corresponding baseline promotion require one approver by default; destructive storage requires two. Approvals count distinct operator identities rather than credential fingerprints and require risk-appropriate RBAC permission. A single-operator installation may explicitly lower the destructive threshold to one with a persistent audit warning. Remotr can enforce separation among operator identities but cannot prove separation from a Git author without future forge identity integration.

The first operator can define rollout controls and contribute the first approval; additional operators approve the unchanged plan until its threshold is met. If baseline promotion is requested in the original authorization, the same threshold covers promotion after successful verification. A later standalone promotion requires fresh approvals. Changing resource hashes, target scope, maintenance authorization, or rollout-control bounds invalidates existing approvals.

Authorization validity is separate from execution windows. Rollout authorization is configurable with a 30-day default and can contain recurring maintenance windows; an endpoint may execute only when it is in the frozen target set, online within a window, currently preflight-ready, and the authorization remains valid. Rollout evidence classifies every frozen target as verified successful, failed/rolled back, capability/preflight blocked, or not seen during authorization.

Manual Fleet baseline promotion is the default and displays all four outcome buckets. Offline, blocked, or not-seen endpoints do not prevent manual promotion, but exceptions require explicit acknowledgement and the applicable approval threshold; those endpoints later use the baseline only after their own current preflight. Automatic `baseline-on-success` is available only when Fleet Approval policy explicitly defines canary stages, minimum successful evidence, and maximum failures. Any unresolved failure or rollback blocks automatic promotion. There is no universal percentage threshold.

A Rollout authorization does not itself release work. During an Execution window, the server issues a short-lived endpoint-specific Execution lease only when the endpoint is in the frozen target set, current preflight passes, authorization remains valid, and a concurrency slot is available. The lease binds Change request, endpoint, resource hashes, attempt number, and expiry and is delivered during authenticated Sync. High-risk work never starts from a cached authorization. Pause or revoke stops new leases; work already applying proceeds to verification or rollback because it cannot be safely pretended cancelled.

Acknowledgement is risk-specific. Network and firewall providers arm a provider checkpoint or independent local rollback watchdog before Apply, then require a new authenticated Sync over the changed path and server acknowledgement before cancelling rollback. Access/privilege providers validate effective policy and preserve a declared recovery principal; Sync proves Remotr access but not human login, so Fleet policy may require manual canary verification before later batches. Reboot persists the lease and prior boot ID, acknowledges immediately before reboot, and succeeds only after reconnect with a different boot ID. Boot policy remains next-boot/reboot-required until coordinated verification. Destructive storage binds stable device identity and verifies postconditions while honestly reporting rollback unavailable when irreversible.

Emergency operation uses a dedicated Break-glass authorization rather than a general force flag. It requires risk-specific RBAC permission, exact resource hashes and endpoint targets, justification, and an external incident/change reference. Defaults are one endpoint, one attempt, and 60-minute validity. It may bypass ordinary approval count, Execution windows, and concurrency, but never schema/provider validation, redaction, current preflight, rollback requirements, or destructive-storage identity and irreversible-operation approval. Fleet-wide break glass requires explicit fleet scope and a second operator. It cannot become a Fleet baseline authorization, and creation, use, expiry, and revocation emit prominent audit/SIEM events. If the server is unreachable, operators must use an out-of-band recovery channel.

Changing a managed field changes the desired-state hash and requires a new Change request. Firewall/network, SSH/sudo, mount declarations, and boot policy may be baseline-eligible when their providers support safe preflight. Reboot is an event rather than a baseline; destructive storage remains bound to endpoint-specific stable device identity; and emergency overrides never become baselines. Initial adoption can aggregate the current high-risk state of an existing fleet into one reviewed baseline request rather than manufacturing historical requests per resource.

Applicators emit activation signals (`reload`, `restart`, `daemon-reload`, `logout-required`, `reboot-required`, or `next-boot`). The engine coalesces compatible signals and executes them after their producing resources and dependencies succeed. Immediate reboot is a separate coordinated resource, never an incidental package action.

Why: activation and risk are cross-cutting lifecycle concerns, not ad hoc fields on downloads or files.

Alternatives considered: authorize only a frozen rollout forever, dynamically authorize all future members immediately, push one long-lived authorization to every endpoint, hard-code one approval for every organization, mandate two people for every change, provide a generic force bypass, or keep per-resource notify fields. Frozen-only authorization creates unbounded onboarding work; immediate dynamic authorization exposes unproven state; long-lived endpoint grants weaken pause, concurrency, and window enforcement; one universal threshold cannot meet regulated controls; universal two-person control makes small installations inoperable; force bypasses become an unaudited normal path; and resource-specific notification fields duplicate behavior.

### 7. Use lock domains and durable transaction metadata

Resources declare lock domains such as package database, account database, firewall, network, storage, or reboot. The agent serializes conflicting operations and respects native provider locks/timeouts.

When rollback is supported, prior-state metadata and payloads are stored only in a root-owned agent directory keyed by resource address, artifact digest, and attempt; applicators do not leave generic adjacent backup files. Metadata retains the last 10 attempts per Resource for at most 30 days. Non-secret payloads retain the last three successful prior states for at most 30 days. Sensitive or secret-bearing payloads exist only while an attempt is armed or unacknowledged and have an absolute 24-hour maximum. A configurable global disk cap provides a third bound, but armed rollback is never pruned to satisfy it. If promised rollback cannot durably reserve required space, Apply is blocked before mutation.

Public metadata is root-only and contains no sensitive values. Durable payloads use AES-256-GCM under an endpoint-local rollback key, preferably sealed by TPM and otherwise stored in a root-only file with the explicit limitation that this does not protect against endpoint-root compromise. Payloads are checksummed and written atomically. The server stores rollback metadata and outcomes, not payloads.

Remotr-managed secrets normally roll back through a retained prior Secret-version reference. When disconnected recovery requires prior secret bytes, such as a Wi-Fi transition, the agent may retain an encrypted short-lived payload until acknowledgement and then destroys it. Secret versions referenced by armed or retained rollback cannot be deleted until the reference expires or an authorized operator explicitly abandons recovery. Cleanup follows success, rollback, expiry, or supersession.

Rollback capability remains `transactional`, `best_effort`, or `none`. A failed rollback is reported alongside the original apply failure rather than replacing it; irreversible operations create no fictional payload and require explicit authorization acknowledgement.

Why: package managers and host databases are shared mutable systems, and the current executor can obscure the primary error when revert fails.

Alternatives considered: rely only on the engine's current sequential loop, keep adjacent backup files indefinitely, retain by count only, or retain by time only. Sequential execution does not model provider exclusion; adjacent backups spread sensitive copies; and any single retention dimension permits either stale or unbounded state.

### 8. Treat redaction as typed data-flow policy

Desired/observed fields are classified public, sensitive metadata, or secret at schema definition. Redaction happens before values enter logs, drift payloads, backups, diagnostics, or persistence. Reports include secret identifiers, versions/fingerprints where safe, and expiry/health, but never secret bytes, private keys, passwords, repository credentials, or command arguments containing resolved secrets.

Secret resources reference a source by identifier; Git and compiled artifacts contain references only. The first providers are:

- `local-file`, for root-readable material provisioned independently of Remotr, migration, and provider contract tests;
- `remotr`, a minimal production secret distributor backed by application-encrypted, versioned records in the server registry.

Operators upload Remotr-managed secret versions through the Admin CLI using stdin or a protected file, never a command-line value. The encryption master key remains outside Postgres in the deployment's secret mechanism. Agents resolve material just-in-time over authenticated mTLS, with authorization bound to endpoint identity, Fleet/endpoint scope, active artifact digest, resource address, and declared purpose. Secret bytes never enter compiled artifacts, caches, logs, reports, or generic rollback storage. Operators can rotate, revoke, inspect safe metadata, and audit access without a plaintext read API.

Each Remotr secret version uses a fresh random 256-bit data-encryption key (DEK) and AES-256-GCM authenticated encryption with random nonces. Postgres stores ciphertext, the wrapped DEK, algorithm/format version, master-key ID, nonces, and authenticated non-secret scope metadata. A versioned master-key key-encryption-key (KEK) keyring remains outside Postgres, preferably in a root-only file or deployment secret mount, with environment-secret input supported for platforms that require it. Production never silently generates a KEK.

The keyring has one active encryption KEK and zero or more decrypt-only historical KEKs. Routine rotation rewraps DEKs under a new active KEK. Suspected KEK compromise performs a full rekey with new DEKs and new secret ciphertext because rewrapping alone does not protect data exposed with the old KEK. Recovery requires both the database backup and external keyring; tooling surfaces this dependency and refuses removal of a KEK still referenced by stored versions. A key-encryption-provider boundary permits later KMS/HSM wrapping without changing resource or stored-secret schemas.

The secret provider interface remains open for later server-side adapters such as Vault or cloud secret managers, while the key-encryption-provider interface covers KMS/HSM wrapping for Remotr-managed ciphertext. Remotr deliberately provides distribution and lifecycle needed by desired-state resources rather than becoming a general-purpose human secret browser or arbitrary credential broker.

Every Secret reference explicitly selects either an exact provider version or the provider's `active` version; there is no implicit latest selector. Pinned versions change only through Git. For `active`, uploading a secret creates an inactive version, while a separate audited server-registry activation changes the effective version. Activation creates a rollout using the risk policy of every referencing Resource: a network or access credential creates a high-risk Change request, while a lower-risk service credential follows its ordinary rollout policy.

Effective Resource hashes include provider, logical secret name, selected version identity, and activation generation but never secret bytes. Activation therefore invalidates collected authorization for the prior effective hash. Revocation blocks future resolution without claiming to erase copies already installed on endpoints; desired-state removal or rotation must converge those copies. The provider-neutral resolution result includes version identity, safe metadata, and bounded material whether resolution is endpoint-local, Remotr-backed, or supplied by a later server-side adapter.

Why: string-scrubbing after logging is too late and incomplete.

Alternatives considered: mark entire resources sensitive and omit all reporting, support local files only, require an external vault, or embed encrypted blobs in Git. Total omission destroys safe compliance evidence; local files do not provision new endpoints; external-only support forces infrastructure choices on every deployment; and encrypted Git creates difficult key distribution and rotation semantics.

### 9. Keep server jobs and endpoint schedules separate

Server-dispatched cron continues to mean “the server decides work is due and the next connected agent executes it.” Endpoint schedules manage persistent local cron entries or systemd timers that execute without server reachability. They use distinct schema kinds, reports, and guarantees.

Why: merging the models would misrepresent offline behavior and observability.

### 10. Stage delivery behind capability gates

The implementation order is:

1. Foundation: strict/versioned schema, structured execution/reporting, registry, provider facts, locks, redaction, and compatibility adapters.
2. M1 truthful convergence: package versions, file metadata/lifecycle, UID enforcement, removal of unimplemented DNF advertisement, and firewall presence/removal.
3. M2 access baseline: groups/users, directories/links, authorized keys, sudo fragments.
4. M3 OS baseline: repositories/keys, sysctl/modules, host identity/time/locale, mounts/swap.
5. M4 durable operations: endpoint schedules, richer services, reboot coordination, network audit/apply.
6. M5 security/workstation: MAC/audit, certificates/secrets, logging, authentication/limits, desktop policy.
7. M6 roadmap only: archives/VCS, destructive storage, containers, alternatives, file capabilities, environment fragments, and transient paths are not implemented or advertised by this change. Each requires a demand-backed child OpenSpec change with provider boundaries, security review, maintenance ownership, and integration-test infrastructure.

Each gate includes schema, validation, composition, provider, engine wiring, unit tests, distro integration tests, telemetry, docs, migration examples, and feature advertisement. A field is not documented as supported until this vertical slice is complete.

Why: landing schemas far ahead of enforcement recreates the current trust gap.

Alternative considered: implement all schemas first and fill providers later. Rejected because accepted but unenforced configuration is the primary problem being fixed.

### 11. Test contracts and providers independently

The dedicated `establish-testing-and-performance-foundation` change lands before M1–M5 work begins. It owns immutable OpenSpec verification IDs, the traceability manifest, public test seams, vertical-slice TDD rules, selective Godog acceptance, continuous race/fuzz/mutation gates, the shared provider-conformance harness, container/VM infrastructure, native benchmarks, and authenticated fleet-load/soak testing. This umbrella change consumes those facilities rather than creating parallel milestone-specific harnesses.

Every umbrella scenario is classified in the traceability manifest. A planned scenario may remain without evidence only while its behavior is unimplemented and unadvertised. Each implementation slice begins with its verification IDs and a failing test at an agreed seam; it cannot advertise a field/provider or complete its task until required unit/contract, acceptance, provider, safety, mutation, and performance evidence passes.

Contract tests run every provider through compliant, drift, apply, second-check idempotence, absence, unsupported, probe-failure, validation-failure, lock-contention, cancellation, activation, redaction, and rollback cases. Provider integration tests run in containers when the kernel/system service behavior is meaningful there, and in isolated VMs for reboot, network, mount, MAC, kernel-module, authentication recovery, and destructive storage behavior. Negative tests cover loss of Remotr connectivity, SSH/sudo lockout, boot failure, secret leakage, and disk destruction.

Server and agent hot paths use the foundation's benchmark and 400-endpoint reference workloads. The scheduled 4,000-endpoint comparison detects nonlinear growth but does not itself advertise a support promise. Absolute latency, CPU, memory, and mutation budgets are established from controlled foundation baselines and thereafter change only through an approved OpenSpec update.

Why: command-mock unit tests alone cannot prove system administration behavior.

Alternative considered: duplicate test infrastructure inside each milestone. Rejected because seams, evidence rules, and performance baselines would drift. A single end-to-end matrix was also rejected because it is slower to diagnose and cannot cheaply exhaust contract edge cases.

## Risks / Trade-offs

- [The roadmap is too large for one implementation branch] → Treat this OpenSpec change as the umbrella contract and implement milestone-sized child changes; do not mark the umbrella complete until all non-optional requirements are delivered or explicitly descoped through an OpenSpec update.
- [Canonical schema migration disrupts existing config repositories] → Provide strict compatibility decoding, render-time deprecation warnings, an automated rewrite path, and at least one release where old input renders to canonical output.
- [Structured reports increase payload and storage size] → Use concise reason codes, redacted summaries, bounded output, digest-based unchanged suppression, and retention limits.
- [Provider matrices become expensive] → Guarantee only explicitly tested distro/backend combinations and report unsupported combinations honestly.
- [Rollback creates a false sense of safety] → Publish rollback capability per resource and require timed external recovery for connectivity/access changes.
- [Author-time validation cannot know endpoint-local runtime state] → Reject statically impossible combinations early and preserve `unsupported` as a first-class runtime outcome.
- [Root-level applicators enlarge attack surface] → Use argv execution, path-safe filesystem operations, strict field validation, least-privilege helpers where practical, and focused security review per high-risk kind.
- [Automatic remediation causes cascading host changes] → Preserve dependency ordering, stop dependent applies after failure, coalesce activation, and gate high-risk enforcement.
- [Optional breadth distracts from core convergence] → Keep every M6 provider roadmap-only until a concrete fleet use case and focused child OpenSpec change justify promotion.

## Migration Plan

1. Add structured execution/report types, resource/provider registries, strict canonical decoding, and adapters for all current handlers without changing their behavior.
2. Version sync/state-report payloads additively; update server persistence and CLI readers before agents begin sending new statuses.
3. Treat unversioned input as schema `0`; add compatibility decoding and deprecation diagnostics for current package/file/user/systemd/firewall YAML. Emit canonical schema `1` plus a lossless schema-`0` compatibility variant where possible, and provide `config validate` and render output that identify fields not actually supported by the targeted agent capability matrix.
4. Deliver M1 vertical slices and make strict enforcement the default only after existing configuration repositories pass validation.
5. Roll out M2–M5 one capability at a time behind advertised endpoint capability gates. The server serves the highest compatible bounded variant; incompatible endpoints remain on their last successfully processed artifact with visible `capability_blocked` status.
6. Retain rollback by downgrading the agent and rendering the prior schema during the compatibility window. High-risk resource state remains untouched on downgrade unless an explicit revert resource is deployed.
7. Retain schema `0` compatibility for at least two minor releases and 90 days, whichever is longer. Remove it only in a separately announced breaking release after fleet telemetry shows no remaining schema-`0` endpoints.

## Open Questions

None. The foundation decisions for this change are resolved; later changes may revisit them through the OpenSpec update workflow.
