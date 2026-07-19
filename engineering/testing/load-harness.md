# Authenticated Sync load harness

`cmd/remotr-load` provisions a fresh one-time enrollment token and unique mTLS
identity for every requested endpoint, then sends one concurrent authenticated
Sync wave through the real server and configured Postgres registry. It does not
approximate fleet activity with health checks or shared credentials.

The command refuses to run without `--allow-load`, a positive `--endpoints`,
and explicit server URL, CA path, Postgres URL, and fleet. Set the equivalent
`REMOTR_LOAD_*` variables only for a disposable environment. The Postgres URL
is used to create one-time enrollment tokens, so it must never reference a
production database. Endpoint identities are prefixed `load-<run-id>-` and
their TLS material remains only in process memory.

Example for an already-running disposable Compose stack:

```sh
REMOTR_LOAD_SERVER_URL=https://localhost:8443 \
REMOTR_LOAD_CA="$PWD/compose/runtime/certs/ca.crt" \
REMOTR_LOAD_DATABASE_URL=postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable \
REMOTR_LOAD_FLEET=test-fleet \
go run -mod=vendor ./cmd/remotr-load --allow-load --endpoints 10 --concurrency 10
```

The JSON result records every client-observed wave: request count, successes,
errors, unchanged responses, request/artifact bytes, and p50/p95/max latency.
It also captures before/after load-generator CPU, RSS, heap, and goroutine
measurements, together with its Postgres pool state and the configured
database's `pg_stat_database` counters (transactions, cache blocks, tuples,
temporary files, deadlocks, and backend count). Database counters are global
to the disposable database, so the raw snapshots remain in the report rather
than being misrepresented as request-exclusive values.

Pass `--steady-cycles N` to send one artifact warm-up wave followed by `N`
unchanged waves; its default `--poll-interval` is the agent's 30 seconds. The
`make load-steady-400` target runs the defined 400-endpoint scenario with a
single default-interval unchanged wave. It still requires the explicit
`REMOTR_LOAD_*` disposable-environment settings and `--allow-load` protection.

`make load-steady-4000` uses the same default-interval steady workload for a
future-scale comparison. The weekly `Fleet load headroom` workflow runs it only
on the labelled controlled runner and retains its JSON result. It is headroom
evidence for nonlinear growth detection, not an advertised supported fleet
size, a capacity SLO, or a pull-request latency gate.

The `Nightly environment verification` workflow runs the 400-endpoint reference
load on the same labelled controlled runner and retains its machine-readable
report. It also runs the container provider matrix and the Vagrant
system-safety fixture on their appropriate environments. Active fuzzing is
scheduled separately; medium/long soak scheduling remains pending its
resource-growth harness.

`make load-startup-reconnect-400` runs the `startup-reconnect` scenario: all
endpoints perform an initial coordinated Sync, the harness closes every idle
client transport, then all endpoints simultaneously reconnect twice while
retaining their last digest and release reference. The result names the three
waves as `simultaneous-startup`, `simultaneous-reconnect`, and
`post-reconnect-recovery`. This is a fresh-client-connection recovery test; it
does not claim to simulate a server or Postgres outage, which is a separate
controlled degradation scenario.

`make load-release-fanout-400` runs the `release-fanout` scenario against the
server's real compiled-artifact store. After baseline delivery, it writes a
new fleet desired artifact under a temporary release ref and advances that
ref, so all endpoints must receive a full artifact. It then writes a desired
artifact override for exactly one load endpoint at the same release; the final
wave should contain one full endpoint-override response and unchanged
responses for the remaining endpoints. The harness restores the original
global release ref before returning, even when the scenario fails.

`make load-telemetry-heavy-400` runs a baseline artifact wave followed by an
unchanged wave carrying bounded, non-secret labels, usernames, system
inventory, drift, and firewall-audit reports. Those are the telemetry fields
that the current server persists.

`make load-capability-mixed-400` assigns 400 authenticated endpoints evenly to
compatible, blocked-existing, unmanaged-new, telemetry-carrying, and
reconnecting populations. Four waves offer and acknowledge a controlled
baseline, advance to a capability-revision target, and reconnect the compatible
recovery population on the agent's stable success cadence. The report includes
aggregate and per-population latency, request/artifact bytes, errors,
`capabilityBlocked`/unmanaged counts, request-start spread, Postgres counters,
and compiled-artifact variant cardinality. The command succeeds only when the
five outcomes remain distinct and the two controlled Releases add exactly four
variants—schema 0 and schema 1 for each Release—rather than endpoint-specific
cache entries.

`make load-server-recovery-400` and `make load-postgres-recovery-400` use the
`outage-recovery` scenario. After a successful baseline wave, the command
pauses exactly the requested local Compose service, records the timed-out
outage wave, unpauses it, and records a recovery wave with the same endpoint
identities. These targets require the separate `--allow-faults` acknowledgement
and are only for disposable Compose infrastructure.

`make load-policy-shaped-recovery-400` verifies the load-shaping policy through
400 real Sync clients. It uses the agent's default 30-second policy to spread
startup, stable post-success polling during a controlled server outage, and
first transient retry after recovery. Each wave reports its start spread and
the largest number of requests beginning in a 100 ms bucket. The scenario
requires a nontrivial spread and rejects any bucket containing one quarter or
more of the fleet; it is intentionally a 30+ second controlled test.

`make load-overload-400` recreates only the local Compose server with
`REMOTR_SYNC_MAX_CONCURRENT=1` and a one-second `Retry-After`, runs a single
concurrent wave, then restores the previous values even when the workload
fails. The report counts typed overloaded responses separately from generic
errors; a successful overload test requires at least one error and every error
to be an overload response.

Telemetry and soak modes build on this identity-provisioning boundary rather
than bypassing it.
