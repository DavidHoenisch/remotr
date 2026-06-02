# Deploy Remotr on Fly.io with Neon Postgres

One-command bootstrap for a production-shaped Remotr server:

- **Fly.io** — `remotr-server` with TCP TLS passthrough (mTLS terminated in the app)
- **Neon** — managed Postgres for the server registry
- **Operator CLI** — bootstrap + first enrollment token on your machine

## Quick start

Install [Fly CLI](https://fly.io/docs/hands-on/install-flyctl/) and [Neon CLI](https://neon.com/docs/reference/neon-cli) (`npm install -g neonctl`), then authenticate:

```bash
fly auth login
neon auth
```

Run the bootstrap installer (pick one):

```bash
# Recommended — keeps your terminal on stdin (prompt works reliably)
bash <(curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/deploy/fly/bootstrap.sh)

# Also works — script prompts via /dev/tty after cloning the repo
curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/deploy/fly/bootstrap.sh | bash
```

You should see a plan summary and `Type yes to continue:` on your terminal.

Non-interactive:

```bash
REMOTR_YES=1 curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/deploy/fly/bootstrap.sh | bash
```

Or from a clone:

```bash
chmod +x deploy/fly/bootstrap.sh
./deploy/fly/bootstrap.sh
```

Non-interactive:

```bash
REMOTR_YES=1 REMOTR_APP_NAME=my-remotr ./deploy/fly/bootstrap.sh
```

## What the script does

1. Verifies `fly` / `neon` CLIs, `git`, `jq`, `openssl`, and `psql` or Docker
2. Clones this repository (when run via `curl | bash`)
3. Creates a Neon project + `remotr` database
4. Applies `sql/schema.sql` and seeds your fleet in `fleet_settings`
5. Generates a Remotr CA + server certificate (`*.fly.dev` SAN)
6. Creates a Fly app, 1GB volume for `/var/lib/remotr`, and sets secrets:
   - `REMOTR_DATABASE_URL`
   - `REMOTR_CA_*`, `REMOTR_TLS_*`
   - `REMOTR_GIT_WEBHOOK_SECRET`
7. Deploys using `deploy/fly/Dockerfile` (Alpine + bundled starter config repo)
8. Waits for the one-time operator bootstrap token
9. Runs `remotr bootstrap` and `remotr enroll token create` locally (if `remotr` or Go is available)
10. Writes `~/.config/remotr/<app>/fly-bootstrap.txt` with URLs and tokens

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_APP_NAME` | `remotr-<random>` | Fly.io app name |
| `REMOTR_FLY_REGION` | `iad` | Fly region |
| `REMOTR_FLY_ORG` | (default org) | Fly organization slug |
| `REMOTR_NEON_PROJECT` | same as app name | Neon project name |
| `REMOTR_NEON_REGION` | `aws-us-east-1` | Neon region id |
| `REMOTR_FLEET` | `default` | Initial fleet name |
| `REMOTR_IMAGE` | (build from source) | Pre-built Docker image |
| `REMOTR_STATE_DIR` | `~/.config/remotr/<app>` | Local operator + CA files |
| `REMOTR_YES` | unset | Skip confirmation prompt |
| `REMOTR_SKIP_OPERATOR` | unset | Skip CLI bootstrap / enroll token |

## Architecture notes

### mTLS on Fly.io

Agents connect with **client TLS certificates**. Fly's edge must **not** terminate TLS for Remotr traffic. The generated `fly.toml` uses **TCP passthrough** on ports `443` and `8443` with no HTTP/TLS handlers.

Server URL for agents and operators:

```text
https://<app-name>.fly.dev
```

### Starter configuration repository

The Fly image bundles `deploy/fly/config-repo/` at `/config-repo` with `fleets/default/desired.yaml`. Replace this with your own GitOps repo by:

1. Setting `REMOTR_GIT_REMOTE_URL` as a Fly secret
2. Mounting or baking your config repo in a custom image
3. Redeploying

See [Configuration repository](../../docs/guides/configuration-repository.md).

### Secrets reference

| Secret | Purpose |
|--------|---------|
| `REMOTR_DATABASE_URL` | Neon Postgres connection string |
| `REMOTR_CA_CERT` / `REMOTR_CA_KEY` | Issue endpoint + operator certs |
| `REMOTR_TLS_CERT` / `REMOTR_TLS_KEY` | Server HTTPS |
| `REMOTR_TLS_CLIENT_CA` | Verify agent/operator mTLS |
| `REMOTR_GIT_WEBHOOK_SECRET` | Git sync webhook auth |

## Manual operations

Redeploy after changes:

```bash
fly deploy --config deploy/fly/fly.toml -a <app-name>
```

View logs:

```bash
fly logs -a <app-name>
```

SSH:

```bash
fly ssh console -a <app-name>
```

## Troubleshooting

| Issue | Fix |
|-------|-----|
| `missing Neon CLI` | `npm install -g neonctl` then `neon auth` |
| `Fly CLI not authenticated` | `fly auth login` |
| Bootstrap token timeout | `fly logs -a <app>` — token is printed on first boot |
| Agent TLS errors | Use CA from `~/.config/remotr/<app>/ca.crt` |
| Schema errors on Neon | Ensure `psql` or Docker is available locally |

More: [Troubleshooting](../../docs/guides/troubleshooting.md)

## Related docs

- [Production deployment](../../docs/guides/production-deployment.md)
- [Operator workflows](../../docs/guides/operator-workflows.md)
- [Agent deployment](../../docs/guides/agent-deployment.md)
