# Operator workflows

The `remotr` binary is the **Admin CLI**. Operators change desired state through Git (the configuration repository). Server registry operations — bootstrap, enrollment tokens, endpoint inventory — go through the Admin CLI over mTLS using **operator credentials**, not endpoint credentials.

There is no web UI in v1.

## Credential model

| Credential | Purpose | Stored on |
|------------|---------|-----------|
| Operator bootstrap token | One-time; creates first operator | Server stdout + `REMOTR_BOOTSTRAP_FILE` |
| Operator credential | Admin API (`/v1/admin/*`) | `~/.config/remotr/` or `REMOTR_OPERATOR_STATE_DIR` |
| Enrollment token | One-time; binds endpoint to fleet | Created by operator; delivered to installer |
| Endpoint credential | Ongoing sync (`/v1/sync`) | Agent `/var/lib/remotr/` |

All TLS identities are issued by the **Remotr CA** (`REMOTR_CA_CERT` / `REMOTR_CA_KEY` on the server).

## Bootstrap the first operator

When the server starts with Postgres and no registered operators, it generates a bootstrap token.

1. Read the token from server logs or the bootstrap file (default `/var/lib/remotr/bootstrap.token` on the server host).
2. Exchange it:

```bash
remotr bootstrap \
  --server-url https://remotr.example:8443 \
  --ca /etc/remotr/ca.crt \
  --token "$BOOTSTRAP_TOKEN" \
  --state-dir ~/.config/remotr
```

3. Confirm credentials exist:

```bash
ls ~/.config/remotr/
# operator.crt  operator.key  ca.crt  state.json
```

The bootstrap token and file are invalidated after a successful exchange. Additional operators will use **operator issuance** (planned API); until then, coordinate bootstrap carefully or use shared break-glass procedures documented in your org.

## Create enrollment tokens

Each new machine needs a one-time enrollment token tied to a **fleet**:

```bash
remotr enroll token create \
  --server-url https://remotr.example:8443 \
  --fleet engineering \
  --ttl 168h \
  --state-dir ~/.config/remotr
```

Output includes the token string and expiry. Deliver the token securely to whoever installs the agent (SSH session, secrets manager, or short-lived file on a provisioning USB).

Tokens are consumed at enroll and cannot be reused.

### Register a fleet before enrolling

Fleets must exist in Postgres `fleet_settings` (remediation policy) before enrollment tokens work.

**Option A — scaffold with registration:**

```bash
export REMOTR_DATABASE_URL='postgres://...'
remotr init ./remotr-config \
  -fleet engineering \
  -policy auto \
  --register-server \
  --enroll \
  --enroll-out /secure/enroll.token
```

**Option B — SQL (when adding a fleet manually):**

```sql
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ('engineering', 'auto')
ON CONFLICT (fleet) DO NOTHING;
```

Remediation policy values:

- `auto` (default) — agent applies changes when drift is detected on sync
- `report` — agent reports drift only; no mutation until policy changes or an operator intervenes

## List and inspect endpoints

Human-readable list:

```bash
remotr endpoint list --server-url https://remotr.example:8443
```

JSON for scripts:

```bash
remotr endpoint list --server-url https://remotr.example:8443 --json
```

Show one endpoint (labels, last drift report):

```bash
remotr endpoint show --server-url https://remotr.example:8443 <endpoint-id>
remotr endpoint show --server-url https://remotr.example:8443 <endpoint-id> --json
```

### Labels

Endpoints report **labels** in the sync request body (for example `site=berlin`, `role=web`). Labels appear in admin queries; v1 does **not** use labels to select configuration paths. Assignment is fleet enrollment plus optional `endpoints/<id>/desired.yaml` override only.

## Publish configuration changes

Desired state changes never go through the Admin CLI. Workflow:

1. Edit `fleets/<fleet>/desired.yaml` (or an endpoint override) in the configuration repository.
2. Open a pull request; review in Git as usual.
3. Merge to the tracked branch (for example `main`).
4. Git sync advances the **release ref** on the server (webhook or poll).
5. Agents pick up the new artifact digest on the next sync.

See [Configuration repository](configuration-repository.md) for layout and override semantics.

## Git sync and release ref

The server serves artifacts from a checkout at `REMOTR_CONFIG_REPO`. The **release ref** is the Git commit SHA agents receive with each artifact.

Configure Git sync on the server:

| Variable | Purpose |
|----------|---------|
| `REMOTR_GIT_REMOTE_URL` | Remote to `git fetch` (optional if repo is updated by external process) |
| `REMOTR_GIT_BRANCH` | Branch to track (default `main`) |
| `REMOTR_GIT_SYNC_POLL_INTERVAL` | Periodic fetch (for example `5m`); `0` disables polling |
| `REMOTR_GIT_WEBHOOK_SECRET` | Shared secret for `X-Remotr-Git-Webhook-Secret` header |

Webhook endpoints (POST):

- `/v1/webhooks/git`
- `/v1/admin/git-sync`

Example forge hook (generic):

```bash
curl -X POST https://remotr.example:8443/v1/webhooks/git \
  -H "X-Remotr-Git-Webhook-Secret: $SECRET"
```

If the config repo is not a Git checkout (plain directory mount), set `REMOTR_RELEASE_REF` to a static label; the server will not advance ref automatically.

## Certificate maintenance

See [CA rotation runbook](../runbooks/ca-rotation.md) for full CA rotation, endpoint re-enrollment, and operator cert replacement.

Quick endpoint cert refresh (CA unchanged):

```bash
remotr enroll token create --server-url ... --fleet engineering
# on endpoint:
remotr-agent enroll --token ... --force --server-url ... --ca ...
```

## Environment summary

Operator CLI flags accept `--state-dir` (default `~/.config/remotr`). Override with `REMOTR_OPERATOR_STATE_DIR`.

Server-side Postgres is required for bootstrap, enrollment tokens, drift telemetry, and dynamic release ref. See [Environment variables](../reference/environment-variables.md).
