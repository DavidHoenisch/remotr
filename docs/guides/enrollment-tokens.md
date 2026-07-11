# Enrollment tokens

Each new machine needs a one-time enrollment token tied to a **fleet**.

## Create a one-time token

```bash
remotr enroll token create \
  --server-url https://remotr.example:8443 \
  --fleet engineering \
  --ttl 168h \
  --state-dir ~/.config/remotr
```

![remotr enroll token create](../assets/demo/enroll-token.gif)

Output includes the token string and expiry. Deliver the token securely to whoever installs the agent (SSH session, secrets manager, or short-lived file on a provisioning USB).

Tokens are consumed at enroll and cannot be reused.

## Deployment tokens (bulk / long-lived install)

For many machines or scripted provisioning, create a **deployment token** (reusable until expiry or revoke):

```bash
remotr deployment create --label prod-laptops-2026 --fleet production --ttl 8760h --out /secure/deploy.token
```

![remotr deployment tokens](../assets/demo/deployment.gif)

Send installers a single command with a pinned CA fingerprint (or provide CA material out of band):

```bash
REMOTR_YES=1 \
REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_DEPLOYMENT_TOKEN='paste-token-here' \
REMOTR_CA_FINGERPRINT='sha256-fingerprint-from-trusted-admin-channel' \
bash <(curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh)
```

See [Installing the agent](installing-agent.md) for the full install-script reference (pinned CA bootstrap, environment variables, and out-of-band CA options).

## Register a fleet before enrolling

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

## Related

- [Installing the agent](installing-agent.md)
- [Agent deployment — enrollment](agent-deployment.md#enrollment)
- [Operator overview — credential model](operator-overview.md#credential-model)
