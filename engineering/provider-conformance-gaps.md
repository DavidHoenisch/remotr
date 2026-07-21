# Provider Conformance Gap Report

This report records the initial representative migration for the testing and
performance foundation. Passing unit-contract evidence does not advertise a
provider or replace the container/VM matrix required by later foundation
tasks.

## Passing representative slices

| Provider | Constructor and controlled boundary | Shared cases passing |
| --- | --- | --- |
| APT package | `apt.New` with stateful `executil.Runner` | present/absent Check, Apply, second Check, idempotent Apply |
| File content | `files.New` with a temporary filesystem | content Check, Apply, second Check, idempotent Apply |
| systemd unit | `systemd.New` with stateful `executil.Runner` | active-state Check, Apply, second Check, idempotent Apply |
| Firewall audit rule | `firewall.New` with a temporary audit log | audit-rule Check, Apply, second Check, idempotent Apply |

## Open gaps

| Provider or seam | Missing/failed contract behavior | Truthful status and follow-up |
| --- | --- | --- |
| Legacy `executor.Handler` | `State(context) (any, bool)` cannot distinguish drift from unsupported capability or probe failure. | The adapter only maps legacy handlers to compliant/drifted. Add typed provider checks before claiming `OS-LPC-002` for a real provider. |
| APT package | No real Debian/Ubuntu package-manager execution or typed `dpkg` probe failure result. | Keep unit evidence only; add pinned container rows in tasks 6.1–6.2 before advertising real-environment support. |
| File content | The current file model manages content but has no desired-absent/removal semantic; an empty desired content accepts both an absent and an existing file. | Do not claim the shared absence/removal case for files. Introduce an explicit absence model before migration. |
| systemd unit | `Apply` ignores its context and `State` converts `systemctl` probe errors to ordinary drift. | Do not claim cancellation/timeout or typed-probe conformance. Refactor the public provider boundary before enabling those cases. |
| Firewall enforcement | The migrated slice is audit mode only. firewalld/nftables enforcement requires a real backend and safety proof; current state failures collapse to drift. | Keep enforcement unadvertised by this harness. Add container proof only where namespaces are sufficient, then VM safety proof for connectivity-risk changes. |
| Firewall rollback | Audit rollback is explicitly `no-rollback`; nftables revert currently suppresses delete errors. | Do not claim rollback completion/failure evidence for enforcement until the rollback behavior is made observable and tested. |
| Activation and redaction | The shared fixtures prove the contract seam, but no representative provider exposes a common activation or diagnostic-redaction boundary yet. | Keep those cases as harness-only evidence; migrate them only when an implemented provider has the public behavior. |
| Locking and concurrency | No representative provider exposes a documented operation lock or concurrent Apply contract. | Retain the generic harness coverage; add a public lock boundary before migrating a provider-specific case. |

## Gate decision

New or changed providers must enter through `providercontract.RunConformance`,
which executes convergence, second-check idempotence, absence, unsupported,
probe/check failure, validation failure, lock contention, cancellation,
activation, redaction, and rollback case families. Existing representative
providers remain legacy migrations with the gaps above; none gains an expanded
advertisement merely because the aggregate harness exists.

## Foundation closeout decision — 2026-07-15

The representative migration is accepted with truthful de-advertisement for
every unresolved legacy gap. The shared contract, representative APT, file,
systemd, and firewall suites, and provider-matrix validation pass. Matrix rows
without real-environment convergence evidence remain `untested`, so they cannot
be used as support claims. Newly advertised provider behavior is gated through
the shared conformance harness and a passing matrix row; this closes foundation
task 5.8 without converting any legacy unit-only result into advertised
distribution support.

## Provider matrix advertisement gate

At the foundation's initial migration, `test/provider-matrix.yaml` deliberately
started empty: the shared unit harness was not evidence for an advertised
provider environment. The matrix is now populated only with rows whose exact
selectors passed. `untested` and `failing` rows retain planned evidence without
becoming support claims. Current claims are summarized in
[Ubuntu 24.04 applicator support](../docs/reference/ubuntu-2404-applicator-support.md)
and [Package provider qualification](testing/package-provider-qualification.md).
Validate a proposed claim with:

```bash
go run -mod=vendor ./scripts/provider-matrix-advertisement-gate.go \
  -capability-id <exact-id> -provider <name> \
  -distribution <id> -release <version> \
  -architecture <arch> -backend <backend> -contract-revision <revision> \
  -environment <container-or-vm>
```
