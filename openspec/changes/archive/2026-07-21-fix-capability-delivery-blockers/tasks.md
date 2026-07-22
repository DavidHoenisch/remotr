## 1. Establish Reproduction and Evidence Boundaries

- [x] 1.1 Record the Ubuntu Pro engineering fixture's normalized target tuples and exact required capability inventory, mapping every row to OS-AEC-104–110, OS-LPC-023–027, OS-PRM-029–030, or OS-UPM-061–064 and to its required public seam and evidence layer.
- [x] 1.2 Preserve a sanitized regression fixture that reproduces the current authenticated Sync result: Ubuntu 26.04 amd64 is blocked by its own missing rows plus an irrelevant Arch/Pacman requirement, while a requested released-agent upgrade is absent.
- [x] 1.3 Establish and record the released legacy-agent decoder floor that can consume `agentUpgrade` while ignoring unknown capability-blocked metadata; classify older releases as requiring the documented out-of-band upgrade path.

## 2. Target-Aware Artifact Requirements

- [x] 2.1 For OS-AEC-108 and OS-UPM-063, name the configuration CLI plus authenticated Sync public seams, add one focused mixed Ubuntu/Arch behavioral test with independently specified APT/Pacman expectations, run it, and record the intended RED failure caused by the aggregate requirement set.
- [x] 2.2 Implement the minimum canonical target-predicate and requirement-projection behavior needed to make the OS-AEC-108/OS-UPM-063 test GREEN while retaining the complete desired artifact and digest.
- [x] 2.3 For OS-AEC-022–024 and OS-AEC-090–091, add malformed, ambiguous, no-match, portable-requirement, schema-0-losslessness, bound, and unacknowledged-artifact regression cases at the configuration CLI and authenticated Sync seams; run focused tests GREEN.
- [x] 2.4 Add or strengthen bounded fuzz properties for target normalization, predicate intersection, variant deduplication, and requirement serialization; commit any discovered crash input as a seed regression.
- [x] 2.5 Add native composition and Sync-selection benchmarks with allocation reporting for a representative heterogeneous fleet, enforce variant count/byte bounds, and record the baseline without using endpoint identity to construct variants.

## 3. Shared Validation and Actionable Git-Sync Diagnostics

- [x] 3.1 For OS-AEC-104–107 and OS-PRM-030, name the configuration CLI and Admin API seams, add one focused differential test showing `config validate` accepts a missing/ambiguous target tuple that server validation rejects, run it, and record the intended RED failure.
- [x] 3.2 Implement the minimum shared target/provider/capability validation service used by both local configuration validation and server Git sync, then rerun the focused differential test GREEN without advancing the Release ref on rejection.
- [x] 3.3 Add table-driven malformed, unsupported-release, unsupported-architecture, provider-mismatch, duplicate-target, case-normalization, and boundary cases through both public seams and require matching classification plus stable diagnostic identity.
- [x] 3.4 Add Admin API and server-log regression evidence that preserves safe file, resource, target, provider, and correlation context while a secret canary, authenticated URL, raw sensitive fragment, and environment value remain absent from responses, logs, and persisted diagnostics.
- [x] 3.5 Add a bounded fuzz property proving the CLI and server validators agree for generated target/provider documents and preserve every new counterexample as a committed corpus seed.

## 4. Frozen Production Capability Catalog

- [x] 4.1 For OS-LPC-023–025 and OS-UPM-061/064, name the composed agent execution plus authenticated Sync seams, add one focused test proving the production default generator omits a checked-in passing Ubuntu Pro row while an injected test constructor exposes it, run it, and record the intended RED failure.
- [x] 4.2 Implement the minimum validated catalog schema, deterministic generator, embedded production catalog, and default-generator wiring needed to make the OS-LPC-023/OS-UPM-061 test GREEN without runtime test-file access.
- [x] 4.3 Generate server agent-release metadata from the same release inputs, remove the synthetic capability-profile map, and add deterministic regeneration, duplicate/conflict, missing-selector, unknown-capability, non-canonical, and agent/server mismatch failures for OS-LPC-024–025.
- [x] 4.4 Add release/build verification that regeneration is clean and that a packaged production agent advertises no capability absent from the frozen catalog on mismatched release, architecture, derivative, backend, or incomplete client-API facts.
- [x] 4.5 Add secret-canary and cleanup assertions proving qualification input and catalog generation retain no token bytes or secret-bearing runtime artifacts.

## 5. Ubuntu 26.04 Provider Qualification

- [x] 5.1 For OS-LPC-026–027, run the representative Ubuntu 26.04 amd64 inventory against the frozen catalog and keep every missing core or specialized row rejected until its governing evidence task passes.
- [x] 5.2 For OS-PRM-029, name the provider-contract seam and add/run the required APT package and applicable repository/key compliant, drifted, Apply, second-Check, absence, exact-version, failure, lock, redaction, and preservation evidence on a pinned real Ubuntu 26.04 amd64 environment before publishing those exact rows.
- [x] 5.3 For OS-UPM-061–062, run the pinned disposable Ubuntu 26.04 amd64 VM provider contract with deterministic `/usr/bin/pro api` doubles, literal argv and protected JSON-stdin assertions, synthetic token canaries, negative derivative/client cases, Apply, second Check, recovery, and verified cleanup; publish only exact passing attachment and requested service/option rows.
- [x] 5.4 Run each remaining applicable core provider through its selected provider-contract, container, or VM safety/recovery fixture; publish only passing exact Ubuntu 26.04 amd64 rows and leave every unsupported configuration rejected without waivers.
- [x] 5.5 Add public configuration validate/discover/render and authenticated Sync evidence for the complete representative Ubuntu Pro fixture, proving the qualified endpoint receives and acknowledges the artifact without requiring Pacman and without claiming live Canonical enrollment or entitled native-package effects.

## 6. Capability-Blocked Upgrade Escape

- [x] 6.1 For OS-AEC-025 and OS-AEC-110, name the authenticated Sync seam, add one focused regression where a legacy endpoint is capability-blocked, explicitly targeted for an approved eligible release, and receives no upgrade instruction, then run it and record the intended RED failure.
- [x] 6.2 Implement the minimum upgrade-selection change that evaluates desired version, approved release, binary platform/architecture, integrity, and authorization independently from artifact-provider requirements; rerun the focused test GREEN with `agentUpgrade` present and artifact activation absent.
- [x] 6.3 Add malformed, unauthenticated, unauthorized, unknown-release, platform-mismatch, architecture-mismatch, revoked/integrity-failure, and no-explicit-upgrade cases proving no instruction is issued.
- [x] 6.4 For OS-AEC-109, add an authenticated Sync regression proving an upgraded agent's valid current capability document is reevaluated, the old artifact stays active until exact-digest acknowledgment, and missing runtime providers keep the endpoint blocked rather than being inferred from version metadata.
- [x] 6.5 Exercise the recorded legacy decoder fixtures and current agent against responses containing both capability-blocked metadata and `agentUpgrade`, documenting the exact out-of-band floor for releases that cannot safely consume the instruction.

## 7. End-to-End Verification and Handoff

- [x] 7.1 Add or extend a Godog scenario using only public declarative steps for the mixed-target Ubuntu Pro workflow: local validation, Git sync, legacy blocked upgrade, current capability report, artifact offer, exact-digest acknowledgment, and active-state reporting.
- [x] 7.2 Run focused tests for every verification ID, the target/validation fuzz suites, catalog regeneration check, benchmarks, provider fixtures, and the relevant authenticated Sync integration suite; record exact evidence and any approved exception in `test/evidence-exceptions.yaml` with an expiry.
- [x] 7.3 Run `make test`, then the required quick/full end-to-end and pinned Ubuntu 26.04 provider/VM suites; do not claim Ubuntu 26.04 support for any row whose selected evidence did not pass.
- [x] 7.4 Update verification traceability, capability matrices, release documentation, configuration-validation guidance, Git-sync diagnostics, blocked-upgrade recovery guidance, and the sanitized Ubuntu Pro qualification disclosure.
- [x] 7.5 Re-run strict OpenSpec validation and confirm `fix-capability-delivery-blockers` is complete before beginning implementation of `add-global-secret-scope`.
