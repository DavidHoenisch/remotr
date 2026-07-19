# Foundation performance baseline — 2026-07-18

## Decision

This collection establishes the initial controlled-runner budgets in
`test/performance/budgets.json`. The budgets are release-blocking decisions,
not automatically regenerated expectations. A change requires review of new
paired evidence and an OpenSpec update.

The approved policy is:

- controlled latency comparisons: at most 20% regression;
- shared-runner deterministic allocation and byte comparisons: at most 10%;
- absolute native, 400-endpoint, database, and soak limits: the versioned
  values in `test/performance/budgets.json`;
- a faster metric cannot offset a violation in another metric.

## Reproducibility metadata

| Item | Value |
| --- | --- |
| Source revision | `9f209b5b92ebfb9619c4cad112f457245a9d696d` |
| Clean-checkout verification tree | `eab3265f9fdcbebc3ba34b55a15557f6210c7397` |
| Go | `go1.26.5 linux/amd64`, vendored modules |
| Kernel | Ubuntu `7.0.0-27-generic`, x86-64 |
| CPU | AMD Ryzen AI 9 HX 370, 12 cores / 24 logical CPUs, one NUMA node |
| Memory | 32,171,106,304 bytes RAM; 8,589,930,496 bytes swap, unused |
| Docker | 29.6.1, `overlayfs` |
| PostgreSQL | `postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777` |
| Fleet environment | Disposable Compose stack, 400 unique enrolled endpoint identities, authenticated mTLS Sync, local Postgres |
| Time zone | America/Los_Angeles |

The source revision identifies the repository base. The verification tree is
the synthetic clean-checkout snapshot used for the final offline suite before
the evidence-only acceptance notes were appended. Task 12.9 proves that the
accepted gate does not depend on the local Mewt database, Compose runtime, or
other uncommitted developer state.

## Commands

All Go commands used `-mod=vendor`. Native benchmark series used ten repeated
samples. The Postgres suite set `REMOTR_BENCH_DATABASE_URL` to the disposable
Compose database and used rollback-only transactions with temporary tables.

```sh
make compose-up
REMOTR_BENCH_DATABASE_URL='postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable' \
  make benchmark-foundation-controlled

REMOTR_LOAD_SERVER_URL=https://localhost:8443 \
REMOTR_LOAD_CA="$PWD/compose/runtime/certs/ca.crt" \
REMOTR_LOAD_DATABASE_URL='postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable' \
REMOTR_LOAD_FLEET=test-fleet \
REMOTR_LOAD_RUN_ID=foundation-baseline-20260718-r3 \
  make load-steady-400
```

The soak tracer used the same environment and endpoint population with
`--scenario soak --steady-cycles 2 --poll-interval 0`. Scheduled medium and
long evidence use the normal 30-second polling interval.

## Native results

The ranges below are minima and maxima across ten samples, rounded only for
readability. The machine gate parses the unrounded Go benchmark output.

| Path | Observed range |
| --- | --- |
| Change-control state round trip, 400 endpoints | 9.70–11.32 ms/op; 180,756 input bytes; 9,399 JSONB storage bytes |
| Compiled-artifact lookup, 1,000-resource artifact | 1.128–1.402 ms/op; 4,442 stored bytes; 50–116 KiB/op |
| Endpoint check-in, 400-endpoint population | 0.693–1.167 ms/op |
| State-report telemetry insert | 0.548–0.988 ms/op |
| System-information inventory upsert | 0.437–0.806 ms/op |
| Fleet reporting, 400 endpoints | 34.77–40.32 ms/op; about 1.90 MiB/op and 18,885 allocs/op |
| Agent compliant full cycle, 100 resources | 17.0–23.9 ms/op; 76,448 report bytes |
| Agent compliant full cycle, 1,000 resources | 4.046–4.341 s/op; about 46.7 MB/op; 763,148 report bytes |
| Agent drifted Check + Apply, 100 resources | 10.9–21.7 ms/op; 170,841 report bytes |
| Agent drifted Check + Apply, 1,000 resources | 4.009–4.534 s/op; about 52.9 MB/op; 1,707,141 report bytes |

The full-cycle agent measurements also emitted CPU time, peak RSS, goroutine
count, artifact and report bytes, process disk blocks, and rollback storage.
Peak RSS was 43,163,648 bytes in this process-level collection; rollback
storage growth was zero.

## Final controlled gate replay — 2026-07-19

After the implementation and mutation closeout, the complete controlled
benchmark target was replayed against the disposable Postgres 16 stack and
evaluated by `performance-benchmark-gate.go`. The gate passed with no
violation. Its conservative observed maxima included 4,271,709,664 ns/op and
46,685,488 B/op for the compliant 1,000-resource cycle; 4,238,539,881 ns/op
and 52,882,144 B/op for the drifted Apply cycle; 47,153,152 bytes peak RSS;
23,442,438 ns/op for 400-endpoint Fleet reporting; and 3,326,807 ns/op for the
Change-control snapshot. The complete unrounded benchmark stream was used by
the gate; these summary values are not regenerated expectations.

## Authenticated 400-endpoint result

The valid `foundation-baseline-20260718-r3` run enrolled 400 unique endpoint
identities and used the current package-provider capability document.

| Wave | Success | Errors / blocked / unmanaged | p50 | p95 | max |
| --- | ---: | ---: | ---: | ---: | ---: |
| Warm artifact delivery | 400/400 | 0 / 0 / 0 | 281.1 ms | 303.1 ms | 304.9 ms |
| Unchanged Sync | 400/400, all unchanged | 0 / 0 / 0 | 185.5 ms | 192.4 ms | 194.9 ms |

The load process consumed 1.538 CPU seconds. RSS grew from 34.6 MB to 73.4 MB
and heap allocation from 8.85 MB to 37.36 MB while holding 400 authenticated
clients. PostgreSQL recorded 9,175 commits, zero rollbacks, zero block reads,
zero temporary files/bytes, zero deadlocks, 25 backends, zero variant growth,
and zero retained telemetry rows for this workload.

## Soak growth tracer

One warm wave and two unchanged observations all completed 400/400 with no
error or capability block. Server heap observations were 18,126,808,
19,152,600, and 44,863,216 bytes, a retained increase of 26,736,408 bytes
within the approved 33,554,432-byte bound. Server goroutines held at 407;
database backends held at 25; retained rows and temporary bytes held at zero.
The two agent containers exposed their own loopback runtime metrics: aggregate
goroutines held at 14, RSS rose only from 35,508,224 to 35,532,800 bytes, and
rollback state held at 12,734 bytes. Server CPU observations were 483, 602,
and 712 jiffies, for per-wave increases of 119 and 110 against the approved
2,000-jiffy limit. The monotonic-growth analysis passed every versioned limit.

## Failure-profile smoke

The failure-capture path was exercised against the same loopback-only server
diagnostics listener with one-second CPU and trace windows. It retained only
sanitized text or aggregate JSON: CPU top (247 bytes), heap top (3,399 bytes),
goroutines (3,735 bytes), trace scheduler top (782 bytes), database summary
(174 bytes), container summary (1,299 bytes), and a 166-byte manifest. No raw
profile, trace, credential, authorization header, private-key marker, or
secret canary was retained, and every file was below the approved 1 MiB cap.

## Interpretation

The initial limits intentionally leave bounded runner variance above the ten
sample maxima while remaining close enough to catch material regressions. The
4,000-endpoint job remains comparison/headroom evidence and does not change
the supported 400-endpoint reference promise.
