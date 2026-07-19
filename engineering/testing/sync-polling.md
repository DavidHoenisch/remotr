# Sync polling policy

The agent starts Sync after a bounded randomized delay, then waits a stable,
endpoint-derived delay after each successful cycle. The delay boundary is
injectable for deterministic tests; production uses timers and process-local
randomness.

For a configured interval `I`, startup delay and stable jitter are each
`min(I / 10, 3s)`. The default 30-second interval therefore has a startup
spread below three seconds and a post-success Sync staleness bound of 33
seconds. Endpoint identity comes from enrolled credentials, then
`REMOTR_ENDPOINT_ID`, then the hostname, so coordinated installs do not retain
the same recurring phase after their first Sync.

Retries and authenticated overload responses use a separate policy and are not
treated as successful cycles. Transient Sync failures start at one second,
double with bounded positive jitter, cap at five minutes, and reset only after
a successful response. Credential, enrollment, and validation statuses retry
on a distinct 15-minute cadence. The server may opt into a bounded
authenticated Sync concurrency limit with `REMOTR_SYNC_MAX_CONCURRENT` and
`REMOTR_SYNC_RETRY_AFTER`; an admitted endpoint receives normal Sync behavior,
while an overloaded authenticated endpoint receives 503 plus `Retry-After`.

The agent preserves pending telemetry on every Sync error because it calls
`Pending.ClearSent` only after a successful response. A positive `Retry-After`
is capped at the five-minute transient maximum; otherwise overload follows the
same transient backoff. Policy tests inject clock and randomness, so they do
not sleep on wall time.
