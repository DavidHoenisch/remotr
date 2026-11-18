## 1. Persistence

- [x] 1.1 Add migration `021` for `endpoints.enrollment_state` (default `enrolled`, CHECK `reserved|enrolled`) and nullable `enrollment_tokens.endpoint_id` FK with `ON DELETE CASCADE`; update `sql/schema.sql` and regenerate queries; add a migration test proving existing endpoints and tokens are unchanged.
- [x] 1.2 Add store operations for reserve (insert reserved row + revoke outstanding bound tokens + insert bound token in one transaction), atomic bound claim during redemption, and unreserve (reserved rows only), with persistence tests for same-fleet re-issue, cross-fleet conflict, enrolled conflict, and cascade cleanup.

## 2. Admin API and operator CLI

- [x] 2.1 For OS-ENR-001/002/004, add one failing Admin API test for `POST /v1/admin/endpoint-reservations` that returns a bound token and a `reserved` endpoint; implement the route with the enrollment-token RBAC permission; add table cases for invalid ID, unknown fleet, unauthenticated, and unauthorized callers.
- [x] 2.2 For OS-ENR-003/011/012, add failing Admin API tests for reserving an enrolled ID (409), re-issuing in the same fleet (old token rejected, new token accepted), and reserving into a different fleet (409); implement them.
- [x] 2.3 For OS-ENR-013/014, add failing Admin API tests for `DELETE /v1/admin/endpoint-reservations/{id}` on reserved (removed, token dead) and enrolled (409) endpoints; implement them.
- [x] 2.4 For OS-ENR-017/018, add failing Admin API and CLI tests showing that unreserve with a compiled override in the current release returns 409 with the explicit error (endpoint ID, release, `endpoints/<id>/`, blocked-Git-sync consequence, `--force`), and that `--force`/`?force=true` removes the reservation, prints the warning, and audits it; implement them.
- [x] 2.5 For OS-ENR-015, add failing CLI seam tests for `remotr endpoint reserve`, `remotr endpoint unreserve [--force]`, and the `STATE` column / `enrollment_state` JSON field in `endpoint list`; implement the CLI and admin client; add secret-canary assertions that bound tokens are absent from list output, logs, and audit records.
- [x] 2.6 Audit endpoint queries (fleet counts, upgrade targeting, delivery state, cron scheduling, desktop views; no desktop UI changes) for `reserved` rows; add regression tests that reserved endpoints are excluded from rollout and compliance views.

## 3. Enrollment

- [x] 3.1 For OS-ENR-005, add a failing authenticated enrollment integration test in which an agent sends its hostname-derived ID with a bound token and receives the bound ID; then implement the bound claim in `handleEnroll`, so that token consumption and claim happen in one transaction and the ignored agent ID is recorded in the audit event.
- [x] 3.2 For OS-ENR-006, add negative enrollment tests for reused, expired, and revoked bound tokens, proving the endpoint state and credentials are unchanged.
- [x] 3.3 For OS-ENR-007, add negative enrollment tests in which an unbound enrollment token, a deployment token, and a token bound to another ID request a reserved ID; prove the reservation stays claimable.
- [x] 3.4 Add a deterministic concurrency test showing that two simultaneous redemptions of one bound token enroll exactly one endpoint.
- [x] 3.5 For OS-ENR-016, add regression tests for unbound and deployment-token enrollment with unreserved IDs, and for override-before-fleet selection on enrolled endpoints.

## 4. Composition and Sync

- [x] 4.1 For OS-ENR-008, add a failing authenticated integration test: reserve, run Git sync with `endpoints/<id>/` present (it succeeds), enroll with the bound token, and confirm the first Sync returns the override artifact digest rather than the fleet digest; make it pass.
- [x] 4.2 For OS-ENR-009, add a failing Git sync test for an override whose ID is neither enrolled nor reserved, asserting a blocked release and an error naming the ID and the reserve remedy; implement the error mapping in `ComposeAll`.
- [x] 4.3 For OS-ENR-010, add an authenticated Sync negative test proving a reserved endpoint cannot Sync.

## 5. End-to-end and documentation

- [x] 5.1 Extend `test/e2e` (or the Godog `enrollment_sync.feature`, only if the declarative step vocabulary already supports it) with the one-step workflow: reserve, merge override, enroll, first Sync delivers the override.
- [x] 5.2 Update `docs/guides/configuration-repository.md` ("Add an endpoint override"), `docs/guides/endpoint-management.md`, and `docs/guides/agent-deployment.md` with the reserve → merge → enroll workflow and the unreserve caveat.
- [x] 5.3 Run `make test`, `make gosec`, and `make test-e2e-quick` against a fresh stack; run traceability lint for the `OS-ENR` IDs.
