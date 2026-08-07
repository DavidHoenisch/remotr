## MODIFIED Requirements

### Requirement: Fleet load uses distinct authenticated endpoint identities
The load harness SHALL exercise the real server and Postgres protocol with unique authenticated endpoint identities, realistic fleet membership, full capability and system-information documents during priming, acknowledged document hashes during steady state, artifact digests, and event telemetry. It SHALL NOT approximate endpoint load with unauthenticated health requests. The harness SHALL attribute application database operations, cache outcomes, payload bytes, and checkpoint writes to workload phases without exposing endpoint identifiers as metric labels.

#### Scenario: Four hundred endpoints perform unchanged Sync
<!-- verification-id: OS-PSA-004 -->
- **WHEN** the reference workload primes 400 endpoints and then runs eligible unchanged polling at the default interval between durability checkpoints
- **THEN** the harness records successful request rate and latency plus server, database, cache, network, allocation, payload, and error metrics, and observes zero application database operations for each eligible fast-path request

#### Scenario: Checkpoint becomes due under steady load
<!-- verification-id: OS-PSA-015 -->
- **WHEN** the same workload crosses a configured liveness and audit checkpoint deadline
- **THEN** database operations are bounded to the expected coalesced checkpoint work and subsequent eligible requests return to zero-operation cache hits

## ADDED Requirements

### Requirement: Fast-path load proves invalidation and cold-path recovery
The authenticated load suite SHALL separately exercise cold priming, steady cache hits, release fan-out, policy mutation, endpoint revocation, checkpoint turnover, eviction, and server restart. It SHALL verify response correctness and database-operation counts for each phase and SHALL fail if hit ratio or resource growth hides stale delivery or authorization.

#### Scenario: Release changes during cached fleet polling
<!-- verification-id: OS-PSA-016 -->
- **WHEN** 400 cached endpoints poll while the release reference advances
- **THEN** no post-advance response serves the prior cached decision, affected endpoints reenter full selection, and the workload returns to unchanged fast-path operation after acknowledgement

#### Scenario: Endpoint is revoked during cached polling
<!-- verification-id: OS-PSA-017 -->
- **WHEN** a cached endpoint credential is revoked while the unchanged workload continues
- **THEN** that endpoint stops receiving successful cached responses at the revocation boundary while unrelated endpoints retain their eligible cache behavior

