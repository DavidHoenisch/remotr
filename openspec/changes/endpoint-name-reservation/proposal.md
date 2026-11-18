## Why

Deploying a machine that needs an endpoint override takes two steps: the VM
enrolls into its fleet and first receives the plain fleet configuration, then a
second merge request adds `endpoints/<endpoint-id>/manifest.yaml`. The override
cannot merge first because Git sync composes every `endpoints/*` directory into
`compiled_artifacts`, whose `endpoint_id` references an enrolled endpoint;
an override for a not-yet-enrolled ID fails composition and blocks release ref
advance. Endpoint IDs are already operator-chosen slugs, so the only missing
piece is a server-authoritative way to claim an ID before the machine exists.

## What Changes

- Operators can reserve an endpoint ID in a fleet before enrollment:
  `remotr endpoint reserve --fleet <fleet> --id <endpoint-id> [--ttl] [--out]`
  (Admin API `POST /v1/admin/endpoint-reservations`). The server creates the
  endpoint in a `reserved` state and returns a single-use enrollment token bound
  to that fleet and ID.
- Git sync composes and stores override artifacts for reserved endpoints, so an
  `endpoints/<endpoint-id>/` override can merge before the VM exists and the
  first Sync after enrollment delivers the final configuration.
- Enrollment with a bound token claims the reserved endpoint; the server, not
  the agent, decides the ID. Any agent-supplied `endpoint_id` is ignored and the
  response returns the bound ID, which the agent already persists.
- Reserved IDs cannot be claimed with unbound enrollment tokens or deployment
  tokens.
- Reserving an already-reserved ID re-issues its token (revoking the previous
  one); `remotr endpoint unreserve <endpoint-id>` releases an unclaimed
  reservation. Enrolled endpoints cannot be reserved or unreserved.
- `remotr endpoint list` and the Admin API show each endpoint's enrollment
  state (`reserved` or `enrolled`).
- Composition failure for an `endpoints/<id>/` directory that is neither
  enrolled nor reserved reports the ID and the remedy (reserve it) instead of a
  raw foreign-key error.
- Override selection stays keyed by endpoint ID only. Hostnames and
  agent-reported labels never select override paths.
- Backward compatible: existing ID-keyed overrides, unbound enrollment tokens,
  deployment tokens, and agents are unchanged.

## Capabilities

### New Capabilities
- `endpoint-name-reservation`: operator reservation of endpoint IDs before
  enrollment, bound single-use enrollment, composition of overrides for
  reserved endpoints, and the reservation lifecycle (re-issue, unreserve, state
  visibility).

### Modified Capabilities

None. No existing spec governs enrollment or configuration-repository
composition.

## Impact

- Database: migration adding endpoint enrollment state and an optional
  endpoint binding on `enrollment_tokens`.
- Server: `internal/server/enroll.go`, `internal/server/admin.go` (new
  reservation routes), `internal/store/postgres` (reserve, claim, unreserve),
  `internal/server/composition.go` (actionable error).
- Operator CLI and admin client: `cmd/remotr` (`endpoint reserve`,
  `endpoint unreserve`, state column), `internal/admin/client.go`.
- Agent: no change required; it already stores the server-returned endpoint ID.
- Docs: `docs/guides/configuration-repository.md`,
  `docs/guides/endpoint-management.md`, `docs/guides/agent-deployment.md`.
- Traceability: new `OS-ENR` verification-ID prefix in
  `test/traceability-prefixes.yaml`.
