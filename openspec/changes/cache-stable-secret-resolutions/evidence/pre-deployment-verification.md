# Pre-deployment verification

Recorded 2026-08-10 for `cache-stable-secret-resolutions`.

## Production baseline

The deployed `v0.6.34` server used the Redis unchanged-Sync fast path. Fly logs
showed ordinary `agent.sync` audit checkpoints about five minutes apart, proving
that unchanged Sync itself was not the 30-second database wakeup.

The same logs showed six successful `POST /v1/secrets/resolve` requests from
`stllr-herman-bot-3da224f3` every 30 to 35 seconds. A second endpoint repeated
the same request at that cadence and received `403`. Every request reopened
endpoint, artifact, authorization, registry, decrypt, and audit paths.

Neon usage remained exactly 0.5000 CUh per hour across the observed seven-hour
post-deployment window. This is the pre-change idle baseline. It does not claim
that the change in this branch has been deployed.

## Deterministic pre-deployment evidence

- `TestSyncRunStableSecretArtifactResolvesOnce` runs the composed agent, real
  Check pipeline, authenticated Sync test endpoint, and authenticated secret
  test endpoint for ten cycles. The first artifact is accepted under report
  policy and the total resolver request count remains one.
- `TestAuthorityCachingResolverMakesOneRequestForStableScope` proves the real
  Remotr HTTP provider boundary is called once and returned material cannot
  alias the retained copy.
- `TestAuthorityCachingResolverInvalidationAndFailureClasses` proves changed
  and missing tokens re-resolve, stable authorization denials remain local, and
  transient failures are not cached.
- `TestAuthorityCachingResolverRetriesConcurrentAuthorityChange` proves an
  in-flight result is cleared and retried when authority changes before the
  resolver returns.
- `TestAuthorityCachingResolverBoundsAndClearsMaterial` proves deterministic
  entry and byte bounds plus controlled plaintext clearing on invalidation.
- `TestSecretAuthorityTokenTracksStableMutationBoundary` proves stable,
  unstable, and changed in-memory authority behavior.
- `TestRedisFastPathSurvivesProcessReplacementAndCoordinatesMutation` extends
  the existing real-Redis integration seam to prove shared tokens, mutation
  invalidation, and epoch-reset rejection. It runs when
  `REMOTR_TEST_REDIS_URL` is provided.
- `BenchmarkAuthorityCachingResolverHit`, collected five times on an AMD Ryzen
  AI 9 HX 370, measured 94.83 to 104.9 ns/op, 16 B/op, and one allocation per
  stable cache hit.

The required `make test` run passed all 67 discovered committed fuzz seed
targets and the complete root-module package suite. Race-detector runs passed
for `internal/secrets`, `internal/server`, and `cmd/remotr-agent`. OpenSpec
validation passed. The real-Redis case was compiled but skipped because this
worktree did not receive `REMOTR_TEST_REDIS_URL`; it remains an explicit canary
gate before deployment.

## Canary and production budgets

Deploy the server before or with the new agent. An older server omits the token,
which intentionally leaves caching disabled.

For one report-mode canary with a server-managed secret:

1. Record resolver request and `agent.secret.resolve` audit counts.
2. Prime the agent once. Ten stable cycles must add zero resolver requests and
   zero resolver audit writes.
3. Activate a test secret version. The next Sync must change the token and the
   next secret use must add exactly one resolver request and one ordinary audit
   event.
4. Leave the workload stable for at least seven minutes. With Neon's five-minute
   suspension setting, Remotr traffic must not prevent the compute from idling.
5. Re-audit hourly compute with the existing authenticated `neonctl` session
   outside the sandbox. A full hour containing only the stable canary workload
   must be below the former 0.5000 CUh continuously-active baseline.

The opt-in performance diagnostics expose only aggregate
`secretAuthorityCache` counters: primes, hits, denial hits, evictions,
invalidations, fail-closed uses, and declined oversized entries. They contain
no references, endpoint identifiers, tokens, fingerprints, or secret material.
