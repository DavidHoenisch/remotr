## Why

Remotr is about to expand from a small set of Linux applicators into a safety-critical fleet-management system, but its current test suite does not continuously prove that implementation behavior matches OpenSpec, real Linux providers, or fleet-scale performance expectations. A durable verification foundation is required before M1–M5 implementation so AI-assisted and human changes remain behaviorally correct, diagnosable, and operationally safe over time.

## What Changes

- Establish OpenSpec as the requirements source of truth, assign stable scenario identifiers, and require machine-checked traceability from every in-scope scenario to automated evidence or an explicit deferred/not-applicable disposition.
- Introduce selective Godog executable specifications for cross-component, operator-visible, and safety-critical workflows without duplicating every OpenSpec example as Gherkin.
- Adopt a strict vertical-slice TDD policy with agreed public seams, demonstrated red-before-green behavior, behavior-focused assertions, controlled use of test doubles, and completion gates for AI-assisted implementation.
- Add PR, nightly, weekly, and release quality gates covering unit and contract tests, race detection, coverage ratchets, fuzzing, mutation testing, container integration, VM safety, and full-stack acceptance.
- Create one reusable provider-conformance harness and a versioned Debian/Ubuntu/Arch environment matrix that prevents a capability from being advertised until its contract and real-provider tests pass.
- Establish native Go benchmarks, statistically compared regression results, a deterministic authenticated fleet-load harness, agent resource-budget tests, and scheduled soak tests.
- Define a 400-endpoint reference workload, synchronized reconnect and release-fan-out stress cases, and a future-scale comparison workload; measure latency, errors, CPU, memory, allocation, goroutine, database, payload, and endpoint overhead.
- Add bounded startup/poll jitter, failure backoff, and overload signaling requirements so fixed polling intervals do not create avoidable synchronized fleet load.
- Repair the current baseline before feature implementation, including the stale fuzz target, missing PR-wide test workflow, missing benchmarks, and absent mutation/Gherkin infrastructure.

## Capabilities

### New Capabilities

- `specification-test-traceability`: Stable OpenSpec scenario identity, verification classification, executable-specification selection, and machine-enforced evidence mapping.
- `test-quality-gates`: TDD rules and tiered CI policy for unit, contract, Godog, race, coverage, fuzz, mutation, integration, and release verification.
- `linux-provider-conformance`: Reusable provider behavior contracts plus container and VM matrices that gate truthful capability advertisement.
- `performance-and-scale-assurance`: Benchmarks, fleet load and soak testing, resource budgets, regression analysis, and load-shaping behavior for server and agent paths.

### Modified Capabilities

None. The repository has no archived main capability specs yet; the active applicator umbrella change will reference this foundation as a prerequisite.

## Impact

- OpenSpec authoring conventions and the active `expand-linux-system-administration-applicators` change.
- Go test organization, shared test harnesses, build tags, fixtures, and vendored test dependencies such as Godog and the selected mutation tool.
- Make targets and GitHub Actions for PR, scheduled, and release gates.
- Docker Compose provider integration and new isolated VM safety environments.
- Agent Sync scheduling/backoff and server overload responses.
- Server Sync/API paths, Postgres integration, artifact composition, agent engine cycles, and observability used by load tests.
- Contributor and AI-agent implementation rules, pull-request evidence, and task completion criteria.
