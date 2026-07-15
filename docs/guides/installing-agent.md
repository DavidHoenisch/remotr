# Installing the endpoint agent

Linux endpoints run `remotr-agent` as a systemd service. The recommended path is **`scripts/install-agent.sh`**: a curl-pipeable installer that downloads the release binary, installs trusted Remotr CA material, enrolls, and enables the sync service.

Admins typically send end users one command that includes the **server URL**, **enrollment token**, and either a pinned CA fingerprint or CA material supplied out of band.

See [Agent deployment](agent-deployment.md) for enrollment details, systemd layout, re-enrollment, and troubleshooting after install.

## Admin workflow

1. **Create a deployment token** (reusable, good for many machines) or a one-time enrollment token:

```bash
# long-lived / bulk installs
remotr deployment create --label prod-laptops --fleet production --ttl 8760h

# single machine
remotr enroll token create --fleet production --ttl 24h
```

![remotr deployment and enrollment tokens](../assets/demo/deployment.gif)

2. **Copy the token** from CLI output (deployment tokens are shown once; use `--out` to write to a file).

3. **Send the install command** to the user (run on the endpoint; needs root):

```bash
curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh | \
sudo REMOTR_YES=1 \
REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_DEPLOYMENT_TOKEN='paste-token-here' \
REMOTR_CA_FINGERPRINT='sha256-fingerprint-from-trusted-admin-channel' \
bash
```

Do **not** prefix with `sudo` on the whole line using `bash <(curl ...)` — that breaks with `/dev/fd/N: No such file or directory`. Put `REMOTR_*` on `sudo bash` (not on `curl`). Or open a root shell (`sudo -i`), then run the install command there before typing `exit`.

Replace `REMOTR_DEPLOYMENT_TOKEN` with `REMOTR_ENROLL_TOKEN` for a one-time token.

4. **Confirm** the endpoint appears:

```bash
remotr endpoint list --server-url https://remotr.example:8443
```

![remotr endpoint list](../assets/demo/endpoint-list.gif)

## Paste-and-run (end user)

**First boot** (enroll when the machine powers on, not during image build):

```bash
curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh | \
sudo REMOTR_YES=1 \
REMOTR_DEFER_ENROLL=1 \
REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_DEPLOYMENT_TOKEN='your-uuid.hexsecret' \
REMOTR_CA_FINGERPRINT='sha256-fingerprint-from-trusted-admin-channel' \
bash
```

**Non-interactive** (automation, IM paste, cloud-init):

```bash
curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh | \
sudo REMOTR_YES=1 \
REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_DEPLOYMENT_TOKEN='your-uuid.hexsecret' \
REMOTR_CA_FINGERPRINT='sha256-fingerprint-from-trusted-admin-channel' \
bash
```

**Interactive** (user types `yes` to confirm):

```bash
curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh | \
sudo REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_DEPLOYMENT_TOKEN='your-uuid.hexsecret' \
REMOTR_CA_FINGERPRINT='sha256-fingerprint-from-trusted-admin-channel' \
bash
```

If the script cannot prompt (no TTY), set `REMOTR_YES=1`. From a root shell only, `bash <(curl ...)` also works.

**From a clone** (development):

```bash
sudo REMOTR_YES=1 REMOTR_SERVER_URL=... REMOTR_ENROLL_TOKEN=... REMOTR_CA_FINGERPRINT=... ./scripts/install-agent.sh
```

### Requirements on the endpoint

- Linux **amd64** or **arm64**
- **root** (apply operations need root)
- **systemd** (unless `REMOTR_SKIP_SYSTEMD=1`)
- Outbound HTTPS to the Remotr server
- `curl`, `tar`, `install(1)`
- Optional: `jq` (`REMOTR_VERSION=latest`), `sha256sum` (`REMOTR_VERIFY_CHECKSUMS=1`)
- `openssl` when using `REMOTR_CA_FINGERPRINT` (required for the default server CA download)

## Remotr CA (public, auto-fetched)

The Remotr **CA certificate is not a secret**. It is public key material used to verify the server and issued client certificates. Operators must not distribute `REMOTR_CA_KEY` (private key); only the CA cert leaves the server host.

### How the install script gets the CA

When you do not set `REMOTR_CA_FILE`, `REMOTR_CA_PEM`, or `REMOTR_CA_URL`, the script:

1. Requires `REMOTR_CA_FINGERPRINT` for the default bootstrap path
2. Downloads PEM from **`GET ${REMOTR_SERVER_URL}/v1/ca.pem`** (unauthenticated; see [HTTP API](../reference/http-api.md#get-v1capem))
3. Verifies the downloaded CA certificate against the fingerprint
4. Saves it to **`/etc/remotr/ca.crt`**
5. Uses that file for enrollment and the sync service (`REMOTR_TLS_CA` in `/etc/remotr/agent.env`)

The server TLS certificate is signed by the same Remotr CA, so the bootstrap download cannot be authenticated by normal TLS validation. The fingerprint must come from a trusted out-of-band admin channel, not from `curl -k` against the same server URL. All later steps (health check, enroll, sync) verify TLS with the downloaded and pinned CA.

Determine the CA fingerprint from a trusted server console, operator workstation, or copied CA file:

```bash
openssl x509 -in /path/to/trusted/ca.crt -noout -subject -fingerprint -sha256
```

### Pin the CA fingerprint

Include a sha256 fingerprint in the install command (from `openssl x509 -fingerprint -sha256`):

```bash
curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh | \
sudo REMOTR_YES=1 \
REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_DEPLOYMENT_TOKEN='...' \
REMOTR_CA_FINGERPRINT='ab:cd:ef:...' \
bash
```

The script rejects the download if the fingerprint does not match.

### Override CA source

| Variable | When to use |
|----------|-------------|
| `REMOTR_CA_FILE` | CA already on disk (air-gapped mirror) |
| `REMOTR_CA_PEM` | Inline PEM in the command (no separate file) |
| `REMOTR_CA_URL` | Custom URL over already-trusted HTTPS, or with `REMOTR_CA_FINGERPRINT` for pinned bootstrap |

## Install script environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_SERVER_URL` | (required) | Server base URL, e.g. `https://remotr.example:8443` |
| `REMOTR_DEPLOYMENT_TOKEN` | (unset) | Reusable deployment enrollment token |
| `REMOTR_ENROLL_TOKEN` | (unset) | One-time enrollment token |
| `REMOTR_ENROLL_TOKEN_FILE` | (unset) | Path to token file (prefer over inline in production) |
| `REMOTR_CA_FINGERPRINT` | (unset) | Required sha256 pin for default server CA auto-fetch; optional for out-of-band CA sources |
| `REMOTR_CA_FILE` | (unset) | Use existing CA file instead of auto-fetch |
| `REMOTR_CA_PEM` | (unset) | Inline CA PEM |
| `REMOTR_CA_URL` | (unset) | Custom URL to download CA; default server CA download uses `${REMOTR_SERVER_URL}/v1/ca.pem` and requires `REMOTR_CA_FINGERPRINT` |
| `REMOTR_VERSION` | `latest` | GitHub release tag or version (`v1.0.0` or `1.0.0`) |
| `REMOTR_GITHUB_REPO` | `DavidHoenisch/remotr` | Release source |
| `REMOTR_BIN_DIR` | `/usr/local/bin` | Binary install path |
| `REMOTR_STATE_DIR` | `/var/lib/remotr` | Enrolled credentials |
| `REMOTR_CONFIG_DIR` | `/etc/remotr` | `ca.crt`, `agent.env` |
| `REMOTR_SYNC_INTERVAL` | `30s` | Written to `agent.env` for systemd |
| `REMOTR_YES` | (unset) | Skip confirmation prompt |
| `REMOTR_SKIP_ENROLL` | (unset) | Install binary/systemd only (agent upgrade path) |
| `REMOTR_DEFER_ENROLL` | (unset) | Enroll on first boot via `remotr-agent-enroll.service` |
| `REMOTR_ENDPOINT_ID` | (unset) | Stable endpoint identifier (hostname-based if unset) |
| `REMOTR_SKIP_SYSTEMD` | (unset) | Binary + CA only |
| `REMOTR_FORCE_ENROLL` | (unset) | Pass `--force` to `remotr-agent enroll` |
| `REMOTR_VERIFY_CHECKSUMS` | (unset) | Verify GitHub release `checksums.txt` |

After install:

| Path | Purpose |
|------|---------|
| `/usr/local/bin/remotr-agent` | Agent binary (or `$REMOTR_BIN_DIR`) |
| `/etc/remotr/ca.crt` | Remotr CA (public) |
| `/etc/remotr/agent.env` | `REMOTR_SERVER_URL`, `REMOTR_STATE_DIR`, sync interval |
| `/var/lib/remotr/` | `agent.crt`, `agent.key`, `ca.crt`, `state.json` after enroll |
| `remotr-agent.service` | Sync loop (enabled by script) |

## From GitHub Releases (manual)

Agent binaries are published alongside the operator CLI on semver tags:

- Linux amd64: `remotr-agent_1.0.0_linux_amd64.tar.gz`
- Linux arm64: `remotr-agent_1.0.0_linux_arm64.tar.gz`

```bash
sudo install -m 0644 ca.crt /etc/remotr/ca.crt
tar -xzf remotr-agent_1.0.0_linux_amd64.tar.gz
sudo install -m 0755 remotr-agent /usr/local/bin/
sudo REMOTR_SERVER_URL=https://remotr.example:8443 \
  REMOTR_ENROLL_TOKEN='...' \
  remotr-agent enroll --ca /etc/remotr/ca.crt --no-sync
```

See [Installing the CLI](installing-cli.md) for release and checksum workflow.

## Build from source

```bash
go build -mod=vendor -o remotr-agent ./cmd/remotr-agent
```

Enroll with a CA from the server or your PKI ceremony — same as [Agent deployment](agent-deployment.md#enrollment).

## Releasing (maintainers)

Pushing a `v*` tag runs GoReleaser (`.github/workflows/release.yml`) and publishes `remotr-agent_*_linux_*` archives. Endpoints need a tagged release before `REMOTR_VERSION=latest` works in the install script.

Local snapshot:

```bash
make release-snapshot
ls dist/remotr-agent_*
```

## Related

- [Enrollment tokens](enrollment-tokens.md) — deployment tokens; [Endpoint management](endpoint-management.md) — endpoint list
- [Agent deployment](agent-deployment.md) — systemd, sync loop, re-enrollment
- [HTTP API: GET /v1/ca.pem](../reference/http-api.md#get-v1capem)
- [Troubleshooting: install script](troubleshooting.md#agent-install-script)
- [Production deployment](production-deployment.md#10-register-the-fleet-and-enroll-a-canary)
