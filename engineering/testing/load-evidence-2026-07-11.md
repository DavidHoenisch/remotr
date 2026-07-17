# Fleet-load evidence — 2026-07-11

This is a controlled local Compose observation, not a production capacity
claim or an advertised support limit. It uses the real HTTPS server and local
Postgres registry with 400 newly enrolled mTLS endpoint identities.

## Reference steady-unchanged workload

The workload was run from the repository root with explicit disposable
`REMOTR_LOAD_SERVER_URL`, `REMOTR_LOAD_CA`, `REMOTR_LOAD_DATABASE_URL`, and
`REMOTR_LOAD_FLEET` settings:

```sh
REMOTR_LOAD_RUN_ID=steady-400-evidence-20260711 make load-steady-400
```

The target provisions 400 unique endpoints, sends a full artifact warm-up
wave, waits the default 30-second polling interval, then sends an unchanged
wave at concurrency 400. It exited successfully.

| Measurement | Warm-up artifact wave | Unchanged wave |
| --- | ---: | ---: |
| Requests / successes / errors | 400 / 400 / 0 | 400 / 400 / 0 |
| Unchanged responses | 0 | 400 |
| Request bytes | 6,800 | 43,200 |
| Artifact response bytes | 192,800 | 0 |
| p50 / p95 / max latency | 232.6 / 247.9 / 250.4 ms | 91.0 / 100.3 / 101.8 ms |

The load-generator process used 1.224 seconds of CPU across the measured
window. Its RSS changed from 35.1 MB to 81.5 MB, heap allocation from 7.3 MB
to 24.5 MB, and goroutines from 6 to 806 while the 400 authenticated clients
and their idle keep-alive connections remained live. The command exits after
the report, so that end-of-window goroutine value is an in-run observation,
not a retained process leak claim.

The harness's Postgres pool ended with one idle connection and recorded one
additional acquisition during the workload snapshot. `pg_stat_database`
increased by 7,442 committed transactions, 29,361 cache-block hits, 11,321
tuples returned, 10,050 tuples fetched, and 488 tuples inserted; it observed
no new temp files or deadlocks. Those database counters cover all activity in
the disposable database, so the JSON report preserves both raw snapshots and
their deltas rather than attributing every counter solely to Sync.

Subsequent results must be compared on a controlled runner before becoming a
budget or gate. The 4,000-endpoint comparison and fault/recovery scenarios are
separate workload tasks.

## Simultaneous startup, reconnect, and recovery

The same local Compose environment also ran:

```sh
REMOTR_LOAD_RUN_ID=startup-reconnect-400-20260711 \
  make load-startup-reconnect-400
```

This creates 400 identities, sends an initial concurrent Sync wave, closes
every client transport, sends a concurrent fresh-TLS reconnect wave, closes
the transports again, and sends the post-reconnect recovery wave. It exited
successfully.

| Wave | Requests / successes / errors | Unchanged | Artifact bytes | p50 / p95 / max latency |
| --- | ---: | ---: | ---: | ---: |
| Simultaneous startup | 400 / 400 / 0 | 0 | 192,800 | 201.4 / 214.5 / 216.9 ms |
| Simultaneous reconnect | 400 / 400 / 0 | 400 | 0 | 190.2 / 205.1 / 207.7 ms |
| Post-reconnect recovery | 400 / 400 / 0 | 400 | 0 | 198.1 / 210.2 / 211.3 ms |

The explicitly closed client transports ensure the second and third waves are
fresh TLS connection attempts while the retained digest/release values exercise
the unchanged Sync protocol. This demonstrates connection-level recovery, not
server or database fault recovery; the latter requires controlled degradation.

## Release fan-out and endpoint override

The real compiled-artifact selection path was exercised with:

```sh
REMOTR_LOAD_RUN_ID=release-fanout-400-20260711 make load-release-fanout-400
```

The harness captured the current fleet artifact, created a distinct artifact
under a temporary release ref, advanced that ref, and finally stored a distinct
override for one load endpoint at that same ref. It restored the global release
ref to `e2e-dev` before returning. The run exited successfully.

| Wave | Requests / successes / errors | Unchanged | Artifact bytes | p50 / p95 / max latency |
| --- | ---: | ---: | ---: | ---: |
| Baseline artifact | 400 / 400 / 0 | 0 | 192,800 | 232.7 / 248.0 / 250.5 ms |
| Release fan-out | 400 / 400 / 0 | 0 | 216,800 | 134.9 / 144.9 / 146.5 ms |
| One endpoint override | 400 / 400 / 0 | 399 | 614 | 131.7 / 142.9 / 144.2 ms |

The second wave proves that the new release ref caused all 400 clients to
receive full artifacts. In the third wave, the one stored endpoint override
was the only non-unchanged response, demonstrating endpoint-over-fleet
selection under concurrent load. The release ref was queried after completion
and was `e2e-dev`, confirming the disposable stack was restored.

## Controlled degradation, timeout, and recovery

The server and Postgres fault targets each pause one disposable Compose service
after baseline Sync, use a two-second per-request timeout for the outage wave,
unpause the service, and run the recovery wave with the same 400 identities.
Both commands exited successfully, and the paused services were subsequently
healthy.

| Fault target | Baseline | Controlled outage | Recovery |
| --- | --- | --- | --- |
| `remotr-server` | 400 / 400 successful; p95 263.0 ms | 400 / 400 timed out; p95 2.002 s | 400 / 400 unchanged; p95 490.5 ms |
| `postgres` | 400 / 400 successful; p95 234.9 ms | 400 / 400 timed out; p95 2.002 s | 400 / 400 unchanged; p95 238.2 ms |

Neither outage wave transferred artifact bytes or reported an overload. The
server recovery wave was slower immediately after unpause, which is recorded
as an observation rather than a gate; the Postgres recovery p95 returned near
its pre-fault result.

## Controlled overload response

`make load-overload-400` temporarily recreated only the disposable server with
`REMOTR_SYNC_MAX_CONCURRENT=1` and `REMOTR_SYNC_RETRY_AFTER=1s`. Its 400-way
concurrent Sync wave had one successful full-artifact response and 399 errors;
all 399 errors were typed overload responses. The report's p50/p95/max request
latencies were 212.3 / 223.1 / 227.1 ms. After the target completed, the
server was recreated with `REMOTR_SYNC_MAX_CONCURRENT=0` and
`REMOTR_SYNC_RETRY_AFTER=5s`, which was verified inside the healthy container.

This validates the overload classification and restoration mechanics. It does
not by itself prove an agent retry schedule; that requires the separate
load-shaping evidence.

## Policy-shaped startup and outage recovery

The agent's real `polling.Policy` delays were applied to 400 authenticated
clients with the default 30-second interval:

```sh
REMOTR_LOAD_RUN_ID=policy-shaped-400-20260711 \
  make load-policy-shaped-recovery-400
```

After a policy-shaped successful startup, the disposable server was paused for
the stable post-success poll, then unpaused for first transient-backoff
recovery. The command exited successfully and the server was healthy after the
run.

| Phase | Result | Start spread | Largest 100 ms start bucket |
| --- | --- | ---: | ---: |
| Startup delay | 400 / 400 successes | 2.984 s | 23 |
| Stable poll during outage | 400 / 400 bounded timeouts | 2.950 s | 42 |
| First jittered retry after recovery | 400 / 400 unchanged successes | 996 ms | 53 |

The scenario rejects any phase with less than 250 ms of start spread or with a
100 ms bucket containing one quarter of the fleet (100 requests). This run
remained below both limits in every phase. It is protocol-level load-harness
evidence using the same policy delays as the agent loop; it does not replace
the deterministic unit/property tests for exact policy bounds.

## Current telemetry-heavy Sync

The implemented Sync telemetry fields were exercised with:

```sh
REMOTR_LOAD_RUN_ID=telemetry-heavy-400-20260711 \
  make load-telemetry-heavy-400
```

After a 400-endpoint artifact baseline, every endpoint submitted bounded
labels, usernames, system inventory, drift, and firewall-audit reports during
an unchanged Sync. The telemetry wave completed 400 / 400 successfully with
15,110,800 request bytes, zero artifact bytes, and p50/p95/max latency of
291.9 / 310.7 / 331.3 ms. Database checks after the run found 400 matching
system-info rows, 400 drift rows, and 400 firewall-audit rows.

This is evidence for the currently implemented telemetry persistence path only.
Mixed schema and capability populations, including capability-blocked
endpoints, remain deliberately out of scope until those public protocol and
selection behaviors exist.
