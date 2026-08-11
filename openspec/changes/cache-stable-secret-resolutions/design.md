## Context

Remotr agents run Check on the current artifact before every Sync. A Check that
references a server-managed secret currently calls `POST /v1/secrets/resolve`
even when the endpoint, artifact, authorization, and active secret version are
unchanged. Each request reopens the server authorization path, reads registry
state, decrypts the value, and writes an audit record. At a 30-second poll
interval, a single endpoint prevents the Neon compute from reaching its
five-minute suspension window.

The unchanged Sync fast path already maintains coordinated global, fleet, and
endpoint authority generations. Its Redis backend makes those generations
shared across serving processes, while its memory backend is permitted only for
one serving process. Secret lifecycle and authorization mutations already
invalidate the relevant authority scope before acknowledging success.

The agent is already the trusted plaintext consumer. It holds resolved material
long enough to inspect or apply a resource, but it does not currently retain
that material between pipeline executions.

## Goals / Non-Goals

**Goals:**

- Make a stable post-prime Check perform zero secret-resolution HTTP requests.
- Invalidate all cached results before Apply after the server reports a changed
  authority token.
- Bound plaintext by entry count and byte count and clear retained byte slices
  on invalidation, replacement, and eviction.
- Cache an authorization denial without retaining its response body so a
  misconfigured endpoint does not write an audit record every 30 seconds.
- Preserve correctness across supported single-process memory and
  multi-process Redis deployments.
- Fail closed when a stable authority token cannot be produced.

**Non-goals:**

- Persist plaintext secret material to disk or to server-side Redis.
- Remove the server's per-resolution authorization checks.
- Change the agent pull interval or Neon suspension setting.
- Share cached plaintext between processes or endpoints.
- Promise cache reuse across an agent restart.

## Decisions

### The agent caches server-managed resolutions in bounded process memory

The Remotr provider is wrapped by an authority-aware resolver. Its key contains
the reference, artifact digest, resource address, and purpose. Endpoint identity
is bound by the mTLS client and the process that owns the cache. Successful
values are copied into and out of the cache. Authorization denials retain only
the denial class.

The cache uses deterministic least-recently-used eviction and enforces both an
entry limit and a material-byte limit. Local-file secrets bypass it because the
server token says nothing about local filesystem authority.

This location removes the network, database, decrypt, and audit work together.
A server-side plaintext cache was rejected because it would retain material in
an additional trust domain and would still incur network and audit traffic.

### Sync carries an opaque token derived from the authority snapshot

Every stable authenticated Sync response carries
`secretAuthorityToken`. The token is a SHA-256 digest over a versioned encoding
of the backend epoch and the endpoint's global, fleet, and endpoint authority
generations. It reveals neither the endpoint identity nor secret metadata.

The in-memory backend creates a cryptographically random process epoch. The
Redis backend creates a random shared epoch with `SET NX`; all serving processes
read the same value. Redis loss or flush creates a new epoch. Generation changes
cover release, targeting, enrollment, secret lifecycle, and authorization
mutations already routed through the fast-path mutation boundary.

An unstable or unavailable authority snapshot yields no token. The agent treats
a missing token as cache-disabled and clears retained entries. This is the
fail-closed path.

A TTL-only design was rejected. With endpoints staggered across a fleet, TTL
refreshes can still keep Neon continuously active, and a TTL weakens revocation
freshness without proving that authority is unchanged.

### Token observation invalidates before artifact Apply

The agent records the token immediately after a successful Sync and before it
handles an artifact or execution lease. A changed or missing token clears every
cached server-managed result, including denials. The first subsequent use must
contact the authenticated resolver again.

Check occurs before Sync so its report can be included in that request. A server
mutation can therefore leave one read-only Check using the prior value until the
next pull observes the new token. Apply never begins with that stale cache after
the changed Sync response is observed.

Reordering Check after Sync was rejected because it would delay compliance
reporting by another cycle or require a second Sync request.

### Compatibility is additive

The response field is optional. Older agents ignore it. A new agent connected
to an older server receives no token and therefore continues resolving every
use. This preserves behavior until both sides are upgraded.

## Risks / Trade-offs

- Plaintext remains resident longer than one pipeline execution. Bounds, copy
  isolation, byte clearing, and process-only lifetime limit that exposure.
- Go and the operating system may retain copies outside the controlled byte
  slices. The implementation makes a best effort and does not claim guaranteed
  physical erasure.
- A missed mutation boundary could permit stale reuse. Tests enumerate all
  mutation classes, and absence or instability of the authority backend disables
  reuse.
- A first request after token change creates a normal resolver audit event. This
  is intentional evidence of the new secret use.
- Multiple simultaneous misses may duplicate one resolution. The initial design
  does not add singleflight because agent pipeline execution is serial. Bounds
  and correctness do not depend on request coalescing.

## Migration Plan

1. Deploy the server field and shared authority epoch while agents still ignore
   it.
2. Release the agent with the authority-aware cache disabled whenever the field
   is absent.
3. Canary one endpoint with a server-managed secret and confirm one resolver
   event at prime, zero on stable cycles, and one after a test rotation.
4. Roll out the agent normally and observe resolver request rate, audit volume,
   Neon active hours, and cache invalidation metrics.
5. Roll back the agent independently if needed. Rolling back the server is also
   safe because the agent clears its cache when the token disappears.

## Open Questions

- Production cache bounds may be tuned after measuring the maximum number and
  size of secret references in real artifacts. Correctness does not depend on
  those values.
- A future protocol could move Check after Sync if same-cycle compliance
  reporting is redesigned, eliminating the one read-only stale Check window.
