## Context

`Engine.runPreflight` (`internal/agent/engine/engine.go`) requires an
`executor.Preflighter` for every non-normal-risk resource. Only ten provider
packages implement one, so resources of other kinds left at their default risk
can never apply. Separately, `rollbackstore.Handle` arms a durable record before
mutation and only the Ubuntu Pro provider ever calls `Acknowledge`; armed
records then block `Reserve` for that address through
`ensureMutationAllowedLocked`.

## Goals / Non-Goals

**Goals:** tell configuration authors about unsatisfiable preflight
requirements before deployment; stop converged resources from staying blocked;
clear today's stuck records without manual endpoint work.

**Non-Goals:** relaxing the engine's preflight requirement; adding real
preflights to every provider (follow-up work per provider); wiring
`RecoverArmed` for genuinely interrupted applies; covering wrapper providers
that delegate rollback to an internal `files`/`filetx` provider.

## Decisions

### Preflight capability comes from the real provider type

`resourceregistry.Resource.ProvidesPreflight` builds the kind's provider
without endpoint facts or runtime capability gating and reports whether it
implements `executor.Preflighter`, the same assertion the engine makes.
Validation resolves each resource's effective risk the same way the engine
does (`Metadata().EffectiveRisk(DefaultRisk())`) and warns when the risk needs
a preflight the provider lacks. Deriving capability from the type means there
is no table to drift. Alternative considered: a static per-kind table plus a
contract test; rejected as redundant. Building providers through `NewProvider`
with empty facts was also rejected, because it returns an unsupported-platform
placeholder that would mark preflight-capable kinds (such as Ubuntu Pro) as
incapable.

### Acknowledge in the engine after a compliant Check

Providers that hold a rollback handle implement
`executor.ConvergedRollbackAcknowledger`. After computing a node's Check, the
engine calls it when the status is `compliant`; failures are logged and do not
change the Check result. `Handle.AcknowledgeConverged` acknowledges every armed
record for the address, ignoring handle ownership, because compliance (not the
arming process) is the evidence of convergence. Acknowledging inside `Apply`
was rejected: it would remove restart-safe revert immediately after a
successful change (`OS-AEC-068`).

## Risks / Trade-offs

- [A resource is compliant by coincidence after an interrupted apply] → the
  prior content is no longer the desired state either way, so restoring it would
  reintroduce drift; acknowledgement only drops the recovery payload.
- [Wrapper providers keep stuck records] → documented non-goal; the affected
  fleet resources are plain `file` kinds.
