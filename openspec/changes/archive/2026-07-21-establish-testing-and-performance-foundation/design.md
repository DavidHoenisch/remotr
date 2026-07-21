## Context

Remotr currently has a fast Go unit suite, a small Compose E2E suite, and nine native fuzz targets. The measured baseline is 122 test files, 380 test functions, seven E2E tests, no benchmarks, no Gherkin features, and 35.6% overall statement coverage. The suite passes with coverage in about six seconds and with the race detector in under thirty seconds on the current development host, so a broad PR gate is affordable.

The assurance is uneven. Decision-heavy packages such as the agent engine have meaningful unit coverage, while server persistence, system applicators, and real provider behavior are much less exercised. General tests run on release tags rather than every pull request. E2E, fuzz campaigns, race detection, and security analysis are not continuous repository-wide gates. The fuzz script also names a removed target and `go test -fuzz` treats the missing match as a successful warning, demonstrating that a handwritten target list can silently stop testing code.

The applicator umbrella change contains 161 requirements and 224 scenarios across twelve capabilities. Those scenarios resemble specification-by-example but are Markdown OpenSpec artifacts, not directly executable Gherkin. Mechanically duplicating all of them into `.feature` files would create two sources of truth and an expensive step-definition surface. At the same time, command-mock unit tests cannot prove Linux convergence, safety recovery, operator workflows, or behavior under a synchronized fleet.

The current agent polls on a fixed 30-second ticker and performs an immediate first Sync. Without jitter or failure backoff, endpoints deployed together can remain synchronized. The server Sync path performs artifact lookup plus multiple telemetry, cron, policy, upgrade, and diagnostic operations, making concurrency and Postgres behavior part of the product contract rather than a late optimization concern.

Stakeholders are contributors, reviewers, release operators, fleet administrators, and AI implementation agents. The foundation must keep fast local feedback while making unsupported behavior, safety gaps, and regressions difficult to merge or advertise.

## Goals / Non-Goals

**Goals:**

- Make every OpenSpec scenario traceable to planned or implemented verification evidence without requiring a one-to-one Godog test.
- Define stable public seams so tests survive internal refactoring and measure behavior that callers or operators observe.
- Use selective Godog scenarios as living acceptance documentation for cross-component, safety-critical workflows.
- Establish strict red-before-green vertical-slice TDD rules suitable for human and AI-assisted implementation.
- Test provider contracts exhaustively in-process and prove advertised Linux behavior in real container or VM environments.
- Continuously exercise races, fuzz properties, and mutation sensitivity according to cost and risk.
- Establish reproducible server and agent benchmarks, 400-endpoint load tests, future-scale comparisons, and soak tests before M1–M5 expansion.
- Prevent synchronized polling and unbounded retry behavior from amplifying fleet load.
- Ratchet quality from the measured baseline without incentivizing meaningless coverage or brittle implementation-coupled tests.

**Non-Goals:**

- Converting all OpenSpec scenarios into Gherkin.
- Treating statement coverage as proof of correctness or imposing one repository-wide percentage on generated code, binaries, adapters, and decision logic alike.
- Running destructive, reboot, or connectivity-loss tests on contributor workstations or shared production infrastructure.
- Replacing Go's `testing` package, native fuzzing, or ordinary table-driven tests with a BDD framework.
- Mocking entire Linux distributions or claiming command construction alone proves convergence.
- Setting permanent absolute latency and resource SLOs before a reproducible baseline is measured on controlled hardware.
- Implementing any M1–M5 applicator behavior as part of this foundation change.

## Decisions

### 1. OpenSpec remains authoritative and scenarios receive immutable verification IDs

Every scenario in an active or archived OpenSpec capability receives a nearby comment of the form `<!-- verification-id: OS-AEC-001 -->`. The identifier remains stable when prose or titles change and is never reused after removal. Capability prefixes are registered centrally to prevent collision.

A machine-readable `test/traceability.yaml` indexes each identifier with source change/capability, lifecycle status, verification class, test selectors, required environments, and an optional deferral reason. A generated check parses OpenSpec instead of relying on a handwritten scenario list. It fails for missing, duplicate, reused, or orphaned identifiers.

`planned` is valid while its feature is unimplemented but is not evidence. A scenario governing advertised behavior must be `verified` and point to passing automated evidence. `deferred` and `not-applicable` require a durable reason and cannot be used for advertised behavior. One test may verify multiple scenario IDs and one scenario may require multiple layers.

Why: traceability gives complete accounting while allowing each requirement to be tested at its cheapest trustworthy seam.

Alternative considered: infer identity from headings or content hashes. Rejected because harmless wording changes would break identity.

Alternative considered: make every OpenSpec example a Godog feature. Rejected because 224 duplicated scenarios would create competing specifications and high glue-code cost.

### 2. Tests target seven agreed public seams

Tests are assigned to one of these observable boundaries before implementation begins:

1. Configuration authoring: repository input through `remotr config validate`, render, or discover output.
2. Operator administration: CLI or authenticated Admin HTTPS API behavior.
3. Agent/server protocol: authenticated Sync request, response, and reported state transitions.
4. Agent execution: composed artifact plus endpoint facts and controlled host boundaries to structured Check/Apply results.
5. Provider contract: declared resource intent to observed real or sandboxed host state.
6. System safety: complete recovery behavior across server, agent, Linux, network, reboot, or storage boundaries.
7. Performance: externally observable request/cycle cost and bounded resource consumption.

Tests SHALL prefer exported or process-level behavior. Test-only accessors are allowed only when they expose an intentionally stable diagnostic seam, not private algorithm structure. Exact argv tests remain valid at the external-process boundary when quoting, noninteractive operation, or forbidden flags are the behavior under test.

Why: pre-agreed seams keep TDD focused and prevent AI-generated tests from merely mirroring implementation structure.

### 3. Godog is a selective acceptance layer integrated with `go test`

Godog is vendored at a pinned reviewed version and wrapped in `test/acceptance` so its pre-1.0 API does not spread through production packages. Features use domain language and stable tags such as `@os_OS-AEC-001`. Step definitions invoke the CLI, HTTPS APIs, Sync protocol, or a deliberately public application seam; they do not call private helpers or query persistence as a verification shortcut.

Initial tracer features cover currently implemented public workflows: configuration validate/render, operator bootstrap and endpoint listing, agent enrollment plus authenticated Sync, app-package listing, and endpoint-label reporting. Capability-blocked artifact delivery, high-risk rollout/baseline authorization, connectivity rollback acknowledgement, secret upload/activation, and coordinated reboot acknowledgement remain future acceptance candidates and SHALL be introduced only with their corresponding public M1–M5 behavior. The pilot must demonstrate independent scenarios, comprehensible failures, bounded step vocabulary, and acceptable PR runtime before expansion.

Provider permutations, parser edge cases, cryptographic properties, retention boundaries, and command construction stay in Go unit, contract, property, fuzz, or integration tests.

Why: these scenarios are valuable as living operator-facing documentation, while lower layers remain faster and more exhaustive.

### 4. TDD proceeds one vertical slice at a time

Before production code is changed, the implementation task names its OpenSpec verification IDs and seams. The developer or agent adds one failing behavioral test, runs it, and records that it failed for the intended missing behavior. Only the minimum implementation required to pass is then added. Negative cases, provider evidence, fuzz properties, mutation checks, and benchmarks follow according to the risk classification before the slice is complete.

Pull-request evidence records the focused red command/result, the final verification selectors, mutation outcome for new critical logic, and benchmark comparison when a hot path changes. CI cannot prove chronological authorship, so mutation sensitivity, reviewable evidence, and prohibition on weakened tests provide defense in depth.

Tests may be deleted or materially weakened only with the associated OpenSpec change and traceability update. Undocumented skips, automatically regenerated expectations derived from current output, and mocks of owned internal collaborators are rejected.

Why: horizontal batches of imagined tests and later implementation are particularly vulnerable to tautological AI output.

### 5. CI uses cost-aware gates rather than one monolithic suite

- Every PR: formatting/vetting, OpenSpec validation, traceability lint, ordinary Go tests, coverage collection/ratchet, race detection, fuzz seed corpora, Godog in-process acceptance, and affected provider contract tests.
- Affected PR or protected pre-merge queue: Compose integration and applicable container-provider matrix.
- Nightly: active fuzz campaigns, full container matrix, VM safety suites, 400-endpoint load tests, and medium soak tests.
- Weekly: full critical-package mutation campaigns, future-scale load comparison, and long soak tests.
- Release: repeat all required deterministic gates, full supported provider matrix, VM safety, migration/mixed-version acceptance, and approved performance baseline comparison.

The target fast PR path is ten minutes or less. Expensive jobs may run concurrently and a protected merge queue may finish environment tests, but required evidence cannot be skipped because an ordinary PR job is slow.

Flaky tests are defects. A failing test may be quarantined only with a tracked owner, incident, expiry, and an equivalent release-blocking signal; silent retries do not convert a flaky result into success.

Why: feedback speed matters, but moving critical assurance entirely out of the merge path makes it advisory.

### 6. Coverage is a navigation and ratchet signal; mutation measures sensitivity

The repository records package and changed-line coverage, excludes generated code explicitly, and initially prevents unexplained regression from the measured baseline. Decision-heavy security, authorization, selection, retention, ordering, and validation modules receive risk-based floors after their first complete vertical slice. Coverage never substitutes for provider, safety, or mutation evidence.

A pinned Mewt 3.0.1 gate targets capability selection, authorization grouping, execution leases, rollback policy, secret versioning/redaction, dependency/activation ordering, and schema compatibility. The repository approves the verified unmodified AGPL executable only as an isolated CI/development process: it is not vendored, linked, distributed, shipped in an image, or exposed as a network service. Any change to that boundary requires a new license review.

Current high-severity mutants are the blocking relevance class because the pilot demonstrated that they replace error and fail-closed paths across the adopted critical scope. The complete high campaign must have zero unexplained survivor. A weekly comprehensive campaign executes medium and low mutations as retained review evidence; those outcomes do not become accepted equivalents or a mutation-score gate. A lower-severity mutant that exposes a security, authorization, rollback, secret, ordering, or validation behavior is promoted to blocking relevance and requires a killing test or reviewed durable disposition. The source-obsolete 2026-07-11 pilot count is retained as history, not imported as an acceptance baseline for regenerated current mutants.

Why: a line can execute without an assertion capable of detecting incorrect behavior.

Alternative considered: enforce an immediate 80% repository-wide coverage floor. Rejected because the current 35.6% baseline contains heterogeneous adapters and generated packages and would encourage low-value tests.

### 7. Fuzz targets are discovered and properties are durable

The fuzz runner discovers `Fuzz*` targets from Go test metadata or source and fails if configured targets do not exist, if discovered targets are omitted, or if a fuzz invocation reports no matching target. Seed corpora run in every ordinary test invocation. Short active campaigns run on affected PRs when bounded; all targets run nightly and longer security-sensitive campaigns run weekly or continuously when infrastructure permits.

Targets assert invariants such as no panic, bounded resource use, path confinement, round-trip stability, canonical equivalence, redaction, authorization monotonicity, state-machine validity, and parser rejection. A discovered failure is minimized, committed to `testdata/fuzz`, and given a regression identifier before repair.

Why: the current stale target proves a static script can appear green while skipping a fuzz function.

### 8. Provider truth is controlled by a shared conformance harness and real environments

One contract harness exercises compliant, drifted, apply, second-check idempotence, absence, unsupported, probe failure, validation failure, lock contention, activation, redaction, rollback, and cancellation behavior for each provider. Contract cases operate on the public provider interface and use deterministic fake clocks/randomness plus controlled external boundaries.

A versioned matrix declares supported distribution release, architecture, init/network/security backend, provider revision, environment kind, and required test selectors. Container tests cover behavior faithfully exposed by namespaces and installed tools. VM tests cover reboot, network recovery, mounts, kernel modules, MAC, authentication recovery, and other host/kernel behaviors containers cannot prove.

Capability advertisement is generated or validated against passing matrix entries. A schema or provider implementation cannot advertise support based on unit tests alone. VM snapshots, networks, and storage are isolated and destroyed after each scenario.

Why: the capability matrix is a product promise and must be derived from evidence.

### 9. Performance is measured at micro, service, and fleet levels

Native Go benchmarks cover parsing/validation/composition, capability selection, dependency ordering, Check/report construction, redaction, JSON/gzip payloads, secret envelope operations, rollback reservation/pruning, and critical Postgres queries. Fixtures use 10, 100, 500, and 1,000 resources as relevant and report time plus allocations.

Benchmarks run at least ten samples for statistical comparison. Shared PR runners produce advisory time results but may hard-gate deterministic allocation or payload bounds. Stable controlled runners own latency and CPU regression gates. Any statistically significant regression beyond an approved per-benchmark budget requires explanation and an updated baseline; an improvement in one metric cannot silently excuse an unacceptable regression in another.

A Go fleet-load harness creates distinct authenticated endpoint identities and drives the real server/Postgres protocol. Its reference workload is 400 endpoints at the default polling interval. It separately measures steady unchanged Sync, synchronized startup/reconnect, release fan-out, telemetry-heavy requests, mixed capability/schema populations, and degraded database/server recovery. A 4,000-endpoint comparison workload detects nonlinear growth but is not initially an advertised support promise.

Agent tests measure full parse/resolve/Check/report and applicable Apply cycles with representative artifacts, recording wall/CPU time, peak RSS, allocations, goroutines, network bytes, disk I/O, rollback capacity, and idle overhead. Scheduled soak tests check monotonic memory, goroutine, connection, table, and disk growth.

The 2026-07-18 controlled baseline approves a 20% paired latency regression bound, a 10% shared-runner deterministic allocation/byte bound, 350 ms warm and 250 ms unchanged p95 at 400 endpoints with zero errors, 5-second 1,000-resource agent cycles, the measured Postgres maxima, and monotonic-growth limits recorded in `test/performance/budgets.json`. The complete values and units in that strict file are release-blocking. They require an approved OpenSpec update to change.

Why: endpoint count alone hides synchronized bursts, artifact size, telemetry writes, and agent resource cost.

### 10. Sync scheduling is load-shaped and overload-aware

Agents apply bounded random startup delay and stable per-endpoint polling jitter while preserving a documented maximum staleness bound. Transient failures use capped exponential backoff with jitter and reset after success. The server may return an authenticated overload response with `Retry-After`; the agent respects the bound without treating overload as successful convergence. Re-enrollment, credential rejection, and permanent validation failures use distinct retry policy.

Deterministic tests inject clock and randomness sources. Load tests prove that 400 simultaneously started agents spread subsequent requests, recover without permanent synchronization, and do not create retry storms when the server or Postgres is degraded.

Why: a fixed ticker turns coordinated installation and outages into repeatable thundering herds.

## Risks / Trade-offs

- [Traceability becomes clerical overhead] → Generate the inventory, require only classification and evidence selection, and let one test cover multiple identifiers.
- [Godog step definitions become a second application framework] → Keep the initial feature set small, cap vocabulary growth through review, and require steps to use public seams.
- [Pre-1.0 Godog changes break tests] → Pin/vendor it and isolate all integration behind one test package.
- [Mutation campaigns are too slow or noisy] → Pilot critical pure logic, baseline reviewed equivalent mutants, use per-target commands, and schedule full campaigns separately.
- [Coverage gates incentivize meaningless tests] → Ratchet by risk/package and pair coverage with mutation and real-provider evidence.
- [Container tests falsely imply kernel correctness] → Record environment kind in the provider matrix and require VMs for behavior containers cannot reproduce.
- [VM tests damage hosts or leak secrets] → Use disposable isolated snapshots, synthetic secret canaries, bounded networks/disks, and mandatory teardown verification.
- [Performance results fluctuate on shared CI] → Gate latency on controlled runners, use repeated statistical comparison, and keep only deterministic budgets on shared runners.
- [A 4,000-endpoint test is mistaken for support] → Label it comparison/headroom evidence; only separately approved SLOs define supported scale.
- [Ten-minute PR target conflicts with required assurance] → Parallelize and use a protected merge queue without converting required jobs into optional checks.

## Migration Plan

1. Record the current suite, coverage, race runtime, E2E behavior, and known gaps without imposing new numerical gates.
2. Add PR-wide ordinary/race/OpenSpec validation and repair fuzz target discovery so the existing baseline is trustworthy.
3. Add verification IDs to the applicator umbrella scenarios and generate the initial traceability manifest with `planned`, `verified`, or approved deferral states.
4. Introduce the test seam policy, pull-request evidence template, Godog wrapper, and five tracer acceptance workflows.
5. Create the provider conformance harness and initial Debian/Ubuntu/Arch matrix; migrate representative current applicators before gating all advertised providers.
6. Run the mutation-tool pilot, record reviewed baseline survivors, and enable focused gates for new critical logic.
7. Add native benchmarks and the authenticated 400-endpoint harness, measure controlled baselines, then approve initial SLOs and regression budgets.
8. Add container, VM, nightly, weekly, and release workflows; prove isolation and teardown.
9. Make each M1–M5 slice subject to traceability, provider, safety, mutation, and performance requirements appropriate to its risk.

Rollback of the foundation means disabling a newly unstable CI gate while retaining its results, issue, owner, and expiry. Traceability identifiers, committed regression corpora, and accepted evidence are not removed when a runner is temporarily disabled.

## Open Questions

None. Initial performance and mutation policies were approved from the 2026-07-18 controlled baseline. Later changes require retained paired evidence and an OpenSpec update.
