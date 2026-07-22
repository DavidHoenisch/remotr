## Context

State reports and endpoint history currently converge in `buildStateReport`: the latest persisted apply failure is attached before the latest drift report is classified. Because classification gives any attached apply failure priority, a month-old failure can override a later compliant report. Delivery state is already stored independently, but the desktop presents compliance without a first-class delivery label.

Operating-system discovery preserves `OSID=pop` but normalizes the public `Distro` field to Debian. Both local resolution and artifact target selection use `Distro`, so `targetDistros: [Debian]` unintentionally includes Pop!_OS even though exact provider qualification uses distribution and release tuples and correctly withholds support.

## Goals / Non-Goals

**Goals:**

- Make newer State-report evidence supersede older apply failures without deleting historical failure records.
- Present active-artifact compliance independently from target-release delivery readiness.
- Represent Pop!_OS as exact `PopOS` configuration/capability identity with `debian` lineage.
- Keep Ubuntu-only configuration and Ubuntu Pro inapplicable to Pop!_OS.
- Fail closed when an exact Pop!_OS provider row has not been qualified.

**Non-Goals:**

- Qualifying or advertising any Pop!_OS provider row without real environment evidence.
- Treating missing applicable requirements as optional or delivering partial desired state.
- Deleting historical failures or changing the immutable audit record.

## Decisions

### Correlate apply failure with the current report

The State-report projection will attach `last_apply_failure` only when it is not older than the latest State report and, when both release refs are present, it refers to the same release. A later report therefore becomes authoritative. The endpoint-detail API continues returning the latest historical failure independently.

Timestamp and release correlation is preferred over clearing the persistence table because it is backward compatible, preserves incident evidence, and works for existing rows without a migration. A current report's own failed `apply` item remains authoritative regardless of the historical summary.

### Add exact Pop!_OS identity

`types.PopOS` is the canonical exact product value and serializes as `PopOS` in configuration and `popos` in capability facts/target predicates. OS discovery maps `ID=pop` to `PopOS` and retains `DistroFamily=debian`. Exact `targetDistros` matching remains unchanged; authors include `PopOS` explicitly where intended.

This is preferred over mapping Pop!_OS to Ubuntu because Ubuntu Pro and other Canonical-only providers require exact Ubuntu identity. It is preferred over keeping the Debian mapping because Debian release qualification must not silently cover a derivative with a different product/version lifecycle.

### Keep capability qualification exact

Pop!_OS will advertise schema and normalized facts, but provider/resource capabilities still require exact published Pop!_OS matrix rows. This change adds no passing rows. A Pop!_OS configuration can therefore be validly targeted yet capability-blocked until evidence is added in a separate qualification change.

### Present delivery as an independent status

Desktop endpoint inventory/detail derives a bounded delivery label from existing Admin API fields: `blocked`, `unmanaged`, `offered`, `current`, or `not_reported`. Compliance remains the current State-report status and is never rewritten from delivery state.

## Risks / Trade-offs

- [Existing repositories relied on Debian implicitly selecting Pop!_OS] → Document the exact-target behavior and require an explicit `PopOS` entry; validation remains deterministic.
- [Pop!_OS becomes blocked from broadly unscoped resources] → Preserve fail-closed delivery and expose exact missing capabilities; qualify providers separately with required real-environment evidence.
- [Clock skew could misorder evidence] → Require both release correlation and report/failure timestamps where available; current report `apply` entries remain the strongest source.
- [A new desktop status adds visual density] → Use one compact Delivery column/token and retain detail for missing requirements.

## Migration Plan

1. Deploy server classification and desktop presentation; no database migration is required.
2. Deploy agents with exact Pop!_OS identity. Until then, older agents continue reporting Debian and remain compatible with the old behavior.
3. Update configuration repositories to add `PopOS` only to resources intentionally applicable to Pop!_OS.
4. Qualify exact Pop!_OS provider rows in a separate evidence-backed change before expecting delivery of those resources.

Rollback restores the previous binary behavior. Persisted endpoint history and desired-state YAML remain readable; `PopOS`-targeted configuration must be removed before rolling back to a parser that does not recognize it.

## Open Questions

None.
