## ADDED Requirements

### Requirement: Stable server-managed secret resolution reuse

The agent SHALL reuse a successful server-managed secret resolution only when
an authenticated Sync response supplies the same non-empty opaque authority
token and the reference, artifact digest, resource address, and purpose are
identical (OS-LSM-032).

#### Scenario: Unchanged Check reuses the primed resolution

- **WHEN** two Check executions use the same server-managed secret scope and
  observe the same non-empty authority token
- **THEN** the first execution SHALL resolve through the authenticated endpoint
  API and the second execution SHALL issue no secret-resolution request

#### Scenario: Local-file secret bypasses server-authority reuse

- **WHEN** a Check resolves a local-file secret
- **THEN** the agent SHALL use the local-file provider without placing its
  material in the server-authority cache

### Requirement: Authority changes invalidate before Apply

The server SHALL change the endpoint's opaque secret-authority token when any
global, fleet, or endpoint mutation can affect the release, targeting,
enrollment, secret lifecycle, or authorization state used to resolve that
endpoint's secrets (OS-LSM-033).

#### Scenario: Rotation forces a fresh resolution

- **WHEN** a secret is activated, revoked, or deleted and the endpoint observes
  the resulting authority token in Sync
- **THEN** the agent SHALL clear prior results before Apply and the next use
  SHALL resolve through the authenticated endpoint API

#### Scenario: Authority is unstable or unavailable

- **WHEN** the server cannot produce a stable coordinated authority snapshot
- **THEN** it SHALL omit the token and the agent SHALL clear retained results
  and resolve through the authenticated endpoint API

#### Scenario: Supported multi-process deployment

- **WHEN** multiple serving processes use the Redis fast-path backend
- **THEN** every process SHALL derive the same token for the same authority
  snapshot and a Redis reset SHALL produce a different token

### Requirement: Cached plaintext is bounded and cleared

The agent SHALL retain server-managed plaintext only in process memory, SHALL
enforce configured entry and material-byte bounds, and SHALL clear controlled
material byte slices before invalidation, replacement, or eviction
(OS-LSM-034).

#### Scenario: Cache bounds are exceeded

- **WHEN** inserting a resolution would exceed the entry or material-byte bound
- **THEN** the agent SHALL evict deterministic least-recently-used entries or
  decline to cache the new result while preserving correct resolution behavior

#### Scenario: Returned material is modified

- **WHEN** a caller modifies the returned material bytes
- **THEN** the cached copy SHALL remain unchanged

### Requirement: Stable authorization denials do not repeat remotely

The agent MAY cache only the `unauthorized` error class for an otherwise valid
server-managed resolution scope while the same non-empty authority token is
current and SHALL NOT cache transient, malformed, or server errors
(OS-LSM-035).

#### Scenario: Unauthorized scope remains unchanged

- **WHEN** the resolver returns unauthorized and the next Check uses the same
  scope and authority token
- **THEN** the next Check SHALL return unauthorized without an HTTP request and
  without retaining the server response body

#### Scenario: Token changes after an authorization denial

- **WHEN** a different authority token is observed after a cached denial
- **THEN** the next use SHALL retry the authenticated endpoint API

#### Scenario: Transient resolver failure

- **WHEN** the resolver returns a transport error or non-authorization server
  error
- **THEN** the agent SHALL NOT cache that failure
