## Context

The umbrella implemented 45 schema-1 resource kinds and broad focused coverage, but the 2026-07-15 exit audit separated three claims that are still being conflated: implementation exists, a real configuration repository composes it, and an exact provider/distribution row has passing evidence. The checked-in repositories compose almost nothing beyond packages, all Ubuntu 24.04 container rows are `untested`, and no Ubuntu security, authentication, logging, desktop, browser, network, firewall, schedule, service, filesystem, or identity row is advertised.

The current provider matrix is too coarse for closeout: rows named `filesystem`, `identity`, `service`, or `repository` can contain several resource contracts and backends with different risks. Image/backend discovery proves only that a test environment exists; it does not prove Check, Apply, second Check, absence, failure, preservation, recovery, or all accepted fields.

This child is a qualification and corrective-evidence change, not a new feature wave. It covers existing non-package M1–M5 behavior on Ubuntu 24.04 amd64. `complete-core-package-providers` owns package and repository implementation/evidence. `complete-applicator-execution-contract`, `complete-capability-compatible-delivery`, and `establish-testing-and-performance-foundation` own the shared safety, delivery, and evidence machinery that this qualification consumes.

## Goals / Non-Goals

**Goals:**

- Produce a reproducible checked-in schema-1 Ubuntu M1–M5 repository that exercises existing public configuration and composition paths.
- Inventory every existing non-package Ubuntu-targetable resource/provider contract and classify it as qualified, blocked by a failing requirement, or deliberately unadvertised.
- Replace broad discovery-only claims with exact release-specific provider rows and executable selectors.
- Use Ubuntu 24.04 containers for ordinary native-provider evidence and Ubuntu 24.04 VM fixtures for access, connectivity, boot, storage, firewall, authentication, system-service, desktop/session, and other safety behavior that containers cannot prove.
- Correct behavior gaps exposed by real qualification one focused red-green slice at a time without weakening the governing requirement.
- Re-run the M1–M5 audit and make archive eligibility a mechanical result of composition, provider, traceability, safety, and dependency gates.

**Non-Goals:**

- Implementing any new capability in `engineering/plans/ubuntu-cmmc-capability-roadmap.md`, including Hub parameters, conditional applicability, password aging, MFA, GDM, expanded browser catalogs, authoritative repositories, remote log delivery, Chrony, USB authorization, cryptographic policy, endpoint inspection, evidence export, or external integrations.
- Duplicating package or repository implementation/evidence owned by `complete-core-package-providers`.
- Qualifying Debian, Arch, RPM-family, non-amd64, or Ubuntu releases other than 24.04.
- Advertising generic `command` or `bootstrap` behavior as equivalent to a typed resource/provider contract.
- Claiming CMMC, NIST, CIS, DISA STIG, or other compliance certification from provider qualification.
- Enforcing high-risk baseline resources automatically from the checked-in example.

## Decisions

### 1. Qualification has a fixed identity tuple

Every support row is keyed by:

`resource capability ID + provider/backend + contract revision + distribution + release + architecture + environment`

For this change, the platform tuple is `ubuntu + 24.04 + amd64`. A broad family row such as `identity/ubuntu/24.04` can remain a discovery grouping, but it cannot advertise `user`, `sudo`, and `loginPolicy` as a unit unless separately referenced exact contract rows all pass. Capability generation consumes only qualifying exact rows.

Alternative considered: promote the existing broad rows after one representative test. Rejected because it would overclaim untested fields, providers, and risk behavior.

### 2. One checked-in repository provides composition proof, not endpoint safety proof

Add a dedicated repository under `test/config-repos/ubuntu-2404-m1-m5/` with a fleet manifest and schema-1 modules organized by milestone and risk domain. It SHALL:

- contain representative resources for every non-package contract intended for Ubuntu qualification;
- include dependencies, provider selections, ownership, lifecycle, activation, and secret references needed to exercise canonical rendering;
- use deterministic test-only values and references, never deployable `CHANGEME` placeholders or real secrets;
- set access, connectivity, boot, destructive, sensitive, and other guarded resources to report-only/non-enforcing intent unless an isolated fixture explicitly authorizes them; and
- remain source-only: `desired.yaml` and `crons.yaml` are rendered to temporary output and never committed.

The public operator CLI validates, discovers, and renders this repository. Golden semantic assertions parse the output and prove that every expected resource address and field survives composition. A passing render does not itself qualify a provider.

Alternative considered: add M1–M5 resources to the default Compose development fleet. Rejected because realistic high-risk examples should not become accidental local enforcement and because a dedicated fixture provides deterministic audit scope.

### 3. The qualification inventory is generated from registered contracts

Create a checked-in qualification manifest that enumerates the registry's non-package Ubuntu-targetable capability contracts, accepted fields, backends, default/effective risk, required evidence class, composed fixture address, provider-matrix row, traceability IDs, and disposition. Validation compares it to the resource/provider registry and fails on silent additions or omissions.

Allowed dispositions are:

- `qualified`: every required selector passes and the capability can be advertised for the exact tuple;
- `blocked`: implementation exists but a named requirement, dependency, or evidence selector is incomplete;
- `unadvertised`: intentionally unsupported/deferred, with a roadmap or specification reason.

There is no `implemented` disposition that implies support. Package/repository contracts reference the sibling change rather than duplicating their evidence. Generic `command`, one-shot `bootstrap`, legacy compatibility kinds, and demand-specific `agentInstall` are recorded explicitly but do not stand in for typed M1–M5 provider qualification.

Alternative considered: maintain the inventory manually only in prose. Rejected because registry drift and broad matrix rows caused the current overstatement risk.

### 4. Evidence environment is selected by behavior risk

The cheapest trustworthy public seam is used, but the following minimum environment policy applies:

| Behavior | Minimum real environment |
| --- | --- |
| Pure file/directory/link/download and other non-service POSIX behavior | Pinned Ubuntu 24.04 container |
| Account database, SSH authorization, sudo, PAM, limits, or recovery-principal behavior | Ubuntu 24.04 VM with login/access recovery fixture |
| Kernel, boot, module, mount, swap, reboot, time/locale service, or systemd lifecycle | Ubuntu 24.04 VM with boot/storage recovery as applicable |
| Firewall, DNS, route, hosts, or network-profile enforcement | Ubuntu 24.04 VM with control-path break/recovery and authenticated acknowledgement |
| Certificate/private key, trust, AppArmor, audit, journald, or logrotate | Ubuntu 24.04 VM when kernel/service/effective state matters, plus secret-canary evidence where values can be sensitive |
| Desktop setting, session, systemd-user, or browser policy | Ubuntu 24.04 desktop/session VM with logged-in and logged-out users |

Provider-contract compliant, drifted, Apply, second-Check, absence, unsupported, probe failure, validation failure, lock contention, cancellation, activation, redaction, and rollback fixtures remain required where applicable. Containers never substitute for VM safety/recovery evidence.

Alternative considered: use privileged containers for all system behavior. Rejected because they do not faithfully prove boot, PAM/login recovery, network reachability rollback, actual systemd/session activation, AppArmor/audit kernel state, mounts, or reboot.

### 5. Qualification is vertical by contract, not a batch test rewrite

Each inventory row begins by naming its governing verification ID(s), public seam, and selected evidence layers. The implementer writes one focused behavioral test with an independently known result and records the intended red failure before modifying provider code. The minimum implementation makes it green, followed by risk-selected checks and the relevant broader suite.

If a provider cannot meet an accepted field or safety requirement, the row remains blocked or the field/provider combination is rejected through a separately reviewed specification change; tests are not weakened to promote the row. New fuzz crashes become committed seed regressions. Skips or manual evidence require a reviewed, expiring record in `test/evidence-exceptions.yaml` and cannot satisfy an advertisement gate by themselves.

Alternative considered: mark existing focused unit coverage as qualification evidence. Rejected because it often tests private command construction or partial paths without real native state or recovery.

### 6. High-risk example state is non-enforcing by construction

The composed repository may render high-risk resources so validation, composition, hashing, capability requirements, and review plans are exercised. It does not authorize Apply. High-risk fixtures use isolated fleets, test identities, explicit Rollout authorization, protected recovery principals/control paths, and disposable VM snapshots. Each VM fixture proves both successful verification and failure recovery before its row can pass.

The execution-contract child must be accepted before high-risk rows are advertised, even if provider-specific VM evidence passes earlier. The capability-delivery child must be accepted before a mixed fleet may receive the resulting artifact. The foundation change must be accepted before traceability disposition is promoted to verified.

Alternative considered: make qualification independent from shared child changes. Rejected because provider tests cannot compensate for missing durable rollback, typed redaction, truthful capability delivery, or release-grade traceability.

### 7. The exit audit is generated from all gates

The refreshed M1–M5 report is computed from:

1. public validation/discovery/render results for the checked-in repository;
2. qualification-manifest completeness against the registries;
3. exact passing provider-matrix selectors;
4. verified traceability selectors and any evidence exceptions;
5. required VM safety/recovery outcomes;
6. capability-advertisement consistency; and
7. acceptance of the four dependent OpenSpec workstreams.

Each milestone reports `qualified`, `blocked`, or `not targeted` per contract. Planned, skipped, untested, missing, or failing evidence keeps the relevant claim blocked. The umbrella archive gate can close only when every non-optional target is qualified or explicitly descoped through an approved OpenSpec update.

Alternative considered: manually declare milestone completion after the broad test suite passes. Rejected because a broad suite does not prove that every advertised claim has the required selector and environment.

## Risks / Trade-offs

- **[Qualification reveals more provider defects than expected]** → Keep work vertical by exact contract row, preserve blocked dispositions, and correct each real gap with focused TDD rather than hiding it behind the aggregate suite.
- **[The fixture becomes a misleading hardening profile]** → Label it test-only, use inert values and report-only high-risk policy, avoid compliance labels, and keep future CMMC/Hub content in the separate roadmap.
- **[Provider-matrix granularity increases maintenance]** → Generate completeness checks from stable capability IDs and contract revisions; exact rows are intentionally the cost of a trustworthy support claim.
- **[VM evidence is slow or environment-sensitive]** → Separate deterministic VM targets by risk domain, pin the Ubuntu box/image, snapshot/restore between destructive cases, and run focused fixtures before aggregate qualification.
- **[Desktop evidence requires additional infrastructure]** → Keep desktop/browser rows blocked until a reproducible logged-in/logged-out Ubuntu session fixture exists; do not substitute static file assertions.
- **[Dependencies finish in a different order]** → Allow provider rows to collect evidence, but keep capability advertisement and final audit blocked until execution, delivery, package, and foundation gates are accepted.
- **[A future registry addition silently escapes qualification]** → Fail manifest completeness validation whenever a new Ubuntu-targetable contract lacks an explicit disposition and evidence class.

## Migration Plan

1. Check in the qualification manifest and completeness validation with every existing non-package Ubuntu-targetable contract initially marked `blocked` or `unadvertised` from the current audit.
2. Add the source-only Ubuntu 24.04 M1–M5 configuration repository and public CLI composition assertions.
3. Split broad discovery rows into exact contract/backend rows while preserving `untested` status; do not change capability advertisement yet.
4. Qualify ordinary POSIX/provider contracts in the pinned Ubuntu 24.04 container one red-green row at a time.
5. Add Ubuntu 24.04 server and desktop VM fixtures, then qualify access, connectivity, boot/storage, system-service, security, and interactive-session contracts one row at a time.
6. Correct real provider behavior gaps through focused TDD and update traceability/evidence selectors without changing the requirement to fit the implementation.
7. Promote an exact row and its manifest disposition only after all required selectors pass and no unexpired exception substitutes for required evidence.
8. Enable capability advertisement only after the row and shared execution/delivery/foundation dependencies pass.
9. Re-run and update the M1–M5 gap report, configuration/provider documentation, and umbrella archive decision from the generated audit inputs.

Rollback removes advertisement for affected exact rows before reverting qualification code or fixtures. The prior broad discovery rows can remain non-advertising diagnostics, and removing the test repository has no endpoint effect because it is never a production fleet source.

## Open Questions

None. The target release, inventory model, composition boundary, evidence environments, dependency gates, and non-goals are fixed for this change.
