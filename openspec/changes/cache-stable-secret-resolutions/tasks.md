## 1. Authority Protocol

- [x] 1.1 Add failing protocol tests for stable, changed, missing, and
  Redis-coordinated secret-authority tokens (OS-LSM-033).
- [x] 1.2 Add backend epochs and derive the opaque token from stable authority
  snapshots without exposing endpoint or secret metadata (OS-LSM-033).
- [x] 1.3 Add the optional token to full and cached Sync responses and verify
  backward-compatible JSON behavior (OS-LSM-033).

## 2. Agent Resolution Cache

- [x] 2.1 Add a failing composed-agent test proving one initial remote
  resolution and zero requests on an unchanged second Check (OS-LSM-032,
  OS-PSA-019).
- [x] 2.2 Implement the bounded authority-aware resolver with copy isolation,
  LRU eviction, and byte clearing (OS-LSM-032, OS-LSM-034).
- [x] 2.3 Wire Sync token observation before Apply and prove a changed or
  missing token forces a new resolution (OS-LSM-033).
- [x] 2.4 Add failing and passing tests for cached authorization denials and
  uncached transient failures (OS-LSM-035).

## 3. Performance and Operations

- [x] 3.1 Add native benchmarks and a churn test proving entry and byte bounds
  during unique-scope and invalidation workloads (OS-PSA-020).
- [x] 3.2 Add resolver/cache counters that reveal prime, hit, denial-hit,
  eviction, invalidation, and fail-closed behavior without secret material.
- [x] 3.3 Run focused tests, the full Go test suite, race-sensitive package
  tests, and OpenSpec validation.
- [x] 3.4 Document canary deployment and Neon-idle verification evidence,
  including resolver request and audit-write budgets (OS-PSA-019).
