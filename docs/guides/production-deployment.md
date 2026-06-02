# Production deployment

This guide covers deploying Remotr outside the Docker Compose dev stack: server on infrastructure you control, agents on Linux endpoints, and a Git-hosted configuration repository.

Assumes Postgres, TLS material, and outbound network access from agents to the server.

## Overview

```text
1. Generate Remotr CA + server TLS cert
2. Provision Postgres + apply schema
3. Deploy remotr-server with config repo checkout
4. Bootstrap first operator
5. Scaffold/register fleets in Git + Postgres
6. Enroll agents with one-time tokens
7. Configure Git sync (webhook + poll)
```

### Fastest path: Fly.io + Neon

If you use Fly.io and Neon, run the bootstrap installer (creates the database, Fly app, secrets, deploy, and operator CLI setup):

```bash
curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/deploy/fly/bootstrap.sh | bash
```

See [deploy/fly/README.md](../../deploy/fly/README.md) for options and architecture notes.

The manual steps below apply when you host the server yourself (VM, Kubernetes, etc.).

## 1. Certificate authority and server TLS

Generate a Remotr CA and sign a server certificate. The Compose script `compose/scripts/gen-certs.sh` is a reference for development; production should use your org's key ceremony (offline CA, HSM, or approved PKI process).

Server needs:

| File | Env var |
|------|---------|
| CA certificate | `REMOTR_CA_CERT`, `REMOTR_TLS_CLIENT_CA` |
| CA private key | `REMOTR_CA_KEY` |
| Server certificate | `REMOTR_TLS_CERT` |
| Server private key | `REMOTR_TLS_KEY` |

Distribute **CA certificate only** to operators and agents (`/etc/remotr/ca.crt`). Never distribute `REMOTR_CA_KEY` off the server host.

See [CA rotation](../runbooks/ca-rotation.md) for rotation procedures.

## 2. Postgres

Apply schema from `sql/schema.sql`:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f sql/schema.sql
```

Create a dedicated database user with least privilege on the `remotr` database.

`REMOTR_DATABASE_URL` is required for enrollment, admin API, drift telemetry, and release ref persistence.

## 3. Configuration repository

Create and push a Git repository:

```bash
remotr init -fleet production ./remotr-config
cd remotr-config
git init && git remote add origin git@github.com:org/remotr-config.git
git add . && git commit -m "Initial fleet configuration"
git push -u origin main
```

Clone on the server host:

```bash
git clone git@github.com:org/remotr-config.git /var/lib/remotr/config-repo
```

Register fleet in Postgres (if not done via `init --register-server`):

```sql
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ('production', 'auto')
ON CONFLICT (fleet) DO NOTHING;
```

## 4. Server deployment

### Docker Hub image

CI publishes `remotr-server` to Docker Hub on changes to `cmd/remotr-server`, `internal/`, and related build files.

**Repository secrets** (Settings → Secrets → Actions):

| Secret | Description |
|--------|-------------|
| `DOCKERHUB_USERNAME` | Docker Hub username or org |
| `DOCKERHUB_TOKEN` | Docker Hub access token ([create](https://hub.docker.com/settings/security)) |

Create the repository `remotr-server` on Docker Hub before the first push.

```bash
docker pull <dockerhub-user>/remotr-server:latest
docker run --rm -p 8443:8443 \
  -e REMOTR_DATABASE_URL=postgres://... \
  -e REMOTR_CONFIG_REPO=/config-repo \
  -v /path/to/config-repo:/config-repo:ro \
  -v /etc/remotr/certs:/certs:ro \
  -v remotr-state:/var/lib/remotr \
  <dockerhub-user>/remotr-server:latest
```

Tags: `latest` on default branch, commit SHA, branch name, and semver on `v*` git tags.

Build locally:

```bash
make docker-server-build
# or
docker build -f docker/remotr-server/Dockerfile -t remotr-server:local .
```

The runtime image is **Alpine 3.21** with `ca-certificates` and `git` (for Git sync).

### Environment file

Example environment (`/etc/remotr/server.env`):

```bash
REMOTR_LISTEN=:8443
REMOTR_CONFIG_REPO=/var/lib/remotr/config-repo
REMOTR_DATABASE_URL=postgres://remotr:***@db.internal:5432/remotr?sslmode=require
REMOTR_TLS_CERT=/etc/remotr/certs/server.crt
REMOTR_TLS_KEY=/etc/remotr/certs/server.key
REMOTR_TLS_CLIENT_CA=/etc/remotr/certs/ca.crt
REMOTR_CA_CERT=/etc/remotr/certs/ca.crt
REMOTR_CA_KEY=/etc/remotr/certs/ca.key
REMOTR_BOOTSTRAP_FILE=/var/lib/remotr/bootstrap.token
REMOTR_GIT_REMOTE_URL=git@github.com:org/remotr-config.git
REMOTR_GIT_BRANCH=main
REMOTR_GIT_SYNC_POLL_INTERVAL=10m
REMOTR_GIT_WEBHOOK_SECRET=<random-32-bytes>
```

Run `remotr-server` under systemd with `EnvironmentFile=/etc/remotr/server.env`.

On first start, capture the **bootstrap token** from journal logs or `REMOTR_BOOTSTRAP_FILE` before it is consumed.

Place the server behind a load balancer or reverse proxy only if TLS passthrough preserves client certificate forwarding for mTLS — terminating TLS at a proxy breaks agent and operator client auth unless you configure mutual TLS end-to-end.

## 5. Operator bootstrap

From a trusted workstation:

```bash
remotr bootstrap \
  --server-url https://remotr.internal:8443 \
  --ca /etc/remotr/ca.crt \
  --token "$(cat bootstrap.token)" \
  --state-dir ~/.config/remotr
```

Verify:

```bash
remotr endpoint list --server-url https://remotr.internal:8443
```

## 6. Git sync webhook

Configure your forge to POST on push to `main`:

- URL: `https://remotr.internal:8443/v1/webhooks/git`
- Header: `X-Remotr-Git-Webhook-Secret: <REMOTR_GIT_WEBHOOK_SECRET>`
- Method: POST

Poll interval (`REMOTR_GIT_SYNC_POLL_INTERVAL`) is the fallback when webhooks fail.

## 7. Enroll endpoints

For each machine:

```bash
# operator
remotr enroll token create \
  --server-url https://remotr.internal:8443 \
  --fleet production \
  --ttl 24h

# endpoint (once)
remotr-agent enroll \
  --server-url https://remotr.internal:8443 \
  --ca /etc/remotr/ca.crt \
  --token "$TOKEN" \
  --state-dir /var/lib/remotr
```

Enable the sync systemd unit — see [Agent deployment](agent-deployment.md).

Confirm enrollment:

```bash
remotr endpoint list --server-url https://remotr.internal:8443
```

## 8. Hardening checklist

| Item | Recommendation |
|------|----------------|
| CA private key | Root-only, no backups in Git |
| Bootstrap token | Read once, delete file after bootstrap |
| Enrollment tokens | Short TTL, single use, secure delivery |
| Postgres | TLS, strong password, network isolation |
| Server listen | Firewall to agent/operator networks only |
| Agent | Root service account (required for apply) |
| Config repo | Branch protection, required reviews on `main` |
| Audit | Retain server logs and Git history |

## 9. Upgrades

1. Build new binaries with `-mod=vendor`.
2. Rolling restart `remotr-server` (agents retry sync).
3. Rolling restart `remotr-agent` on endpoints or package via your config management.

Run `make test` and `make test-e2e` before promoting a release.

## Related docs

- [Getting started](../tutorial/getting-started.md) — local Compose equivalent
- [Operator workflows](operator-workflows.md)
- [Agent deployment](agent-deployment.md)
- [Environment variables](../reference/environment-variables.md)
- [Troubleshooting](troubleshooting.md)
