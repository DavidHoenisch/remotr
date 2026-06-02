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

Non-interactive (set org explicitly — required when you belong to multiple orgs):

```bash
REMOTR_YES=1 REMOTR_FLY_ORG=archangel curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/deploy/fly/bootstrap.sh | bash
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
7. Deploys the pre-built Docker Hub image (`docker.io/<user>/remotr-server:latest` by default)
8. Waits for the one-time operator bootstrap token
9. Runs `remotr bootstrap` and `remotr enroll token create` locally (if `remotr` or Go is available)
10. Writes `~/.config/remotr/<app>/fly-bootstrap.txt` with URLs and tokens

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_APP_NAME` | `remotr-<random>` | Fly.io app name |
| `REMOTR_FLY_REGION` | `iad` | Fly region |
| `REMOTR_FLY_ORG` | (prompt if multiple) | Fly organization slug — required for non-interactive runs when you have more than one org |
| `REMOTR_NEON_PROJECT` | same as app name | Neon project name |
| `REMOTR_NEON_REGION` | `aws-us-east-1` | Neon region id |
| `REMOTR_FLEET` | `default` | Initial fleet name |
| `REMOTR_IMAGE` | `docker.io/$REMOTR_DOCKER_USER/remotr-server:latest` | Docker image to deploy |
| `REMOTR_DOCKER_USER` | see `deploy/fly/defaults.env` | Docker Hub user for default image |
| `REMOTR_IMAGE_TAG` | `latest` | Image tag when `REMOTR_IMAGE` is unset |
| `REMOTR_BUILD_FROM_SOURCE` | unset | Set to `1` to build `deploy/fly/Dockerfile` on Fly instead |
| `REMOTR_STATE_DIR` | `~/.config/remotr/<app>` | Local operator + CA files |
| `REMOTR_YES` | unset | Skip confirmation prompt |
| `REMOTR_NEON_REUSE` | unset | Reuse existing Neon project with the same name |
| `REMOTR_DATABASE_URL` | (create Neon project) | Use existing Postgres instead of Neon |
| `REMOTR_SKIP_OPERATOR` | unset | Skip CLI bootstrap / enroll token |
| `REMOTR_FLY_SKIP_IPV4` | unset | Skip dedicated IPv4 allocation (~$2/mo) |

## Architecture notes

### mTLS on Fly.io

Agents connect with **client TLS certificates**. Fly's edge must **not** terminate TLS for Remotr traffic. The generated `fly.toml` uses **TCP passthrough** on ports `443` and `8443` with no HTTP/TLS handlers.

Because of that, Fly cannot use a **shared** IPv4 address. Bootstrap allocates:

- **Dedicated IPv6** (free) — always
- **Dedicated IPv4** (~$2/mo) — default; skip with `REMOTR_FLY_SKIP_IPV4=1`

Without at least one dedicated IP, `https://<app>.fly.dev` will not resolve in DNS. If you declined IPs during an interactive `fly deploy`, fix an existing app with:

```bash
fly ips allocate-v6 -a <app-name>
fly ips allocate-v4 -y -a <app-name>   # optional but recommended for IPv4-only networks
```

Server URL for agents and operators:

```text
https://<app-name>.fly.dev
```

### Starter configuration repository

The Fly bootstrap pulls the published image from Docker Hub (built by `.github/workflows/remotr-server-docker.yml`). It bundles `deploy/fly/config-repo/` at `/config-repo` with `fleets/default/desired.yaml`. Replace this with your own GitOps repo by:

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
| `REMOTR_GIT_TOKEN` | GitHub PAT for private config repo (with `REMOTR_GIT_REMOTE_URL`) |

## Manual operations

Redeploy after changes (same image tag or pin a version):

```bash
fly deploy --config deploy/fly/fly.toml --image docker.io/<user>/remotr-server:latest -a <app-name>
```

Build from source on Fly instead of Docker Hub:

```bash
REMOTR_BUILD_FROM_SOURCE=1 ./deploy/fly/bootstrap.sh
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
| `jq: parse error` after Neon create | Neon returned plain-text `ERROR:` (not JSON). Re-run with `REMOTR_NEON_REUSE=1`, set `REMOTR_DATABASE_URL`, or fix region/org limits (`neonctl me`) |
| `dockerfile ... not found` on deploy | Update bootstrap script (image deploy) or set `REMOTR_IMAGE` to a published Hub image |
| Image pull failed | Confirm `docker pull <user>/remotr-server:latest` works; override with `REMOTR_IMAGE` |
| Agent TLS errors | Use CA from `~/.config/remotr/<app>/ca.crt` |
| `remotr-*.fly.dev` does not resolve | App has no dedicated IPs — run `fly ips allocate-v6` and `fly ips allocate-v4 -y` (TCP/mTLS cannot use shared IPv4) |
| Crash loop: `read ca cert: path must be absolute` | Redeploy image with entrypoint (≥ latest after fix); bootstrap stores PEM in Fly secrets, entrypoint writes them to `/run/remotr/certs` |
| `TLS handshake error ... EOF` every ~15s | Harmless — was Fly `tcp_checks` probing a TLS port; removed from `fly.toml`. App is fine if `/healthz` works |
| Schema errors on Neon | Ensure `psql` or Docker is available locally |

More: [Troubleshooting](../../docs/guides/troubleshooting.md)

## Related docs

- [Production deployment](../../docs/guides/production-deployment.md)
- [Operator workflows](../../docs/guides/operator-workflows.md)
- [Agent deployment](../../docs/guides/agent-deployment.md)
