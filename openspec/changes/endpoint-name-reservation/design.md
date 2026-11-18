## Context

Enrollment (`POST /v1/enroll`, `internal/server/enroll.go`) redeems a one-time
enrollment token or a reusable deployment token, both bound to a fleet, then
registers the endpoint with the ID the agent sent. The agent sends either an
explicit `--endpoint-id` or a hostname-derived default
(`internal/identity/endpointid.go`) and stores whatever ID the server returns.
Endpoint IDs are already operator-chosen slugs (4–63 `[a-zA-Z0-9-]`).

Git sync composes every `endpoints/*` directory (`configcompose.RenderAll`) and
stores the result in `compiled_artifacts`, whose `endpoint_id` references
`endpoints(id)`. An override for an ID that has not enrolled fails
`ComposeAll`, which blocks release ref advance. At Sync, the server selects the
endpoint artifact before the fleet artifact (`internal/server/composition.go`).

So the only reason a deploy takes two steps is that the endpoint row does not
exist until the machine enrolls.

## Goals / Non-Goals

**Goals:**

- One-step deploys: the override can merge before the VM exists, and the first
  Sync delivers the final configuration.
- The server, not the agent, decides which reserved ID an enrolling machine
  gets.
- No agent change and no change to existing tokens or overrides.

**Non-Goals:**

- Selecting overrides by hostname, labels, or any other agent-reported fact.
- Role/profile overrides shared by many endpoints.
- A separate display name distinct from the endpoint ID.
- Offline `config validate` checks against server reservation state (the CLI
  has no server state; Git sync gives the authoritative error).
- Re-enrolling or re-keying an already enrolled endpoint.
- Desktop admin app support for reservations. This is an intentional temporary
  parity gap; the desktop app shows reserved endpoints only through its
  existing endpoint listing.

## Decisions

### Reserve the endpoint ID itself, not a separate name

IDs are already human-chosen slugs, and overrides, compiled artifacts, Sync
identity, and the CLI are all keyed by ID. A separate name would need a second
uniqueness domain, name↔ID collision rules, and a second override lookup.
Alternative considered: `endpoints/<name>/` resolved through a name column.
Rejected as extra surface with no added capability.

### A reservation is an endpoint row in the `reserved` state

Add `enrollment_state TEXT NOT NULL DEFAULT 'enrolled' CHECK (enrollment_state
IN ('reserved','enrolled'))` to `endpoints`. A reserved row has a fleet and a
NULL `cert_fingerprint` (already nullable). Because the row exists, the
`compiled_artifacts` foreign key holds and Git sync composes the override with
no change to composition or Sync selection. Since the row has no certificate,
mTLS Sync cannot authenticate as it.

Alternatives considered:
- A separate `endpoint_reservations` table plus relaxing the artifact foreign
  key. Rejected: it needs a second key space for artifacts and a claim-time
  copy.
- Skipping unknown override dirs at sync and composing on enrollment. Rejected:
  Sync would fall back to the stored fleet artifact and deliver the wrong
  configuration.

Endpoint queries that assume enrollment (list, fleet counts, upgrade targeting,
delivery state) must treat `reserved` rows correctly. Listing shows them with
their state; scheduling, upgrade, and compliance views exclude them.

### Bind single-use enrollment tokens to an endpoint

Add a nullable `endpoint_id` column to `enrollment_tokens` that references
`endpoints(id)` with `ON DELETE CASCADE`. Reservation runs one transaction:
insert the reserved row, revoke any outstanding bound tokens for the ID, and
insert a new bound token (same 24-byte format and default 7-day TTL as
`enroll token create`). Deployment tokens stay unbound; they are reusable and
cannot represent a one-machine claim.

### Claim atomically at enrollment; the server's ID wins

`handleEnroll` redeems the token. If the token is bound, the server ignores the
request's `endpoint_id` and claims the reserved row in one statement:
`UPDATE endpoints SET enrollment_state='enrolled', cert_fingerprint=… WHERE
id=$bound AND fleet=$fleet AND enrollment_state='reserved'`. Token consumption
and the claim happen in the same transaction, so a failed claim does not burn
the token and two concurrent redemptions cannot both succeed. The response
already carries `endpoint_id`, and the agent saves the returned ID, so older
agents work unchanged. The ignored agent-supplied ID is recorded in the audit
event for diagnosis.

For unbound tokens, the existing `ON CONFLICT (id) DO NOTHING` insert already
rejects a reserved ID with `ErrEndpointExists`. The spec makes that behavior
contractual and covered by a test.

Alternative considered: reject a mismatched agent-supplied ID. Rejected: agents
always send a hostname-derived default, so every bound enrollment without an
explicit `--endpoint-id` would fail.

### Admin API and CLI

- `POST /v1/admin/endpoint-reservations` `{fleet, endpoint_id, ttl}` returns
  `{endpoint_id, fleet, token, expires_at}`. Reserving an existing
  reservation in the same fleet re-issues the token; reserving an enrolled ID or
  an ID reserved in another fleet returns 409.
- `DELETE /v1/admin/endpoint-reservations/{endpoint_id}` deletes only a
  `reserved` row (409 otherwise). Bound tokens cascade.
- Permission: the same RBAC permission as enrollment token creation.
- CLI: `remotr endpoint reserve --fleet F --id ID [--ttl] [--out FILE]`,
  `remotr endpoint unreserve ID`; `endpoint list` gains a `STATE` column and an
  `enrollment_state` JSON field.
- Both mutations emit audit events without token values.

### Actionable composition error

When storing an endpoint artifact fails on the foreign key, `ComposeAll`
returns `endpoint <id> override: endpoint is neither enrolled nor reserved;
run 'remotr endpoint reserve --fleet <fleet> --id <id>'`. Behavior does not
change (release advance stays blocked); only the error does.

## Risks / Trade-offs

- [Unreserving while `endpoints/<id>/` is still in Git blocks the next Git
  sync] → `unreserve` returns 409 with an explicit, stable error if the current
  release has a compiled artifact for the ID, for example:
  `endpoint gitlab-runner-01 has an override in release <ref>
  (endpoints/gitlab-runner-01/); unreserving will block Git sync until that
  directory is removed. Remove it first or re-run with --force`. `--force`
  (`?force=true`) unreserves anyway, and the CLI prints the same consequence as
  a warning. Both outcomes are audited.
- [Reserved rows are counted as real machines in fleet views or upgrade
  rollouts] → audit every endpoint query for state handling and add regression
  tests for list, fleet counts, and upgrade targeting.
- [A leaked bound token lets an attacker claim that endpoint's config, including
  secret refs] → same exposure as an unbound token for that fleet, narrowed to
  one ID and single use; TTL default 7 days; re-issue revokes it.
- [A stale reservation is never claimed] → reservations are visible in
  `endpoint list` with state; the token expires while the row stays for explicit
  cleanup, so a pending Git override is never silently broken.

## Migration Plan

- Migration `021`: add `endpoints.enrollment_state` (default `enrolled`, so
  existing rows are unchanged) and `enrollment_tokens.endpoint_id` (nullable
  FK with cascade). Both additions are backward compatible.
- Rollback: drop the column only after deleting reserved rows. Older servers
  ignore the new columns, but they would treat reserved rows as enrolled
  endpoints with no certificate, so delete reservations before downgrading.

## Open Questions

None. Resolved: `unreserve --force` exists, and the conflict returns an
explicit error; desktop parity is out of scope.
