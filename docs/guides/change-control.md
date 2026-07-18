# Review and control high-risk changes

Remotr change control records a hash-bound review plan, freezes its endpoint
targets, collects operator approvals, and creates a bounded rollout
authorization. Use it to review the exact fleet, release, resources, and
targets being authorized rather than approving a mutable label such as
“latest.”

## Current support boundary

!!! danger "Do not treat change control as a fleet-wide safety boundary yet"
    High-risk `remotr:...@active` secret resolution is currently wired to this
    authorization gate. Generic Git desired-state releases do **not**
    automatically create change requests, and the agent does not yet consume
    returned execution leases to gate every high-risk resource Apply.
    High-risk resources are currently governed by their resource-specific
    preflight plus `enforce: true` where required.

    Keep Git review, a `report`-mode canary fleet, resource-specific guarded
    transactions, backups, and console recovery in place. Do not cite a
    `change authorize` result as proof that every desired-state mutation was
    lease-gated.

The operator-facing workflow is useful today for:

- authorizing high-risk uses of an activated server-managed secret;
- inspecting the immutable review evidence associated with those uses;
- creating reviewed baseline-adoption requests from an explicit JSON plan;
- exercising the request lifecycle through the Admin API and CLI.

The execution-progress, automatic baseline-promotion, and break-glass models
are not currently exposed as complete operator workflows. This guide does not
present them as available controls.

Production change-control state uses a versioned Postgres snapshot. The server
requires `REMOTR_DATABASE_URL`, restores the registry before serving, and
persists requests, approvals, rollout and baseline authorization, lifecycle,
outcomes, policies, audit history, break-glass state, execution progress,
leases, and attempt accounting in the singleton
`change_control_state` row. Apply migration `016_change_control_state.sql`
before starting this server version. An unreadable, invalid, or
stale-revision state fails closed instead of silently starting an empty
registry.

Each mutation commits one complete revision before its API or Sync operation
reports success. A failed commit leaves the prior state in force. Continue to
verify lifecycle controls after maintenance as an operational check, but a
successful pause or revoke is durable across an ordinary server restart.

## Terms

| Term | Meaning |
| --- | --- |
| Change request | Immutable review evidence plus mutable authorization state. |
| Release ref | The Git revision associated with the plan. |
| Artifact digest | Digest of the composed artifact being reviewed. |
| Desired hash | Hash binding authorization to one resource's intended state. |
| Frozen target | An endpoint included when the request was created. New endpoints do not silently join it. |
| Authorization group | High-risk resources reviewed and authorized together. |
| Rollout authorization | Time- and scope-bounded approval carrying attempt and concurrency limits. |
| Execution lease | A five-minute, endpoint-specific grant the server model can issue after preflight. Generic agent Apply is not yet wired to require it. |
| Baseline | A fleet/resource/hash/provider authorization derived from verified rollout evidence. Runtime outcome wiring is not yet complete. |

## Risk classes

Resources use one of six risk classes:

| Risk | Intended use |
| --- | --- |
| `normal` | Routine convergence without a high-risk preflight. |
| `sensitive` | Security-sensitive state such as mandatory policy. |
| `connectivity` | DNS, routes, firewall, or network state that can sever management. |
| `access` | Login, sudo, SSH, and account controls that can lock out recovery. |
| `boot` | Kernel or reboot state that can prevent a clean return. |
| `destructive` | Intent that can irreversibly remove or replace important state. |

Every non-`normal` class requires a resource-specific preflight in the change
control model. Many typed applicators also require explicit `enforce: true`.
See each [resource-kind contract](../reference/resource-kinds.md) for the
actual provider behavior.

## Workflow: authorize a high-risk active secret

This is the change-control workflow that is connected end to end today.

### 1. Upload an inactive version

```bash
remotr secret upload wifi/office \
  --file /run/secrets/office-wifi \
  --fleet production
```

The file must be a protected regular file owned by the invoking user, with
mode `0600` or stricter. The command returns safe metadata only.

### 2. Activate the exact version

```bash
remotr secret activate wifi/office 3 --json
```

Activation discovers current `@active` uses. For a high-risk use, the returned
metadata contains a rollout binding and `changeRequestId`. The version is
marked active in the encrypted registry, but the bound endpoint/resource
cannot resolve it until that request has an active rollout authorization.

Exact references such as `remotr:wifi/office@3` are pinned and do not follow
this activation switch. Choose exact or `@active` deliberately when authoring
the resource.

### 3. Review the frozen evidence

```bash
remotr change show <change-id> --json
```

Review at least:

- `fleet`, `release_ref`, and `artifact_digest`;
- `authorization_group` and the strictest `risk`;
- every resource `address`, `desired_hash`, provider, dependency, predicted
  effect, and rollback class;
- every frozen endpoint and its compatibility/preflight evidence;
- `required_approvals`, prior approvals, and `policy_warning`;
- the audit history.

Stop if the release ref is unfamiliar, a hash is missing, a target is
unexpected, or a resource crosses the intended authorization group. Fix the
source and create a new request; do not authorize ambiguous evidence.

### 4. Authorize a bounded rollout

Start with one attempt and one concurrent endpoint:

```bash
remotr change authorize <change-id> \
  --attempt-limit 1 \
  --max-concurrency 1 \
  --justification "CHG-4821: production Wi-Fi rotation"
```

The CLI currently exposes attempt and concurrency bounds. The Admin API also
accepts `valid_from`, `valid_until`, and recurring UTC execution windows. When
omitted, validity starts immediately and ends after 30 days.

The default approval threshold is one distinct operator for non-destructive
risk and two distinct operators for destructive risk. RBAC still determines
whether an operator may call the authorization route. When a request needs a
second approval, run the same command with a different operator credential,
then confirm the state:

```bash
remotr change show <change-id> --json
```

Do not infer final authorization from the first command's formatted
`valid_until` value on a multi-approval request; inspect
`authorization_state` and the approval count.

### 5. Observe and control the request

One snapshot:

```bash
remotr change show <change-id>
```

Poll for a bounded period:

```bash
remotr change watch <change-id> --interval 5s --timeout 10m
```

Without `--timeout`, `watch` behaves like a single `show`.

Stop new lease issuance while investigating:

```bash
remotr change pause <change-id>
```

Resume an already authorized rollout:

```bash
remotr change resume <change-id>
```

Revoke it:

```bash
remotr change revoke <change-id>
```

Pausing and revoking affect the durable rollout gate. They do not undo
material already installed on an endpoint. Revoke or rotate the secret and
change desired state when endpoint cleanup is required.

### 6. Verify the endpoint separately

Because generic lease consumption and outcome reporting are not fully wired,
verify the actual protected resource through its public evidence:

```bash
remotr endpoint state report <endpoint-id> --json
remotr logs list --since 1h --json
```

For connectivity, access, boot, and destructive changes, also use the
resource's documented recovery and provider verification procedure.

## Request states

| State | Meaning |
| --- | --- |
| `pending` | Created but missing the configured approval threshold. |
| `authorized` | A rollout authorization exists and is inside its lifecycle. Time-window checks still apply. |
| `paused` | New lease issuance and active rollout gating are stopped until resume. |
| `revoked` | Rollout gating is stopped. This does not roll back endpoint state. |

The current lifecycle implementation permits `resume` whenever a rollout
authorization exists. Your operational policy should treat revocation as
final and create a new request rather than resuming a revoked request.

## Baseline adoption

`baseline-adopt` creates one reviewed request from a JSON `FleetPlan`. It does
not discover state for you. Generate the plan from trusted inventory and
artifact evidence, review it as a change artifact, and keep it with the change
record.

Example `baseline-plan.json`:

```json
{
  "fleet": "production",
  "release_ref": "4c6ab63d15ce8f4de8b3a614bc84acfe0f2b4d62",
  "artifact_digest": "sha256:8628e9ad5fd596c57a9f9fd4f3378d899cbbef6be96c42b8e472bc558a5e29aa",
  "targets": [
    {
      "endpoint_id": "endpoint-01",
      "compatible": true,
      "preflight_ready": true
    },
    {
      "endpoint_id": "endpoint-02",
      "compatible": true,
      "preflight_ready": false,
      "preflight_reason": "awaiting console recovery check"
    }
  ],
  "resources": [
    {
      "address": "network/office-uplink",
      "desired_hash": "sha256:7255f5e2df3fc4764bea3e77ffc756f785a64ee539db38a57db9a9206c1da0c3",
      "risk": "connectivity",
      "provider": "network-manager",
      "authorization_group": "network-cutover",
      "depends_on": [],
      "activation_targets": ["eth0"],
      "predicted_effects": [
        {
          "code": "network_default_route_replace",
          "details": {
            "fields": [
              {
                "path": "connection_profile",
                "sensitivity": "secret",
                "projection": "presence",
                "present": true
              }
            ]
          }
        }
      ],
      "rollback_class": "transactional",
      "baseline_eligible": true
    }
  ]
}
```

Create and review the request:

```bash
remotr change baseline-adopt \
  --fleet production \
  --file baseline-plan.json \
  --json
```

The URL fleet overrides the JSON `fleet`; keep both equal so the reviewed file
is self-describing. Adoption marks high-risk resources baseline-eligible and
groups them under `baseline-adoption`.

## Baseline promotion

Promotion requires an authorized request, a baseline-eligible non-destructive
resource, and at least one `verified_successful` target outcome. Any failed,
blocked, or unseen target requires an explicit exception acknowledgement:

```bash
remotr change baseline-promote <change-id> \
  --resource network/office-uplink \
  --acknowledge-exceptions
```

!!! warning
    The production agent/server path does not yet record the complete lease
    progress and target outcomes needed for this command in ordinary generic
    desired-state rollouts. A request can therefore remain ineligible for
    promotion even when an endpoint was verified out of band. Do not invent
    or infer successful outcomes.

When active, a baseline is exact: fleet + resource address + desired hash +
provider, and still requires preflight readiness. A changed desired hash
invalidates it in the model.

## Server restart recovery

The registry restores its last successfully persisted revision on an ordinary
single-server restart. Request creation, approvals, rollout authorization,
unexpired lease occupancy, per-endpoint attempt counts, lifecycle state,
baselines, outcomes, progress, policy, and break-glass records survive.

Before a planned restart:

1. Apply all database migrations, including `016_change_control_state.sql`.
2. Back up Postgres and keep `remotr change show <id> --json` with the external
   change record as independent review evidence.
3. Pause new high-risk work and verify that the updated state was returned.
4. Restart one server process; active-active Remotr servers do not share an
   in-process cache refresh protocol and are not documented as supported.
5. Confirm the same IDs, approvals, leases, attempt counts, and lifecycle state
   were restored:

   ```bash
   remotr change list --json
   remotr change show <change-id> --json
   ```

6. Confirm Postgres and server logs show no revision conflict or state decode
   failure, then verify the protected endpoint separately before continuing.

If the `change_control_state` row is missing after a database restore, do not
re-authorize from memory or recreate a request until you have determined why
the backup omitted it. Restore the database and KEKs from a mutually
consistent recovery point. The server intentionally refuses to run without a
Postgres-backed registry.

## Audit and RBAC

Change-control mutations pass through the operator mTLS Admin API and create
server audit events. Use dedicated operator credentials so approvals are
attributable:

```bash
remotr logs list --since 24h --json
remotr rbac operator list --json
```

Keep the external ticket ID in `--justification`. The justification is audit
metadata; it is not a substitute for reviewing hashes and frozen targets.

## Command summary

| Command | Purpose |
| --- | --- |
| `remotr change list` | List requests. |
| `remotr change show ID` | Show review evidence, state, approvals, outcomes, and audit history. |
| `remotr change watch ID --timeout DURATION` | Poll and print bounded snapshots. |
| `remotr change authorize ID ...` | Add an approval and, when the threshold is met, authorize the rollout. |
| `remotr change pause ID` | Stop new active rollout gating/lease issuance. |
| `remotr change resume ID` | Restore an existing authorized rollout to active state. |
| `remotr change revoke ID` | Stop the rollout gate. |
| `remotr change baseline-adopt ...` | Create a request from a reviewed FleetPlan JSON document. |
| `remotr change baseline-promote ...` | Promote a verified eligible resource hash. |

See [CLI reference](../reference/cli.md#change-control) and [HTTP API
reference](../reference/http-api.md) for exact interfaces.
