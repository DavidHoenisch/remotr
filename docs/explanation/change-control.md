# How change control is designed

Change control answers a narrower question than Git review: “Which exact
high-risk resource hashes may run on which already-known endpoints, under what
bounds?” Git establishes author intent and release history; a change request
binds runtime authorization to composed evidence.

## Why hashes and frozen targets matter

An approval attached only to “production” or “latest” can drift underneath the
reviewer. Remotr's model freezes:

- fleet and release ref;
- composed artifact digest;
- every included resource address and desired hash;
- provider, predicted effects, and rollback class;
- the target endpoint set and compatibility/preflight evidence.

A later endpoint cannot silently enter an old approval. A changed resource
hash is a different authorization subject. This makes the review evidence
useful even when the operation itself is performed later.

## How resources become requests

Only non-`normal` risk resources are change roots. The planner keeps fleet
boundaries intact and groups roots in two ways:

1. resources with the same explicit `authorizationGroup` are reviewed
   together;
2. otherwise, connected high-risk resources sharing dependency or activation
   relationships form a component.

The request then includes the dependency closure, including normal-risk
prerequisites. A dependency cannot cross into a different explicit
authorization group. The request's risk is the strictest included class.

This is why stable resource addresses matter. Renaming a configuration or
resource changes its address and therefore changes the review and baseline
identity.

## Approval and authorization are different records

An approval records an operator identity, time, and justification. A rollout
authorization is created only when the threshold is met. By default the model
requires one distinct operator for non-destructive risk and two for
destructive risk.

The rollout authorization copies the request's hashes and frozen targets and
adds:

- valid-from and valid-until times;
- maximum attempts per endpoint;
- maximum concurrent active leases;
- optional recurring UTC execution windows.

The CLI currently exposes attempt and concurrency limits and uses immediate
validity for 30 days. The Admin API can express the full time/window model.

## Preflight and execution leases

The intended execution sequence is:

```text
frozen request
  -> distinct approvals
  -> bounded rollout authorization
  -> endpoint-specific preflight
  -> five-minute execution lease
  -> prepare/apply/verify/acknowledge or rollback
  -> target outcome
```

The server issues a lease only when the rollout is active, the endpoint is a
compatible frozen target whose frozen and current preflight evidence is ready,
its attempt limit remains, and the request's concurrency bound has capacity. A
lease carries the exact resource hashes. For rollback-advertising resources,
the endpoint Check path reserves and releases the protected recovery payload
capacity before claiming readiness. A failed normal prerequisite is propagated
as a block to the affected high-risk resource and authorization group.

Break glass can shorten the ordinary approval path, but it is not an escape
from plan safety. A new break-glass record names an existing canonical Change
request; Fleet, risk, resource hashes, provider evidence, and targets are
server-derived from that request. Creation and each use require compatible
frozen targets, exact hashes, classified predicted effects, resource-level
preflight readiness, dependency closure, and rollback-reservation evidence.
Legacy unbound break-glass records remain readable after restore but cannot be
used. The model is persisted, but no complete operator-facing break-glass
workflow is currently exposed.

The types and server Sync response support this model, but the current agent
does not consume execution leases as the generic gate for resource Apply and
does not report the complete progress/outcome lifecycle. Typed high-risk
providers instead use their local preflight, explicit enforcement opt-in, and
guarded rollback mechanisms. This implementation boundary is operationally
significant, not merely a documentation detail.

## Baselines

A baseline is an authorization for one exact tuple:

```text
fleet + resource address + desired hash + provider
```

It still requires a ready preflight. Changing the desired hash invalidates the
baseline in the model. Promotion requires an authorized, baseline-eligible,
non-destructive resource and at least one verified successful target outcome;
exceptions must be acknowledged explicitly.

Because ordinary agent rollout outcomes are not fully wired today, operators
must not interpret the presence of baseline APIs as automatic evidence of a
successful fleet deployment.

## Where change requests currently come from

The complete connected trigger today is activation of a high-risk
`remotr:...@active` secret use. The server derives its request from the proposed
safe secret-version identity, the composed registered Resource, and current
authenticated endpoint evidence. Activation records the resulting effective
resource hash and binds later resolution to an active change request.

The CLI can ask the server to derive a baseline-adoption request for one Fleet.
It supplies no hashes, providers, or effects; those facts come from the current
composed artifact, registered provider contracts, and a current authenticated
schema-9 endpoint report cohort that matches the exact Release, artifact,
provider revision, and canonical resource hashes. Every registered endpoint is
then frozen with its ready, blocked, missing, stale, or incompatible evidence;
the request fails closed when no cohort reproduces the canonical plan. Generic
Git sync does not currently plan every high-risk desired-state diff into a
request.

## Persistence is part of the security property

The production registry stores a versioned snapshot in the Postgres
`change_control_state` singleton. The snapshot schema can represent requests,
approvals, rollout authorizations, baselines, policies, leases, per-target
attempts, outcomes, automatic-promotion policy, and break-glass records. A
persisted mutation uses an expected-revision comparison; a stale writer cannot
overwrite a newer grant. A failed save restores the prior in-memory snapshot.

Every change-control mutation saves the complete new snapshot before reporting
success. This includes lifecycle changes, baseline promotion and invalidation,
outcomes, execution progress, approval and automatic-promotion policy, and
break-glass creation, use, expiry, and revocation. A failed save restores the
prior observable state and authenticated Sync never receives an unpersisted
lease.

Startup loads and strictly validates the persisted payload before serving.
Unknown fields, unsupported state versions, mismatched identifiers, dangling
rollouts, or impossible revisions fail closed. For mutations that save a
revision, this preserves active lease concurrency and attempt accounting
across a process restart rather than silently resetting rollout bounds.

This design assumes one active server process. The persisted revision prevents
two stale processes from overwriting each other, but there is no live cache
refresh protocol between servers. Postgres migration
`016_change_control_state.sql`, database backups, and single-server recovery
are therefore part of the authorization boundary.

## Defense in depth

Change control complements rather than replaces:

- protected Git branches and rendered artifact review;
- canary fleets in `report` mode;
- typed provider validation and bounded ownership;
- network/firewall timed rollback;
- reboot generation, maintenance, and acknowledgement controls;
- access recovery principals and console access;
- audit logs and separately attributable operator credentials.

The operational procedure and current caveats are in [Review and control
high-risk changes](../guides/change-control.md).
