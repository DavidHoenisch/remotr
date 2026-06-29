# remotr operator workflows (AI quick reference)

## Credential model

| Credential | Used for | Location |
|------------|----------|----------|
| Operator mTLS | Admin API `/v1/admin/*` | `~/.config/remotr/` (or `--state-dir`) |
| Enrollment token | One-time agent enroll | Created by operator; never commit |
| Endpoint cert | Agent sync | On endpoint only |

Operator changes **desired state** via Git (configuration repository). Registry operations use the **remotr CLI** with operator credentials.

## Typical admin flows

### New operator workstation

1. Obtain bootstrap token from server admin.
2. `remotr bootstrap --server-url URL --ca ca.crt --token TOKEN`
3. `remotr doctor`
4. `remotr endpoint list`

### Enroll new machines

1. `remotr enroll token create --fleet engineering --ttl 168h` (or deployment token for bulk).
2. Deliver token securely to installer; use `scripts/install-agent.sh` on the endpoint.
3. Confirm with `remotr endpoint list`.

### Change desired state (GitOps)

1. Edit modules and/or `fleets/<fleet>/manifest.yaml` (or an endpoint manifest) in the config repo.
2. Run `remotr config validate .` locally.
3. Commit and push; server composes artifacts when git sync advances release ref.
4. Agents pull on next sync; verify with `remotr fleet state report --fleet FLEET`.

### Investigate drift

1. `remotr fleet state report --fleet FLEET --verbose`
2. `remotr endpoint show <id>` for one machine.
3. Fix desired state in Git; re-sync.

### Decommission endpoint

1. `remotr endpoint remove <id> --confirm <id>`
2. Remove `endpoints/<id>/manifest.yaml` from config repo if present.
3. Stop `remotr-agent.service` on the host separately.

### Upgrade remotr-agent

1. `remotr fleet agent upgrade --fleet FLEET --version vX.Y.Z`
2. Monitor with `remotr endpoint show <id>` (agent_upgrade section).

## Configuration repository layout

Kind-tagged modular layout (recommended):

```
remotr.yaml
modules/*.yaml              # kind: module
applications/**/*.yaml      # kind: application
crons/**/*.yaml             # kind: crons
fleets/<fleet>/manifest.yaml    # kind: manifest — fleet entry point
endpoints/<id>/manifest.yaml    # optional kind: manifest override
```

The server composes deployable artifacts at release ref advance. Preview with `remotr config render --fleet <name>`.

Run `remotr config validate .` before push.

## Safety rules for AI agents

- Never print or commit bootstrap tokens, enrollment tokens, deployment tokens, or private keys.
- Always use `--confirm` matching resource id for `endpoint remove`, `deployment revoke`, and RBAC deletes in scripts.
- Prefer `--json` for parsing; use `--quiet` when saving tokens with `--out`.
- Run `remotr doctor` before blaming network or TLS issues.
- State report commands exit **4** when drift exists — that is expected, not a CLI failure.
