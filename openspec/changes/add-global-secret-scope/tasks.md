## 1. Scope Model and Persistence

- [x] 1.1 For OS-LSM-061/062, name the Admin API/CLI public seam and add one focused failing table test for explicit `global`, omitted, malformed, and identifier-bearing global scope inputs; then implement the canonical scope discriminator and validation to make it pass.
- [x] 1.2 Add a bounded fuzz property for scope/schema parsing that accepts only canonical global/Fleet/Endpoint combinations and never turns omission or malformed input into global scope.
- [x] 1.3 For OS-LSM-063, add a failing registry seam test proving a later version cannot change a logical secret's scope; then enforce immutable scope with the minimum repository/service change and cover negative and boundary cases.
- [x] 1.4 Add the database migration and constraints for the scope discriminator, backfill valid existing Fleet/Endpoint records, and add migration tests that reject neither/both-scope legacy rows and preserve authenticated envelope metadata.
- [x] 1.5 Add persistence round-trip and secret-canary tests proving global metadata survives create/list/restart while plaintext remains encrypted, redacted, and absent from database-safe/API/CLI output.

## 2. Operator and Administration Surfaces

- [x] 2.1 For OS-LSM-069/073, add failing Admin API tests for a bounded logical-secret collection that requires no ID, returns only classified safe summaries, paginates deterministically, and omits secrets outside the caller's scope; then implement the collection query and representation.
- [x] 2.2 Add an explicit server-wide global-secret permission to create, activate, revoke, delete, and abandon recovery; prove OS-LSM-066 through an authorized Admin API integration test and indistinguishable denial cases for Fleet-only operators.
- [x] 2.3 For OS-LSM-069/070, add failing CLI seam tests that make `secret list` enumerate logical secrets and `secret show <id>` display version metadata in stable table and structured output; implement the commands and actionable migration error/guidance for legacy `secret list LOGICAL-NAME` use.
- [x] 2.4 For OS-LSM-071/072, add pseudo-terminal tests for picker selection, empty inventory, cancellation, stale selection, and safe labels, plus non-TTY/structured-output tests proving omitted IDs fail promptly; then connect `secret show` to the standard picker.
- [x] 2.5 Extend operator CLI secret mutations with opt-in global selection, mutually exclusive scope flags, and safe impact output; add command-level tests that assert no secret material enters argv, diagnostics, config, or retained output.
- [x] 2.6 Extend desktop secret parity to select and render global scope only if global administration is in release scope; otherwise document and test the intentional temporary parity boundary called out by the design.

## 3. Authenticated Resolution

- [x] 3.1 For OS-LSM-067, add a failing authenticated Sync/resolution integration test in which endpoints from two fleets resolve one global version through their respective active artifacts; implement global as only a satisfied fleet-membership predicate and make the test pass.
- [x] 3.2 For OS-LSM-015/016/068, add focused negative tests for wrong endpoint, inactive/mismatched artifact digest, wrong resource address, wrong purpose, revoked version, and inactive rollout, asserting one bounded denial shape and zero material/existence/scope leakage.
- [x] 3.3 Add focused mutation evidence for the authorization conjunction so removing or negating any endpoint, artifact, address, purpose, version-status, or rollout predicate is detected; record only reviewed expiring exceptions if a mutant is truly equivalent.
- [x] 3.4 Add a regression test proving Fleet- and Endpoint-scoped references retain their current authorization behavior and that lookup never falls back from an unavailable scoped secret to a global secret.

## 4. Global Lifecycle and Rollout Planning

- [x] 4.1 For OS-LSM-021/064/074, add a failing Admin API integration test that discovers high-risk active consumers, including Ubuntu Pro consumers in multiple fleets, and creates canonical Change requests plus exact per-Fleet/resource rollout bindings before one activation generation commits; implement bounded indexed discovery and atomic planning.
- [x] 4.2 For OS-LSM-074/075, add activation negative tests for incomplete discovery, unauthorized uses, stale artifacts, duplicate or omitted consumers, planner/persistence failure, and high-risk uses without a Change-request binding, proving failure leaves the prior active version and generation unchanged while complete lower-risk bindings may commit without a request.
- [x] 4.3 For OS-LSM-076, add a failing authenticated resolution test showing that an `active` use with no exact binding is denied; implement explicit match-required validation and require an active Change-control gate for every matching high-risk binding.
- [x] 4.4 For OS-LSM-065, add a failing persistence test for a rollback reference retained by another fleet; implement cross-fleet retention/deletion safety and verify authorized abandonment, expiry, cleanup, and audit behavior.
- [x] 4.5 Add revoke and rotation regression tests showing one global version history, monotonic activation generations, no remote-erasure claim, and safe affected-fleet counts without inaccessible usage detail.
- [x] 4.6 For OS-LSM-077, add a failing authenticated Sync test proving an exact current state report can bootstrap the execution lease before artifact acknowledgement; implement the bounded digest-based bootstrap and retain negative stale-digest and stale-release cases.

## 5. Ubuntu Pro Shared Enrollment Token

- [x] 5.1 For OS-UPM-041, add a public configuration plus authenticated provider-contract test using two fleets and one synthetic global Ubuntu Pro token; prove exact protected stdin attachment, successful second Checks, and no fleet-scoped secret copies before implementing the consumer path.
- [x] 5.2 For OS-UPM-065, add wrong-purpose/resource/artifact denial tests and a secret-canary sweep across argv, environment, temporary files, desired state, hashes, plans, audit, logs, reports, errors, rollback state, Sync, persistence, and retained evidence.
- [x] 5.3 Extend the pinned Ubuntu VM qualification fixture for the globally scoped synthetic token path and cleanup evidence without claiming use of a live Canonical token or entitled native effects.
- [x] 5.4 For OS-LSM-078, add a failing provider-contract test for LF- and CRLF-terminated Ubuntu Pro token files; normalize exactly one terminal line ending at the protected API boundary and retain complete original-buffer zeroization.

## 6. Performance, Documentation, and Traceability

- [x] 6.1 Add native benchmarks with allocation reporting for global consumer discovery, activation planning, and authenticated resolution using representative fleet/reference counts; compare against the checked-in controlled baseline.
- [x] 6.2 Add an authenticated load-harness scenario for simultaneous multi-fleet global activation/resolution, using injected clock/randomness where applicable and asserting bounded query/load behavior without wall-clock sleeps in deterministic tests.
- [x] 6.3 Update CLI, secret-management, configuration-format, HTTP API, authorization, deployment/migration, desktop parity, and Ubuntu Pro documentation with `secret list`/`secret show [id]`, non-interactive behavior, opt-in global examples, activation fail-closed semantics, blast-radius guidance, and migration procedures.
- [x] 6.4 Map OS-LSM-061 through OS-LSM-076 and OS-UPM-041/065 to committed evidence in `test/traceability.yaml`, update qualification rows, and validate no evidence was weakened or silently replaced.
- [x] 6.5 Run focused tests after every red/green slice, then the selected database/provider/VM/benchmark/load/mutation checks, `make test`, and relevant quick end-to-end suite; save bounded red/green and final evidence under the change.
- [x] 6.6 Record the post-authorization lease-bootstrap regression, race, full-suite, strict OpenSpec, and traceability evidence discovered during rollout validation.
- [ ] 6.7 Record focused provider, race, full-suite, strict OpenSpec, traceability, release, and live rollout evidence for terminal enrollment-token normalization.
