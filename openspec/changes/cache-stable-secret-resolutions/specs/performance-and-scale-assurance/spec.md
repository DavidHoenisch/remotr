## ADDED Requirements

### Requirement: Stable secret-bearing cycles are database quiet

After one successful prime, an unchanged agent cycle that references
server-managed secrets SHALL perform zero `/v1/secrets/resolve` requests, zero
secret registry reads or decryptions, and zero secret-resolution audit writes
while the same authority token remains current.

#### Scenario: Thirty-second stable pull workload
<!-- verification-id: OS-PSA-019 -->

- **WHEN** a composed agent runs at least ten unchanged cycles with one or more
  server-managed secret references and a stable authority token
- **THEN** resolver, registry, decrypt, and audit counters SHALL remain
  unchanged after the priming cycle

#### Scenario: Neon suspension window
<!-- verification-id: OS-PSA-020 -->

- **WHEN** stable secret-bearing agents are the only application workload for
  longer than the configured Neon suspension timeout
- **THEN** Remotr application traffic SHALL NOT prevent the Neon compute from
  entering idle state

### Requirement: Secret cache resources remain bounded

The endpoint cache SHALL expose native benchmark and load evidence showing that
entry count and retained material bytes remain within configured bounds under
unique-scope churn and authority invalidation.

#### Scenario: Unique-scope churn
<!-- verification-id: OS-PSA-021 -->

- **WHEN** the load harness resolves more unique scopes than the configured
  entry limit
- **THEN** retained entry count and material bytes SHALL remain within bounds
  and later misses SHALL continue to resolve correctly

#### Scenario: Repeated authority invalidation
<!-- verification-id: OS-PSA-022 -->

- **WHEN** the authority token changes repeatedly under load
- **THEN** retained entries SHALL return to zero after each invalidation without
  unbounded latency or memory growth
