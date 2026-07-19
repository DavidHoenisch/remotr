## 1. Make the Existing Baseline Trustworthy

- [x] 1.1 Record current test inventory, package coverage, race runtime, E2E coverage, CI cadence, and known skipped or failing cases in a versioned baseline report.
- [x] 1.2 Add a repository-wide pull-request workflow for strict OpenSpec validation, formatting/vetting, ordinary Go tests, coverage artifacts, and the Go race detector.
- [x] 1.3 Fix the stale Postgres fuzz target reference and make zero-match fuzz invocations fail rather than warn successfully.
- [x] 1.4 Replace the handwritten fuzz target list with discovery that fails for omitted, duplicate, or missing native Go fuzz targets.
- [x] 1.5 Make all discovered fuzz seed corpora run under the ordinary PR test command and add a bounded short-fuzz command for affected packages.
- [x] 1.6 Repair or explicitly retire known E2E harness defects, including global CLI flag placement, so the baseline suite is reliably green from a clean stack.
- [x] 1.7 Add deterministic test timeouts, failure artifact collection, and package-level timing output without introducing silent retries.
- [x] 1.8 Document generated-code coverage exclusions and publish package plus changed-line coverage without imposing a blanket repository percentage.

## 2. Establish Verification Identity and Traceability

- [x] 2.1 Define and validate the immutable `verification-id` comment syntax and centrally registered capability prefixes.
- [x] 2.2 Build an OpenSpec inventory parser that discovers scenarios across active and archived changes and reports source change, capability, requirement, and scenario.
- [x] 2.3 Assign stable verification IDs to all scenarios in this foundation change and `expand-linux-system-administration-applicators` without changing their behavioral text.
- [x] 2.4 Define the versioned `test/traceability.yaml` schema for lifecycle state, verification classes, selectors, environments, and disposition reasons.
- [x] 2.5 Generate the initial traceability manifest for the applicator umbrella using truthful `planned`, `verified`, and deferred classifications.
- [x] 2.6 Implement traceability lint for missing, duplicate, reused, malformed, and orphaned identifiers and for invalid selectors or environment references.
- [x] 2.7 Implement the advertisement gate that rejects active capability claims with `planned`, deferred-only, missing, or failing governing evidence.
- [x] 2.8 Add tests proving one selector can cover multiple IDs and one ID can require multiple evidence layers without weakening completeness checks.
- [x] 2.9 Add traceability validation to PR, scheduled, and release workflows and emit actionable source locations on failure.

## 3. Encode Test Seams and TDD Governance

- [x] 3.1 Document the seven approved public seams with examples of acceptable and implementation-coupled tests for this repository.
- [x] 3.2 Add a pull-request template requiring verification IDs, selected seams, red command/result, final selectors, mutation outcome, and benchmark impact.
- [x] 3.3 Update contributor and AI-agent instructions to require one vertical red-green slice at a time and prohibit unapproved test weakening, derived expectations, internal mock call assertions, and undocumented skips.
- [x] 3.4 Provide shared fake clock, seeded randomness, synthetic secret-canary, bounded context, and cleanup helpers at external boundaries.
- [x] 3.5 Add repository checks for permanent focused-test markers, unowned skips/quarantines, and expired flaky-test exceptions.
- [x] 3.6 Define the reviewed exception format for an intentionally manual, not-applicable, equivalent-mutant, or temporarily quarantined evidence item.

## 4. Pilot Selective Godog Acceptance

- [x] 4.1 Pin and vendor a reviewed Godog version and isolate all library integration inside `test/acceptance` with no production dependency wiring.
- [x] 4.2 Add a `go test`-integrated acceptance runner with deterministic scenario isolation, tag filtering, bounded timeouts, and redacted failure attachments.
- [x] 4.3 Implement traceability lint that requires every Godog scenario to carry known active `@os_<verification-id>` tags.
- [x] 4.4 Define a small declarative step vocabulary over CLI, Admin API, Sync protocol, and controlled agent execution seams rather than private helpers.
- [x] 4.5 Add a configuration-authoring tracer feature covering validation failure and deterministic rendered output through the operator CLI.
- [x] 4.6 Add an operator-bootstrap tracer feature covering one-time bootstrap, credential use, and endpoint listing through the operator CLI.
- [x] 4.7 Add an enrollment-and-Sync tracer feature covering agent enrollment, stored credentials, and authenticated artifact delivery.
- [x] 4.8 Add an app-package tracer feature covering operator-visible listing of the seeded package catalog.
- [x] 4.9 Add an endpoint-label tracer feature covering authenticated Sync label reporting and operator-visible endpoint state.
- [x] 4.10 Review the pilot for step duplication, implementation coupling, runtime, failure clarity, and domain readability; record explicit acceptance or a revised boundary before expanding it.

## 5. Build the Provider Conformance Harness

- [x] 5.1 Define the public provider contract adapter used by the shared harness without exposing private provider algorithms.
- [x] 5.2 Add shared compliant, drifted, Apply, and second-check idempotence contract cases.
- [x] 5.3 Add shared absence/removal, unsupported, probe-failure, and validation-failure contract cases.
- [x] 5.4 Add shared lock-contention, cancellation, timeout, and concurrent-operation contract cases.
- [x] 5.5 Add shared activation ordering/deduplication, redaction-canary, rollback-class, and rollback-failure contract cases.
- [x] 5.6 Preserve focused exact-argv assertions for shell avoidance, argument separation, noninteractive operation, and forbidden unsafe flags at the process boundary.
- [x] 5.7 Migrate representative APT, file, systemd, and firewall providers through the harness and publish a gap report for every failed case.
- [x] 5.8 Fix or truthfully de-advertise representative-provider gaps before enabling the conformance gate for newly advertised behavior.

## 6. Establish the Real Linux Environment Matrix

- [x] 6.1 Define the versioned provider matrix schema for distribution release, architecture, backend, contract revision, environment kind, and required selectors.
- [x] 6.2 Add pinned Debian and Ubuntu container environments for behavior containers can faithfully prove.
- [x] 6.3 Add a pinned Arch container environment and separate Pacman from any AUR-helper assumptions.
- [x] 6.4 Add disposable VM orchestration with snapshot restore, isolated management network, bounded disks, synthetic credentials, and verified teardown.
- [x] 6.5 Add VM fixtures for network/control-path rollback and authenticated recovery acknowledgement.
- [x] 6.6 Add VM fixtures for reboot/boot-ID, mount, kernel module/sysctl, AppArmor, and authentication recovery behavior as introduced by dependent changes.
- [x] 6.7 Add negative safety scenarios for connectivity loss, last-admin-path removal, invalid boot state, ambiguous devices, and secret-canary leakage.
- [x] 6.8 Retain bounded redacted provider facts, state transitions, safe argv, and system diagnostics for failed environment cases.
- [x] 6.9 Generate or validate capability advertisement from passing provider-matrix evidence and prove an untested matrix row remains unadvertised.

## 7. Strengthen Native Fuzzing

- [x] 7.1 Review every current fuzz target for a durable property, bounded input/resource behavior, and useful seed diversity.
- [x] 7.2 Add fuzz properties for schema-version parsing, capability documents, artifact selection, and mixed-version Sync payloads.
- [x] 7.3 Add fuzz properties for resource addresses, dependency graphs, authorization grouping, execution leases, and activation ordering.
- [x] 7.4 Add fuzz properties for secret references/redaction, rollback retention metadata, safe paths, and report serialization bounds.
- [x] 7.5 Make minimized failures produce committed `testdata/fuzz` regression inputs with stable issue or verification references.
- [x] 7.6 Add affected-package active fuzzing to the appropriate PR path and all-target medium/long campaigns to nightly and weekly workflows.

## 8. Pilot and Gate Mutation Testing

- [x] 8.1 Pin a Mewt pilot version and document installation, checksum/source, license review, target scope, test commands, and timeout policy.
- [x] 8.2 Generate mutants for capability selection, authorization grouping, execution leases, rollback policy, secret versioning/redaction, dependency ordering, and schema compatibility.
- [x] 8.3 Measure per-package mutant count, baseline test time, campaign duration, mutator relevance, equivalent-mutant rate, and cross-package test needs.
- [x] 8.4 Configure per-target fast tests with a comprehensive fallback and verify that optimization does not miss cross-package kills.
- [x] 8.5 Define reviewed survivor/baseline metadata and prove stable individual-mutant reproduction.
- [x] 8.6 Record the pilot decision; if accepted, add focused changed-critical-package campaigns and a weekly complete critical-logic campaign.
- [x] 8.7 Enforce no new unexplained relevant survivor for new critical logic without using mutation score as a substitute for missing functional tests.
  - The adopted PR diff gate and weekly complete campaign use the checksum-pinned isolated Mewt 3.0.1 executable. The final 29-target candidate caught 1,207/1,207 current high-severity mutants with no survivor or exception; source-obsolete pilot outcomes remain historical evidence and cannot bypass the gate.

## 9. Add Native Performance Benchmarks

- [x] 9.1 Add Go benchmarks with allocation reporting for model parsing and validation at representative artifact sizes.
- [x] 9.2 Add benchmarks for fleet/endpoint composition, schema compatibility variants, and capability-based artifact selection.
- [x] 9.3 Add benchmarks for dependency graph construction, ordering, Check/report construction, and activation coalescing.
- [x] 9.4 Add benchmarks for redaction, JSON/gzip Sync payloads, state-report bounds, and unchanged suppression.
- [x] 9.5 Add benchmarks for secret envelope encryption/rewrap and rollback reservation, encryption, pruning, and cleanup.
- [x] 9.6 Add controlled Postgres integration benchmarks for compiled-artifact lookup, endpoint check-in, telemetry writes, authorization/lease lookup, and Fleet reporting.
  - Ten-sample controlled collections now cover compiled-artifact lookup, 400-endpoint check-in, state-report and bounded inventory writes, 400-endpoint Fleet reporting, and the Change-control/Execution-lease snapshot through rollback-only Postgres transactions.
- [x] 9.7 Pin the benchmark fixture generator and record 10/100/500/1,000-resource inputs without deriving expected behavior from benchmark output.
- [x] 9.8 Add repeated benchmark collection and `benchstat` comparison with separate latency, allocation, payload, and storage metrics.
- [x] 9.9 Publish advisory PR comparisons and controlled-runner gate results without hard-gating noisy shared-runner latency.

## 10. Build Fleet Load and Agent Resource Harnesses

- [x] 10.1 Build a Go load harness that provisions unique endpoint identities and exercises authenticated Sync against the real server and Postgres.
- [x] 10.2 Implement the 400-endpoint steady unchanged workload at the default polling interval with latency, error, CPU, memory, goroutine, database-pool/query, and byte metrics.
- [x] 10.3 Add 400-endpoint simultaneous startup/reconnect and recovery workloads.
- [x] 10.4 Add 400-endpoint release fan-out, endpoint-override, and full-artifact delivery workloads.
- [x] 10.5 Add telemetry-heavy and mixed schema/capability workloads including capability-blocked endpoints.
  - Telemetry-heavy Sync and the authenticated 400-endpoint mixed-capability workload are implemented. Run `capability-mixed-400-20260718-r2` covered five equal compatible, blocked-existing, unmanaged-new, telemetry-carrying, and reconnecting populations with zero errors; detailed process, database, byte, latency, spread, and cardinality evidence is retained in `engineering/testing/capability-delivery-load-evidence-2026-07-18.md`.
- [x] 10.6 Add controlled server and Postgres degradation, overload response, timeout, and recovery workloads.
- [x] 10.7 Add a scheduled 4,000-endpoint comparison workload and label it headroom evidence rather than an advertised support promise.
- [x] 10.8 Add agent full-cycle benchmarks for compliant and drifted artifacts, measuring wall/CPU time, peak RSS, allocations, goroutines, bytes, disk I/O, and rollback storage.
  - The full-cycle benchmark crosses parse, resolve, engine construction, Check/report, and applicable Apply for 10/100/500/1,000-resource artifacts and emits all required custom resource metrics.
- [x] 10.9 Add medium and long soak harnesses that detect monotonic server, database, agent, temporary-file, and rollback growth.
  - The measured soak requires a controlled growth probe and samples server plus both real agent processes, Postgres, temporary storage, retained rows, and rollback state. Nightly medium and weekly long workflows retain the machine-readable result.
- [x] 10.10 Capture bounded redacted CPU, heap, goroutine, trace, query, and system profiles when performance gates fail.
  - Failure capture retains only size-bounded sanitized text/aggregate artifacts and a manifest; the controlled smoke found no raw profile, trace, credential, authorization, private-key, or secret-canary content.

## 11. Implement Sync Load Shaping

- [x] 11.1 Introduce injectable clock and randomness boundaries for polling, retry, overload, lease, and expiry tests.
- [x] 11.2 Add bounded startup delay and stable per-endpoint polling jitter with a documented maximum Sync staleness bound.
- [x] 11.3 Add capped exponential transient-failure backoff with jitter, success reset, and distinct permanent credential/enrollment/validation behavior.
- [x] 11.4 Define and implement authenticated Sync overload signaling with bounded `Retry-After` handling and pending-telemetry retention.
- [x] 11.5 Add deterministic unit/property tests for jitter distribution bounds, backoff cap/reset, permanent failure policy, and overload behavior without wall-clock sleeps.
- [x] 11.6 Prove through the 400-endpoint load harness that coordinated startup and outage recovery do not preserve synchronized request waves.

## 12. Establish Budgets and Activate the Gates

- [x] 12.1 Run controlled benchmark and 400-endpoint baselines on pinned hardware/environment and record complete reproducibility metadata.
- [x] 12.2 Approve initial server latency/error/CPU/memory/database budgets and agent cycle/idle/resource budgets through an OpenSpec update.
- [x] 12.3 Approve per-benchmark relative regression bounds, deterministic shared-runner bounds, and mutation acceptance policy from measured results.
- [x] 12.4 Add nightly workflows for active fuzzing, full containers, VM safety, 400-endpoint load, and medium soak.
  - Active fuzzing, full containers, Vagrant safety, 400-endpoint controlled load, and medium soak are scheduled with explicit time bounds and retained evidence.
- [x] 12.5 Add weekly workflows for complete critical mutation, 4,000-endpoint comparison, long fuzzing, and long soak.
- [x] 12.6 Update the release workflow to require the supported provider matrix, VM safety, mixed-version/migration acceptance, and approved performance comparison.
- [x] 12.7 Add dashboards or retained machine-readable histories for coverage, mutation, benchmarks, load, soak, and flaky-test status.
- [x] 12.8 Document local focused commands, CI ownership, failure triage, baseline updates, exception expiry, and safe environment operation.
- [x] 12.9 Verify the complete foundation from a clean checkout and prove no required gate depends on uncommitted state, developer-global tools, or production credentials.
  - The synthetic clean-checkout proof ran offline from tree `eab3265f9fdcbebc3ba34b55a15557f6210c7397`, stayed clean, and passed the full suite, all 45 fuzz seed corpora, vet, policy gates, workflow and Compose parsing, and strict OpenSpec validation. Mutation used the repository-pinned checksum installer in an isolated checkout; controlled performance used only disposable local credentials.
- [x] 12.10 Mark the applicator umbrella implementation unblocked only after traceability, Godog pilot, provider harness, fuzz discovery, mutation decision, and initial performance budgets are accepted.
  - The acceptance audit records every prerequisite as complete; the umbrella foundation prerequisite is now unblocked without broadening any provider-support claim.
