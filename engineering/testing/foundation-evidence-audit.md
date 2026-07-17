# Foundation evidence audit — 2026-07-15

This audit separates passing evidence from planned coverage in the
`establish-testing-and-performance-foundation` OpenSpec change. A checked task
has implementation and verification evidence; an unchecked task is not made
complete merely because a related test or document exists.

## Current verified foundation

The deterministic suite (`go test -mod=vendor ./...`), strict OpenSpec
validation, documentation build, Compose validation, provider fixtures, fuzz
seed discovery, benchmark collection, and the mutation pilot are implemented
in the change. This closeout pass additionally completed:

- a truthful representative-provider migration decision that leaves
  unit-only matrix rows unadvertised;
- durable fuzz properties for authorization grouping, execution leases,
  activation ordering, rollback metadata, and bounded compliance reports;
- current high-severity mutation campaigns for capability selection,
  authorization grouping, leases, activation, rollback, and versioned secret
  behavior, with no uncaught mutant in that expanded scope;
- allocation-reporting benchmarks for capability selection, activation
  coalescing, envelope encryption/rewrap, and rollback
  reservation/encryption/pruning/cleanup; and
- deterministic clock/randomness boundaries covering polling, backoff,
  overload, authorization windows, leases, and expiry.

The rollback cleanup benchmark exposed and now retains a public-seam
regression for pruning a stale record without aborting the current armed save.

The authenticated load harness has produced local Compose evidence for 400
unique mTLS endpoints covering:

- default-interval unchanged Sync;
- fresh-TLS startup/reconnect recovery;
- release fan-out and one-endpoint override selection;
- server and Postgres timeout/recovery plus typed overload;
- policy-shaped startup, stable outage polling, and transient recovery;
- current persisted telemetry fields.

The detailed commands, measurements, safety boundaries, and the 4,000-endpoint
headroom distinction are in [load evidence](load-evidence-2026-07-11.md) and
the [load harness reference](load-harness.md).

## Deliberately incomplete coverage

| Open task group | Current status | What must happen before completion |
| --- | --- | --- |
| 8.7 mutation enforcement | All 296 original high-severity mutants in the expanded targets were caught; the regenerated rollback target catches all 59 current high-severity mutants. The Mewt pilot decision still rejects adoption as a gate. | Resolve the AGPL CI policy decision, triage the 127 historical survivors plus the expanded medium/low scope, and approve a mutation acceptance policy. |
| 9.6 Postgres benchmarks | The versioned Change-control snapshot now covers every mutation. Its real 400-endpoint JSONB compare-and-swap round trip was collected locally at 4.85–7.68 ms/op across four 10-iteration collections for a 180,756-byte input and 9,399-byte JSONB value; the other query families still lack equivalent controlled collections. | Benchmark compiled-artifact, endpoint check-in, telemetry, and Fleet reporting on the controlled database, then repeat the Change-control series on the pinned runner before approving budgets. |
| 10.5 load | Telemetry-heavy Sync is evidenced. The Sync protocol still has no capability document, mixed-schema selection, or capability-blocked delivery outcome. | Implement those public protocol behaviors in the governing product change, then add the mixed-population workload. |
| 10.8–10.10 resource/soak/profiles | Load-process and database snapshots exist. | Implement agent-cycle/rollback storage measures, growth harnesses, and performance budgets that can trigger bounded profiles. |
| 12.1–12.3 budgets | Controlled-runner workflows and evidence formats exist. | Run pinned-hardware baselines and obtain explicit OpenSpec approval for budget and mutation policy. |
| 12.4–12.7 expensive gates/history | Fuzzing, container, Vagrant, 400-reference, benchmark, and 4,000-headroom workflows are scheduled or retained. | Add medium/long soak, complete mutation policy, and retain a unified history for every required evidence family. |
| 12.9–12.10 acceptance | Operational documentation exists. | Verify from a clean checkout and obtain the required foundation and budget acceptance before unblocking the applicator umbrella. |

## Interpretation rules

- `planned`, `deferred`, and `not-applicable` are dispositions, not passing
  evidence for an advertised public behavior.
- A current-path test does not stand in for capability-document delivery or
  durable Postgres authorization behavior that has not been implemented.
- Performance observations are not release budgets until captured on the
  controlled environment and approved through OpenSpec.
- The 4,000-endpoint workflow is headroom evidence, not a supported-fleet-size
  claim.

Use [verification foundation operations](foundation-operations.md) to run,
triage, update, or safely retire this evidence.
