# Pre-change baseline and verification map

Recorded 2026-08-07 before production edits for
`add-unchanged-sync-fast-path`.

## Baseline commands

- `env GOCACHE=/tmp/remotr-go-cache GOTMPDIR=/tmp make test`
  - Result: PASS outside the filesystem/network sandbox.
  - All 66 discovered committed fuzz seed targets passed, followed by the
    complete root-module package suite.
  - A sandbox-only attempt failed because `httptest` could not bind loopback;
    this was an execution-environment restriction, not a repository failure.
- `env GOCACHE=/tmp/remotr-go-cache GOTMPDIR=/tmp go test -mod=vendor
  ./internal/agent/sync -run '^$' -bench '^BenchmarkSyncResponseJSONAndGzip$'
  -benchmem -count=10`
  - Result: PASS.
  - Existing 10-resource JSON response: 795 payload bytes, 1,106 B/op, 2
    allocs/op, 846.9-1,544 ns/op across ten samples.
  - Existing 10-resource gzip response: 257 payload bytes, 814,736-814,739
    B/op, 21 allocs/op, 109,241-160,348 ns/op across ten samples.
  - Existing 1,000-resource JSON response: 60,195 payload bytes,
    66,263-66,376 B/op, 2 allocs/op, 37,939-43,162 ns/op across ten samples.
  - Existing 1,000-resource gzip response: 2,999 payload bytes,
    821,904-821,906 B/op, 24 allocs/op, 289,951-377,789 ns/op across ten
    samples.
- `make load-steady-400` with the disposable Compose server/Postgres and 400
  distinct mTLS identities.
  - First controlled sample: FAIL at the existing gate because warm-up p95 was
    384,464,015 ns versus the approved 350,000,000 ns maximum. The harness
    withheld its report after the gate failure. A fresh controlled repeat is
    recorded below.
  - Fresh controlled repeat: PASS. Warm-up: 400/400 successful, p50
    264,093,818 ns, p95 296,299,862 ns, 168,800 request bytes, 193,200 response
    bytes. Unchanged wave: 400/400 successful and unchanged, p50 146,928,333
    ns, p95 153,355,377 ns, 205,200 request bytes, zero response payload bytes.
  - Whole-run Postgres delta (the pre-change harness did not yet attribute
    operations per phase): 9,004 commits, 2 blocks read, 47,409 block hits,
    36,638 tuples returned, 18,753 tuples fetched, and 1,264 tuples inserted.
    Retained rows, artifact variants, temp files/bytes, rollbacks, and deadlocks
    did not grow. This non-zero aggregate is the database-operation baseline
    that OS-USF-001 and OS-PSA-004/015 must replace with phase-attributed zero
    operations on eligible hits and bounded checkpoint work.

## Verification-layer map

| Verification ID | Public seam | Required evidence layers |
| --- | --- | --- |
| OS-USF-001 | Authenticated Sync | instrumented persistence boundary, zero-operation hit test, native benchmark, authenticated load/Postgres phase attribution |
| OS-USF-002 | Authenticated Sync | bypass table at the request/response seam, deterministic injected-clock deadline cases |
| OS-USF-003 | Authenticated Sync | malformed/mismatched hash cases, canonical document unit cases, bounded hash-summary fuzz property, no-persistence assertion |
| OS-USF-004 | Agent Sync client and protected credential state | request wire assertions, lost-response retransmission, changed-document behavior, restart persistence |
| OS-USF-005 | Postgres persistence and authenticated Sync | changed-only capability/system/delivery integration cases, persistence-failure and durable-before-ack cases |
| OS-USF-006 | Git Sync/Admin/enrollment mutation APIs racing authenticated Sync | deterministic generation-barrier races, global/fleet/endpoint invalidation cases, race detector |
| OS-USF-007 | Server configuration, authenticated Sync, Postgres liveness/audit | interval boundary table, deterministic clock window/count cases, failure/restart and redaction canary cases |
| OS-USF-008 | Legacy authenticated Sync fixture | prior-schema compatibility, malformed/unauthenticated/unknown-endpoint regressions, cold-start fallback |
| OS-USF-009 | Server configuration and authenticated Sync | entry/byte/TTL/observation boundaries, deterministic eviction, soak growth, multi-process fail-closed status |
| OS-USF-010 | Authenticated Sync plus bounded diagnostics | hit/miss/invalidation/eviction/document/checkpoint/disabled metrics, latency/bytes/database operations, sensitive-cardinality canary |
| OS-PSA-004 | Authenticated 400-identity load harness | full-document priming, hash-only steady phase, request/response bytes, cache outcomes, zero database operations per eligible hit |
| OS-PSA-015 | Authenticated 400-identity load harness | checkpoint turnover phase, exact bounded coalesced Postgres operations, return to zero-operation hits |
| OS-PSA-016 | Authenticated load plus public release/policy mutation | release fan-out boundary, stale-response exclusion, hit-ratio recovery, resource/database growth |
| OS-PSA-017 | Authenticated load plus endpoint revocation API | revocation linearization boundary, rejected revoked identity, unrelated-endpoint hit behavior, growth/error evidence |
| OS-USF-011 | Server environment configuration and authenticated Sync | disabled/memory/Redis selection table, URL/prefix boundaries, secret canary, real Redis fallback and mutation-barrier outage cases |
| OS-PSA-018 | Authenticated 400-identity load through two server processes | Redis command/connection counts, process replacement, zero-Postgres shared hits, interruption/recovery, mutation fan-out, latency and bounded resource growth |

## Redis extension baseline

Recorded 2026-08-07 before Redis production edits. The repository had no Redis
client dependency, Redis service, Redis commands, shared cache connections, or
Redis load phase. Consequently the pre-change Redis command/connection count
was zero and process replacement necessarily cold-started the memory cache.
The existing in-memory budgets remain the comparison baseline; Redis adds
separate network-latency, command-count, connection-count, and key-growth
measurements rather than weakening those budgets.

The first OS-USF-011 public-seam test was run before implementation:
`env GOCACHE=/tmp/remotr-go-cache GOTMPDIR=/tmp go test -mod=vendor
./cmd/remotr-server -run '^TestFastPathBackendSelection$' -count=1`. Intended
red: build failure because `server.FastPathConfig` had no selectable backend.
