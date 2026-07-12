# Foundation evidence audit — 2026-07-11

This audit separates passing evidence from planned coverage in the
`establish-testing-and-performance-foundation` OpenSpec change. A checked task
has implementation and verification evidence; an unchecked task is not made
complete merely because a related test or document exists.

## Current verified foundation

The deterministic suite (`go test -mod=vendor ./...`), strict OpenSpec
validation, documentation build, Compose validation, provider fixtures, fuzz
seed discovery, benchmark collection, and the mutation pilot are implemented
in the change. The authenticated load harness has additionally produced local
Compose evidence for 400 unique mTLS endpoints covering:

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
| 5.8 provider conformance | Provider evidence is audited, but representative advertised-provider gaps remain. | Fix the gap or truthfully de-advertise it, then enable the gate only for the supported promise. |
| 7.3–7.4 fuzzing | Parsing, configuration, and current Sync properties are covered. | Add fuzz properties only when execution leases, activation ordering, secret versions, and rollback retention are implemented. |
| 8.2 and 8.7 mutation | The Mewt pilot is reproducible and evidence-only. | Add the missing critical concepts, review survivors, and approve a mutation enforcement policy. |
| 9.2, 9.3, 9.5, 9.6 benchmarks | Current composition, dependency, and payload work is benchmarked. | Implement schema/capability selection, activation, secrets, rollback, authorization, and leases; then benchmark the real Postgres paths. |
| 10.5 load | Current telemetry-heavy Sync is evidenced. | Add mixed schema/capability populations and capability-blocked delivery when the protocol and selector exist. |
| 10.8–10.10 resource/soak/profiles | Load-process and database snapshots exist. | Implement agent-cycle/rollback storage measures, growth harnesses, and performance budgets that can trigger bounded profiles. |
| 11.1 load-shaping seams | Clock and randomness are injectable for polling, retry, and overload. | Add the lease/expiry behavior before claiming seams for those absent paths. |
| 12.1–12.3 budgets | Controlled-runner workflows and evidence formats exist. | Run pinned-hardware baselines and obtain explicit OpenSpec approval for budget and mutation policy. |
| 12.4–12.7 expensive gates/history | Fuzzing, container, Vagrant, 400-reference, benchmark, and 4,000-headroom workflows are scheduled or retained. | Add medium/long soak, complete mutation policy, and retain a unified history for every required evidence family. |
| 12.9–12.10 acceptance | Operational documentation exists. | Verify from a clean checkout and obtain the required foundation and budget acceptance before unblocking the applicator umbrella. |

## Interpretation rules

- `planned`, `deferred`, and `not-applicable` are dispositions, not passing
  evidence for an advertised public behavior.
- A current-path test does not stand in for a capability, lease, secret, or
  rollback behavior that has not been implemented.
- Performance observations are not release budgets until captured on the
  controlled environment and approved through OpenSpec.
- The 4,000-endpoint workflow is headroom evidence, not a supported-fleet-size
  claim.

Use [verification foundation operations](foundation-operations.md) to run,
triage, update, or safely retire this evidence.
