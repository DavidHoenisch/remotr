# Capability-compatible delivery load evidence — 2026-07-18

The capability-mixed reference workload ran against the disposable local
Compose server and Postgres registry with 400 newly enrolled, unique mTLS
endpoint identities:

```sh
REMOTR_LOAD_RUN_ID=capability-mixed-400-20260718-r2 \
  make load-capability-mixed-400
```

The workload distributed 80 endpoints into each of five populations:
compatible, blocked with an active artifact, unmanaged new, blocked with
bounded telemetry, and compatible reconnecting. It exited successfully.

| Wave | Outcome | Request / artifact bytes | p50 / p95 / max | Start spread / largest 100 ms bucket |
| --- | --- | ---: | ---: | ---: |
| Baseline offer | 320 / 320 successful | 106,880 / 27,200 | 684.4 / 709.8 / 714.0 ms | 78.7 ms / 320 |
| Baseline activation | 320 / 320 successful and unchanged | 154,240 / 0 | 531.9 / 548.2 / 549.4 ms | 1.64 ms / 320 |
| Mixed capability target | 400 / 400 successful; 240 blocked; 80 unmanaged | 3,209,520 / 13,600 | 309.1 / 334.2 / 341.2 ms | 24.0 ms / 400 |
| Stable-jitter reconnect | 80 / 80 successful and unchanged | 41,600 / 0 | 19.6 / 26.3 / 34.8 ms | 2.981 s / 8 |

The load-generator used 1.942 seconds of CPU. RSS moved from 34.2 MB to
63.1 MB, heap allocation from 10.8 MB to 18.2 MB, and goroutines from 6 to
166. The reconnect population used the ordinary 30-second successful polling
cadence and spread its requests across 2.981 seconds; no retry or overload
responses occurred.

Postgres reported 24,348 committed transactions, 15 blocks read, 134,614 cache
hits, 41,619 tuples returned, 39,991 tuples fetched, 2,603 tuples inserted,
zero temporary files, and zero deadlocks during the measured workload. The
ending backend count was 25. Compiled-artifact variant cardinality increased by
exactly four: schema 0 and schema 1 for each of the two controlled Releases.

Post-run readback found 400 delivery-state rows, 240 blocked rows, 80 unmanaged
rows, and 320 endpoints retaining an active artifact. The telemetry population
produced exactly 80 drift reports, 80 system-information rows, and 80 firewall
audit reports, all without advancing the blocked target to active state.

The first collection attempt exposed a real persistence-boundary defect before
producing acceptable evidence: Postgres JSONB normalized requirement-set key
ordering while the reader required byte-exact canonical JSON. The corrected
reader still rejects unknown fields, trailing data, invalid bounds, and digest
mismatches, but verifies JSONB semantic content through the separately indexed
canonical digest. The successful collection above was taken only after that
regression was fixed and the server image rebuilt.
