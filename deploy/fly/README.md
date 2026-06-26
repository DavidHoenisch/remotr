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
6. Creates a Fly app, 1GB volume for `/var/lib/remotr`, and a **Tigris** object storage bucket (`fly storage create`) for custom app packages
7. Sets secrets:
   - `REMOTR_DATABASE_URL`
   - `REMOTR_CA_*`, `REMOTR_TLS_*`
   - `REMOTR_GIT_WEBHOOK_SECRET`
   - Tigris (via `fly storage create`): `BUCKET_NAME`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_ENDPOINT_URL_S3`, `AWS_REGION`
8. Deploys the pre-built Docker Hub image (`docker.io/<user>/remotr-server:latest` by default)
9. Waits for the one-time operator bootstrap token
10. Runs `remotr bootstrap` and `remotr enroll token create` locally (if `remotr` or Go is available)
11. Writes `~/.config/remotr/<app>/fly-bootstrap.txt` with URLs and tokens

## Custom app packages (Tigris)

Bootstrap provisions a private [Tigris](https://fly.io/docs/tigris/) bucket on Fly.io for zip-based custom apps. **Only the server** holds S3 credentials (Fly secrets below). Operators publish with operator mTLS — no local bucket env file.

| Fly secret (set by `fly storage create`) | Remotr usage |
|------------------------------------------|--------------|
| `BUCKET_NAME` | Package bucket (also accepts `REMOTR_S3_BUCKET`) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Server upload/presign |
| `AWS_ENDPOINT_URL_S3` | S3 endpoint (also accepts `REMOTR_S3_ENDPOINT`) |
| `AWS_REGION` | Region (`auto` for Tigris) |

Optional server override: `REMOTR_S3_PRESIGN_TTL` (default 30m).

```bash
remotr package build --path ./mycli --push --state-dir ~/.config/remotr/<app>
```

Skip object storage entirely: `REMOTR_SKIP_TIGRIS=1 ./deploy/fly/bootstrap.sh`

See [Custom app packages](custom-app-packages.md).

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
| `REMOTR_SKIP_TIGRIS` | unset | Skip Tigris bucket (`fly storage create`) |
| `REMOTR_TIGRIS_BUCKET` | `<app-name>-packages` | Tigris bucket name when provisioning storage |

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

The Fly bootstrap pulls the published image from Docker Hub (built by `.github/workflows/remotr-server-docker.yml`). It bundles `deploy/fly/config-repo/` at `/config-repo` with a modular starter layout (`modules/`, `fleets/default/manifest.yaml`, composed `desired.yaml`). Replace this with your own GitOps repo when ready (see [Configuration repository](configuration-repository.md)):

1. Setting `REMOTR_GIT_REMOTE_URL` as a Fly secret
2. Mounting or baking your config repo in a custom image
3. Redeploying

See [Configuration repository](configuration-repository.md).

### Secrets reference

| Secret | Purpose |
|--------|---------|
| `REMOTR_DATABASE_URL` | Neon Postgres connection string |
| `REMOTR_CA_CERT` / `REMOTR_CA_KEY` | Issue endpoint + operator certs |
| `REMOTR_TLS_CERT` / `REMOTR_TLS_KEY` | Server HTTPS |
| `REMOTR_TLS_CLIENT_CA` | Verify agent/operator mTLS |
| `REMOTR_GIT_WEBHOOK_SECRET` | Git sync webhook auth |
| `REMOTR_GIT_TOKEN` | GitHub PAT for private config repo (with `REMOTR_GIT_REMOTE_URL`) |
| `BUCKET_NAME` | Tigris bucket for custom app packages (from `fly storage create`) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Tigris credentials for presign/upload |
| `AWS_ENDPOINT_URL_S3` | Tigris S3 endpoint (`https://fly.storage.tigris.dev` or `https://t3.storage.dev`) |
| `AWS_REGION` | Tigris region (`auto`) |

## Enroll Linux endpoints

After bootstrap, enroll machines with the [agent install script](installing-agent.md). Use your Fly app URL and a deployment or enrollment token (from `fly-bootstrap.txt` or `remotr deployment create`):

```bash
REMOTR_YES=1 \
REMOTR_SERVER_URL=https://<app-name>.fly.dev \
REMOTR_DEPLOYMENT_TOKEN='paste-token-here' \
bash <(curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh)
```

The script downloads the agent binary and fetches the public CA from `https://<app-name>.fly.dev/v1/ca.pem` — no separate `ca.crt` file needed. Optional: copy `~/.config/remotr/<app>/ca.crt` if you already have it from bootstrap and set `REMOTR_CA_FILE`.

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

### Rotate Tigris (S3) credentials

Custom app packages store zips in Tigris. Only the **server** needs those credentials (`AWS_*` + `BUCKET_NAME` Fly secrets). Rotate them without printing secrets:

```bash
REMOTR_APP_NAME=remotr-b8108f ./deploy/fly/rotate-tigris-credentials.sh
```

Non-interactive:

```bash
REMOTR_YES=1 REMOTR_APP_NAME=remotr-b8108f ./deploy/fly/rotate-tigris-credentials.sh
```

Prerequisites:

- `fly auth login` and `tigris login` (OAuth) with access to the bucket organization
- `jq` installed locally

The script (blue-green by default):

1. Creates a new Tigris access key scoped to the app bucket
2. Imports `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` into Fly via `fly secrets import` (stdin, never echoed)
3. Waits for redeploy and verifies bucket access
4. Deletes the superseded access key
5. Writes a stamp file at `~/.config/remotr/<app>/s3-credential-rotation.json` (timestamp + key id, **no secret**)

Options:

| Variable | Purpose |
|----------|---------|
| `REMOTR_TIGRIS_BUCKET` | Override bucket name (default: read from Fly app) |
| `REMOTR_TIGRIS_ORG` | Tigris org id (`flyio_...`), org name (`ep-stellarbridgea.app`), or Fly slug (`ep-stellarbridgea-app`). Default: inferred from the Fly app owner org |
| `REMOTR_TIGRIS_ROTATE_IN_PLACE` | Rotate secret on the current key instead of create-and-swap |
| `REMOTR_KEEP_OLD_KEY` | Do not delete the previous access key after success |
| `REMOTR_SKIP_VERIFY` | Skip post-deploy S3 access check |

Do not run with `bash -x` / `set -x` — that would trace secrets into your shell log.

Fly-managed Tigris orgs do not work with `tigris orgs select`; the script switches orgs via an isolated temp `~/.tigris/config.json` using the org **id** (`flyio_...`). List ids with `tigris whoami --json`.

## Troubleshooting

| Issue | Fix |
|-------|-----|
| `missing Neon CLI` | `npm install -g neonctl` then `neon auth` |
| `Fly CLI not authenticated` | `fly auth login` |
| Bootstrap token timeout | `fly logs -a <app>` — token is printed on first boot |
| `jq: parse error` after Neon create | Neon returned plain-text `ERROR:` (not JSON). Re-run with `REMOTR_NEON_REUSE=1`, set `REMOTR_DATABASE_URL`, or fix region/org limits (`neonctl me`) |
| `dockerfile ... not found` on deploy | Update bootstrap script (image deploy) or set `REMOTR_IMAGE` to a published Hub image |
| Image pull failed | Confirm `docker pull <user>/remotr-server:latest` works; override with `REMOTR_IMAGE` |
| Agent TLS errors | Install script fetches `/v1/ca.pem`; or use `REMOTR_CA_FILE=~/.config/remotr/<app>/ca.crt` |
| `remotr-*.fly.dev` does not resolve | App has no dedicated IPs — run `fly ips allocate-v6` and `fly ips allocate-v4 -y` (TCP/mTLS cannot use shared IPv4) |
| Crash loop: `read ca cert: path must be absolute` | Redeploy image with entrypoint (≥ latest after fix); bootstrap stores PEM in Fly secrets, entrypoint writes them to `/run/remotr/certs` |
| `TLS handshake error ... EOF` every ~15s | Harmless — was Fly `tcp_checks` probing a TLS port; removed from `fly.toml`. App is fine if `/healthz` works |
| Schema errors on Neon | Ensure `psql` or Docker is available locally |

More: [Troubleshooting](troubleshooting.md)

## Related docs

- [Production deployment](production-deployment.md)
- [Operator overview](operator-overview.md)
- [Installing the agent](installing-agent.md)
- [Agent deployment](agent-deployment.md)
