# Deploy Remotr in production

This guide deploys one `remotr-server`, Postgres, a Git configuration checkout,
an operator workstation, and Linux endpoint agents. It uses a systemd-managed
server binary so every required path and permission is visible. A container is
equivalent if the same files and lifecycle guarantees are mounted explicitly.

!!! warning "Plan the trust boundary first"
    The server holds the CA private key used to issue operator and endpoint
    certificates. If Remotr-managed secrets are enabled, it also needs an
    external key-encryption-key (KEK) keyring. Protect the server host as a
    certificate authority and secret-processing system.

## Architecture and prerequisites

Prepare:

- a Linux server reachable over HTTPS by every endpoint and operator;
- PostgreSQL with TLS, backups, and a dedicated Remotr database role;
- a DNS name that will not change during enrollment;
- a Git repository and read-only deploy credential;
- a Remotr CA and server certificate whose SAN contains that DNS name;
- a trusted operator workstation with the `remotr` CLI;
- console or recovery access to endpoints before enforcing access, network,
  boot, or destructive resources.

Recommended starting topology:

```text
operators --mTLS--> remotr-server <--mTLS-- endpoint agents
                         |
                         +--> PostgreSQL
                         +--> Redis primary (optional shared Sync cache)
                         +--> read-only Git checkout
                         +--> private S3-compatible bucket (optional apps)
```

Use one server instance with the memory backend, or multiple instances sharing
one Redis primary and namespace. Every modeled change-control mutation is durable in
Postgres, including creation, approval, authorization, lifecycle, baseline,
policy, audit, break-glass, lease, outcome, progress, and attempt state. The
server coordinates only the disposable unchanged-Sync decision cache through
Redis; Postgres remains authoritative.
See
[Change-control restart recovery](change-control.md#server-restart-recovery).

## 1. Install the binaries

Install the operator and agent from a tagged release and verify their published
checksums as described in [Installing the CLI](installing-cli.md). The current
GoReleaser archives do not include `remotr-server`; build the server from the
matching vendored source tree or use the project server container:

```bash
go build -mod=vendor -o ./bin/remotr-server ./cmd/remotr-server
sudo install -o root -g root -m 0755 ./bin/remotr-server /usr/local/bin/remotr-server
```

Create a locked service identity and directories:

```bash
sudo useradd --system --home /var/lib/remotr --shell /usr/sbin/nologin remotr
sudo install -d -o remotr -g remotr -m 0750 /var/lib/remotr
sudo install -d -o remotr -g remotr -m 0750 /var/lib/remotr/config-repo
sudo install -d -o root -g remotr -m 0750 /etc/remotr/certs
sudo install -d -o root -g remotr -m 0750 /etc/remotr/secrets
```

On distributions where `useradd` is elsewhere or the no-login shell has a
different path, use the local equivalent and confirm the resulting UID/GID.

## 2. Provision the CA and server certificate

Production certificate generation belongs to your approved PKI process. The
files consumed by Remotr are:

| File | Owner/mode | Environment |
| --- | --- | --- |
| CA certificate | `root:remotr 0640` | `REMOTR_CA_CERT` and `REMOTR_TLS_CLIENT_CA` |
| CA private key | `root:remotr 0640` or stricter | `REMOTR_CA_KEY` |
| Server certificate | `root:remotr 0640` | `REMOTR_TLS_CERT` |
| Server private key | `root:remotr 0640` or stricter | `REMOTR_TLS_KEY` |

Install them:

```bash
sudo install -o root -g remotr -m 0640 ca.crt /etc/remotr/certs/ca.crt
sudo install -o root -g remotr -m 0640 ca.key /etc/remotr/certs/ca.key
sudo install -o root -g remotr -m 0640 server.crt /etc/remotr/certs/server.crt
sudo install -o root -g remotr -m 0640 server.key /etc/remotr/certs/server.key
```

Verify before starting the service:

```bash
openssl verify -CAfile ca.crt server.crt
openssl x509 -in server.crt -noout -subject -issuer -ext subjectAltName
openssl pkey -in server.key -pubout -outform pem | sha256sum
openssl x509 -in server.crt -pubkey -noout | sha256sum
```

The two final hashes must match. The SAN must contain the exact hostname in
`REMOTR_SERVER_URL`. Distribute only the CA certificate to operators and
endpoints. Never distribute the CA private key.

`compose/scripts/gen-certs.sh` is a development reference, not a production
key ceremony.

## 3. Create and migrate Postgres

Create a dedicated database and role using your database platform's normal
workflow. Keep the DSN out of shell history and Git.

From the release/source tree, apply the base schema and ordered migrations:

```bash
sudo env REMOTR_DATABASE_URL='postgres://remotr:REDACTED@db.internal:5432/remotr?sslmode=require' \
  make migrate
```

`make migrate` uses `scripts/migrate.sh`, applies `sql/schema.sql`, then every
file under `sql/migrations/` in order. It requires `psql` or the script's
supported database helper. Run it before the first server start and before
starting a newer server binary during upgrades.

Migration `020_global_secret_scope.sql` adds the explicit secret-scope
discriminator. It deterministically backfills legacy rows only when exactly one
Fleet or Endpoint identifier is present, rejects ambiguous/neither-scope data,
and does not rewrite authenticated envelope JSON. Back up Postgres and the
complete KEK keyring before applying it. Deploy readers that understand the
new discriminator before enabling `secret upload --global`; database rollback
is unsafe after a global record exists because older schemas cannot represent
that scope.

Verify with a read-only connection:

```bash
psql "$REMOTR_DATABASE_URL" -c 'select 1'
```

Postgres is required for production enrollment, fleet settings, operator/RBAC
state, audit events, telemetry, compiled artifacts, diagnostics, cron history,
app packages, encrypted secret versions, and durable change-control state. The
server executable now refuses to start without `REMOTR_DATABASE_URL`.

## 4. Create and validate the configuration repository

On an administrator workstation:

```bash
REMOTR_DATABASE_URL='postgres://remotr:REDACTED@db.internal:5432/remotr?sslmode=require' \
  remotr init --fleet production --policy report --register-server ./remotr-config
cd ./remotr-config
remotr config validate .
remotr config render --fleet production .
git init
git add .
git commit -m "Initialize production fleet"
git remote add origin git@github.com:ORG/remotr-config.git
git push -u origin main
```

`--register-server` writes the authoritative `report` policy directly to
Postgres. Run it only from a protected host that may reach the database. If
operators are intentionally denied direct database access, have the database
deployment step perform the equivalent idempotent registration before issuing
an enrollment token:

```sql
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ('production', 'report')
ON CONFLICT (fleet) DO UPDATE
SET remediation_policy = EXCLUDED.remediation_policy,
    updated_at = now();
```

Start the fleet in server-side remediation policy `report`. Observe a canary
cohort before changing it to `auto` through the authoritative fleet registry.
The policy hint in `remotr.yaml` is documentation, not the active server value.

Clone with a read-only deploy identity on the server:

```bash
sudo -u remotr git clone --branch main --single-branch \
  git@github.com:ORG/remotr-config.git \
  /var/lib/remotr/config-repo
```

Confirm the service user can fetch and the checkout validates:

```bash
sudo -u remotr git -C /var/lib/remotr/config-repo fetch origin main
remotr config validate /var/lib/remotr/config-repo
```

For HTTPS Git, use `REMOTR_GIT_TOKEN`; Remotr supplies it to Git without
persisting it in `.git/config`. Give the token read-only access to this one
repository.

## 5. Configure server-managed secrets (optional)

Generate a 32-byte KEK with your secret-management system and build the
versioned keyring outside Git:

```json
{
  "active": "kek-2026-07",
  "keys": {
    "kek-2026-07": "<base64-of-exactly-32-random-bytes>"
  }
}
```

The file-backed loader currently requires a root-owned regular file with no
group/other permission bits:

```bash
sudo install -o root -g root -m 0600 kek-keyring.json /etc/remotr/secrets/kek-keyring.json
```

That source can be opened only when the server process has permission to read a
root-only file. The non-root systemd deployment in this guide therefore uses
`REMOTR_SECRET_KEK_KEYRING_B64`, populated from the same keyring by the
deployment secret mechanism, and omits `REMOTR_SECRET_KEK_KEYRING`. Do not
weaken the file mode to make it readable by the service user; the loader will
reject it.

Keep old KEKs in `keys` while any Postgres record references them. Back up the
keyring separately from Postgres. A database backup without its historical
KEKs cannot recover encrypted secret versions. When secrets are enabled, server
startup reads every encrypted version from the restored database, validates its
envelope, and checks the configured current and historical KEKs. Startup fails
closed if any record is malformed or any referenced KEK is unavailable. The
coverage diagnostic serializes only classified key identifiers and secret
references; it never contains ciphertext, wrapped keys, KEKs, or secret bytes.

## 6. Write the environment file

Create `/etc/remotr/server.env` outside the configuration repository:

```bash
REMOTR_LISTEN=:8443
REMOTR_CONFIG_REPO=/var/lib/remotr/config-repo
REMOTR_DATABASE_URL=postgres://remotr:REDACTED@db.internal:5432/remotr?sslmode=require

REMOTR_TLS_CERT=/etc/remotr/certs/server.crt
REMOTR_TLS_KEY=/etc/remotr/certs/server.key
REMOTR_TLS_CLIENT_CA=/etc/remotr/certs/ca.crt
REMOTR_CA_CERT=/etc/remotr/certs/ca.crt
REMOTR_CA_KEY=/etc/remotr/certs/ca.key
REMOTR_BOOTSTRAP_FILE=/var/lib/remotr/bootstrap.token

REMOTR_GIT_REMOTE_URL=git@github.com:ORG/remotr-config.git
REMOTR_GIT_BRANCH=main
REMOTR_GIT_SYNC_POLL_INTERVAL=10m
REMOTR_GIT_WEBHOOK_SECRET=REPLACE_WITH_RANDOM_VALUE

# Set after capacity testing; omitted means no explicit server admission cap.
# REMOTR_SYNC_MAX_CONCURRENT=400
REMOTR_SYNC_RETRY_AFTER=5s
REMOTR_ARTIFACT_PRUNE_AGE=720h

# Single-process memory mode (default).
REMOTR_UNCHANGED_SYNC_BACKEND=memory
REMOTR_SERVER_PROCESSES=1
REMOTR_UNCHANGED_SYNC_MAX_ENTRIES=10000
REMOTR_UNCHANGED_SYNC_MAX_BYTES=67108864
REMOTR_UNCHANGED_SYNC_TTL=10m
REMOTR_UNCHANGED_SYNC_CHECKPOINT_INTERVAL=5m

# For replacement-safe or multi-process operation instead use:
# REMOTR_UNCHANGED_SYNC_BACKEND=redis
# REMOTR_REDIS_URL=rediss://:REDACTED@redis.example:6380
# REMOTR_UNCHANGED_SYNC_REDIS_PREFIX=remotr-production

REMOTR_SECRETS_ENABLED=true
REMOTR_SECRET_KEK_KEYRING_B64=REPLACE_WITH_BASE64_OF_THE_COMPLETE_KEYRING_JSON
```

If secrets are disabled, set `REMOTR_SECRETS_ENABLED=false` and omit both KEK
variables. Never configure both file and base64 keyring sources.

Protect the file:

```bash
sudo chown root:remotr /etc/remotr/server.env
sudo chmod 0640 /etc/remotr/server.env
```

Supply `REMOTR_SECRET_KEK_KEYRING_B64` as strict, single-line standard base64
of the complete JSON document. Prefer having the deployment secret manager
render the environment file; avoid typing the value into shell history.

See [Environment variables](../reference/environment-variables.md) for Git,
S3, admission-control, and fallback variables.

Size the fast path from authenticated load evidence. Its caps include pending
durable checkpoints as well as live decisions. Quiet hits intentionally create
no database traffic; the next Sync after each checkpoint deadline writes one
check-in and one aggregate audit event. Git release changes and global,
Fleet-scoped, or endpoint-scoped authority mutations invalidate affected
entries before mutation. Memory cold-starts after restart. Redis decisions can
survive replacement; all correctness operations must use the primary and
support `EVAL`. If Redis is unavailable, Sync uses Postgres, cache fills stop,
and authority mutations return unavailable before durable changes.

For rollback, set `REMOTR_UNCHANGED_SYNC_FAST_PATH=false` and restart the
server. Agents need no downgrade: optional document hashes are ignored by the
legacy authoritative path, and full documents are requested whenever needed.
Switching between memory and Redis is safe because the cache is disposable.
Use a deployment-unique prefix, keep provider eviction disabled so coordinator
keys are not evicted, and size command volume and key growth from authenticated
load evidence.

## 7. Install the systemd service

`/etc/systemd/system/remotr-server.service`:

```ini
[Unit]
Description=Remotr Linux MDM server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=remotr
Group=remotr
EnvironmentFile=/etc/remotr/server.env
ExecStart=/usr/local/bin/remotr-server
WorkingDirectory=/var/lib/remotr
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/remotr
ReadOnlyPaths=/etc/remotr/certs
CapabilityBoundingSet=
LockPersonality=true

[Install]
WantedBy=multi-user.target
```

Check that the service account can read every configured file before enabling:

```bash
sudo -u remotr test -r /etc/remotr/certs/ca.key
sudo -u remotr test -r /etc/remotr/certs/server.key
sudo -u remotr test -r /etc/remotr/server.env
```

Start and inspect:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now remotr-server
sudo systemctl status remotr-server
sudo journalctl -u remotr-server -n 100 --no-pager
```

If your Git transport needs an SSH agent, known-host state, or writable Git
metadata, configure those explicitly for the `remotr` account. The hardening
above allows writes below `/var/lib/remotr`, including the checkout.

## 8. Verify TLS and health

From a machine holding the trusted CA certificate:

```bash
curl --cacert ca.crt https://remotr.example.internal:8443/healthz
```

Expected output: `ok`.

Inspect the presented identity and chain:

```bash
openssl s_client \
  -connect remotr.example.internal:8443 \
  -servername remotr.example.internal \
  -CAfile ca.crt \
  -verify_return_error </dev/null
```

Do not use `curl -k` as a production verification. If placing a load balancer
or reverse proxy in front, use TCP/TLS passthrough so the server receives the
operator or endpoint client certificate. Ordinary TLS termination at the
proxy breaks Remotr mTLS unless you have deliberately implemented and tested
end-to-end client-certificate forwarding—which the server does not accept as
a substitute header by default.

## 9. Bootstrap the first operator

Read the root/service-protected token once on the server and transfer it over a
trusted channel:

```bash
sudo cat /var/lib/remotr/bootstrap.token
```

On the operator workstation:

```bash
remotr bootstrap \
  --server-url https://remotr.example.internal:8443 \
  --ca /secure/ca.crt \
  --state-dir ~/.config/remotr/production \
  --token -
```

Paste the token on stdin, then verify:

```bash
remotr --state-dir ~/.config/remotr/production doctor
remotr --state-dir ~/.config/remotr/production fleet list
remotr --state-dir ~/.config/remotr/production logs list --since 10m
```

The bootstrap token is single-use. Restrict or delete its file after successful
bootstrap according to your recovery policy. Create later automation
credentials with `remotr admin credential stamp` and minimum RBAC roles; do not
copy the global administrator private key between systems.

## 10. Register the fleet and enroll a canary

The fleet was registered in `report` mode in step 4. If you used the database
provisioning alternative, complete it before this point. Confirm the
authoritative registry entry appears:

```bash
remotr fleet list
```

Create one token:

```bash
remotr enroll token create \
  --fleet production \
  --ttl 1h \
  --out /secure/production-canary.token \
  --quiet
```

Install the endpoint using a CA fingerprint obtained from the trusted CA file:

```bash
openssl x509 -in ca.crt -noout -fingerprint -sha256
```

Follow [Installing the agent](installing-agent.md), then verify:

```bash
remotr endpoint list
remotr endpoint show <endpoint-id>
remotr endpoint state report <endpoint-id> --json
```

Do not switch to automatic remediation until the canary report matches the
rendered artifact and every unsupported/preflight result is understood.

## 11. Configure Git webhook and polling

Configure the Git provider to send:

```text
POST https://remotr.example.internal:8443/v1/webhooks/git
X-Remotr-Git-Webhook-Secret: <configured secret>
```

Polling is the recovery path when a webhook is missed. Test both:

```bash
remotr git sync --json
sudo journalctl -u remotr-server --since '10 minutes ago' --no-pager
```

Confirm that an invalid configuration commit fails composition and leaves the
old release active before relying on this guard.

## Backups and restore requirements

Back up these as one documented recovery set:

- PostgreSQL, with restore testing;
- the CA certificate and private key;
- every active and historical KEK referenced by encrypted secret records;
- Git configuration history and deploy-key recovery;
- the server environment and systemd unit without exposing them to Git;
- S3 package objects and catalog-consistent retention when custom apps are
  used.

The latest saved Change-control snapshot is included in Postgres through the
`change_control_state` row. Treat that row and encrypted secret activation
records as one recovery set. Every successful lifecycle, baseline, outcome,
progress, policy, and break-glass mutation is part of that snapshot. Retain the
external change record as independent review evidence.

Restore in this order:

1. database and network identity;
2. CA/server TLS files and all KEKs;
3. configuration checkout at a known reviewed ref;
4. optional S3 catalog objects;
5. server environment and service;
6. health/TLS/operator verification;
7. restored change-control IDs, approvals, leases, and attempt counts checked
   against the external change record.

## Upgrade procedure

1. Read release notes and compare required Go/Postgres/platform support.
2. Back up Postgres, CA material, KEKs, config ref, and optional S3 state.
3. Pause new high-risk secret activations and export their safe request
   metadata as independent evidence.
4. Install the new binary but do not start it yet.
5. Run `make migrate` from the matching release source.
6. Restart the server and inspect logs.
7. Run the verification gate below.
8. Confirm existing request IDs, approvals, and any required pause or revoke
   were restored before resuming high-risk work.
9. Upgrade a canary endpoint before requesting a fleet agent upgrade.

Verification gate:

```bash
curl --cacert ca.crt https://remotr.example.internal:8443/healthz
remotr doctor
remotr git sync --json
remotr fleet list --json
remotr endpoint list --json
remotr logs list --since 15m --json
```

Then verify at least one endpoint sync and state report. A healthy server alone
does not prove artifact retrieval or agent mTLS.

## Production readiness checklist

- [ ] Server certificate chain and hostname verified without `-k`.
- [ ] CA private key and server key readable only by the service boundary.
- [ ] Postgres TLS, least privilege, backup, and restore test complete.
- [ ] KEK backup/rotation procedure complete, or secrets disabled explicitly.
- [ ] Config repository validates and canary render reviewed.
- [ ] Fleet starts in `report` mode.
- [ ] Git webhook and polling both tested.
- [ ] Operator credential and RBAC recovery documented.
- [ ] Agent enrollment uses an out-of-band CA certificate or fingerprint.
- [ ] Endpoint console recovery exists before high-risk enforcement.
- [ ] Change-control migration, Postgres backup, and single-server restart runbook tested.
- [ ] Fast-path entry/byte limits are based on authenticated load evidence, or the fast path is explicitly disabled.
- [ ] Fast-path rollback and cold-restart recovery are rehearsed; serving process count is one.
- [ ] Upgrade migration and rollback procedure rehearsed.

## Related documentation

- [Installing the CLI](installing-cli.md)
- [Installing the agent](installing-agent.md)
- [Bootstrap an operator](bootstrap-operator.md)
- [Enrollment tokens](enrollment-tokens.md)
- [Environment variables](../reference/environment-variables.md)
- [Configuration repository](configuration-repository.md)
- [Troubleshooting](troubleshooting.md)
