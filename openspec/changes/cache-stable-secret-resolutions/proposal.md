## Why

An unchanged 30-second agent cycle re-resolves every server-managed secret,
causing repeated registry reads, decryptions, and audit writes even though the
secret authority and deployable artifact have not changed. Production logs
show this path keeping Neon continuously active after the unchanged Sync fast
path was deployed.

## What Changes

- Add a bounded, process-memory cache for successful server-managed secret
  resolutions used by the agent's Check and Apply pipeline.
- Add an opaque secret-authority token to Sync responses so an agent can prove
  that a cached resolution remains valid without contacting the secret
  resolver.
- Change the token whenever release, targeting, enrollment, secret lifecycle,
  or authorization authority affecting an endpoint changes, including across
  server restart and supported multi-process deployments.
- Clear cached material before replacement or eviction and never persist
  plaintext secret material to endpoint state.
- Suppress repeated resolver calls for a stable authorization denial while the
  same authority token and resolution scope remain current.
- Add composed-agent, authorization, mutation, restart, resource-bound, load,
  and Neon-idle evidence for the new behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-security-and-secret-management`: Permit bounded endpoint-memory reuse
  only while an authenticated server token proves the secret authority is
  unchanged, with immediate invalidation and plaintext cleanup requirements.
- `performance-and-scale-assurance`: Require stable agent cycles with
  server-managed secrets to perform zero secret-resolution requests and zero
  related database operations after priming.

## Impact

The Sync response schema gains a backward-compatible opaque authority token.
The server authority-generation and Redis coordination paths, agent Sync
state, server-managed secret resolver, composed execution tests, load harness,
performance budgets, protocol documentation, and deployment evidence are
affected. No external service or persistent plaintext cache is introduced.
