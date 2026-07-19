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

## Acceptance status

| Open task group | Current status | What must happen before completion |
| --- | --- | --- |
| 8.7 mutation enforcement | Mewt 3.0.1 is approved only as an isolated verified test process. Pull requests mutate every changed registered critical target, weekly CI runs the comprehensive campaign, and the current reviewed survivor baseline is zero. | Complete; any unexplained high or otherwise security-relevant survivor is blocking, while mutation score cannot replace functional evidence. |
| 9.6 Postgres benchmarks | Ten-sample controlled collections cover Change-control, compiled-artifact lookup, endpoint check-in, telemetry insert/upsert, and 400-endpoint Fleet reporting through rollback-only real Postgres transactions. | Complete; ranges and environment are recorded in `foundation-performance-baseline-2026-07-18.md`. |
| 10.5 load | Telemetry-heavy Sync and authenticated mixed-capability Sync are implemented and evidenced. A 400-endpoint run covered five equal populations, completed with zero errors, reported 240 capability-blocked and 80 unmanaged outcomes, retained active telemetry attribution, and added exactly four bounded artifact variants. | Complete for task 10.5; retain `capability-delivery-load-evidence-2026-07-18.md` as the controlled baseline when the protocol or harness changes. |
| 10.8–10.10 resource/soak/profiles | Full agent cycles, repeated authenticated resource-growth samples, and bounded redacted failure capture are implemented. | Complete after the profile smoke proof recorded in the foundation change. |
| 12.1–12.3 budgets | Controlled ten-sample native, 400-endpoint, and soak-tracer evidence produced the versioned absolute and relative policy. | Complete; changes require OpenSpec review rather than result replacement. |
| 12.4–12.7 expensive gates/history | PR, nightly, weekly, and release workflows retain machine-readable coverage, mutation, benchmark, load, soak, fuzz, and test/flaky status for 90 days or longer. | Complete. |
| 12.9–12.10 acceptance | The complete candidate passed from a clean offline checkout, remained clean after verification, and uses repository-pinned mutation tooling plus disposable local performance credentials. The applicator umbrella foundation prerequisite is checked without changing provider support claims. | Complete; retain `foundation-clean-checkout-proof-2026-07-19.md` with the accepted baseline. |

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
