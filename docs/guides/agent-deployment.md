# Agent deployment

Each Linux endpoint runs `remotr-agent` as a long-lived service. The agent enrolls once with an **enrollment or deployment token**, stores **endpoint credentials** under `/var/lib/remotr/`, then polls the server over **mTLS** on a fixed interval.

**Installing on endpoints:** see [Installing the agent](installing-agent.md) for the paste-and-run install script (server URL + token only; CA from `GET /v1/ca.pem`).

Agents do not listen for inbound connections. All traffic is outbound HTTPS to `remotr-server`.

## Requirements

- Linux with systemd (recommended)
- Root for apply operations (packages, `/etc`, `systemctl`, command resources)
- Outbound HTTPS to the Remotr server
- Enrollment or deployment token from an operator (see [Installing the agent](installing-agent.md))

Supported distros for in-document targeting: **Debian**, **Ubuntu**, **Arch**. Package managers: `apt`, `pacman`, `yay`, `dnf`.

## Install the binary manually

Build or copy `remotr-agent` to the endpoint:

```bash
go build -mod=vendor -o /usr/local/bin/remotr-agent ./cmd/remotr-agent
```

Install the Remotr CA for enrollment trust (public cert — fetch from the server or copy from your operator workstation):

```bash
install -d -m 0755 /etc/remotr
curl -kfsSL https://remotr.example:8443/v1/ca.pem -o /etc/remotr/ca.crt
# or: install -m 0644 ca.crt /etc/remotr/ca.crt
```

## Enrollment

Enrollment exchanges the token for a client certificate. **CSR mode is default**: the agent generates a private key locally and sends a CSR to `POST /v1/enroll`. The private key never leaves the machine.

```bash
remotr-agent enroll \
  --server-url https://remotr.example:8443 \
  --ca /etc/remotr/ca.crt \
  --token "$ENROLLMENT_TOKEN" \
  --state-dir /var/lib/remotr
```

After success:

```text
/var/lib/remotr/
  agent.crt    # client certificate
  agent.key    # private key (mode 0600)
  ca.crt       # Remotr CA
  state.json   # {"endpoint_id":"..."}
```

Flags:

| Flag | Description |
|------|-------------|
| `--force` | Replace existing credentials (re-enrollment) |
| `--no-sync` | Store credentials only; exit without starting sync loop |
| `--server-key` | Legacy: server generates key pair (avoid in production) |
| `--sync-interval` | Override interval after enroll (default from env or 30s) |

Environment alternatives:

| Variable | Purpose |
|----------|---------|
| `REMOTR_ENROLL_TOKEN` | Token string |
| `REMOTR_ENROLL_TOKEN_FILE` | Path to token file (absolute path; common in Compose/systemd) |
| `REMOTR_SERVER_URL` | Server base URL |
| `REMOTR_TLS_CA` | CA for server trust during enroll |
| `REMOTR_STATE_DIR` | Credential directory (default `/var/lib/remotr`) |

## Sync loop

With credentials present, running `remotr-agent` with no subcommand starts the sync loop:

```bash
REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_STATE_DIR=/var/lib/remotr \
REMOTR_SYNC_INTERVAL=30s \
remotr-agent
```

Each sync:

1. POST `/v1/sync` over mTLS (identity from client cert, not request body)
2. If the server returns `agentUpgrade`, download and install that release, then restart `remotr-agent.service` (v0.1.15+ uses staging + rename to avoid “text file busy” on Linux)
3. Compare artifact digest; skip YAML download if unchanged (upgrade instructions are still delivered when tainted)
4. Resolve in-document targeting (`targetDistros`, `targetArch`) using local OS facts
5. Run **Check** across all applicable resources
6. Apply per fleet **remediation policy** (`auto` or `report`)
7. Report labels, drift, apply failures, and agent version / upgrade status in the sync request body

Legacy file-based TLS (without enrolled credentials) is still supported via `REMOTR_TLS_CERT`, `REMOTR_TLS_KEY`, and `REMOTR_TLS_CA` for migration scenarios.

## systemd unit example

```ini
[Unit]
Description=Remotr endpoint agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=REMOTR_SERVER_URL=https://remotr.example:8443
Environment=REMOTR_STATE_DIR=/var/lib/remotr
Environment=REMOTR_SYNC_INTERVAL=30s
Environment=REMOTR_TLS_CA=/etc/remotr/ca.crt
ExecStart=/usr/local/bin/remotr-agent
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Boot-time enrollment (first power-on)

`scripts/install-agent.sh` installs `remotr-agent-enroll.service` (oneshot) and `remotr-agent.service` (sync). For **deferred enrollment on first boot**, set `REMOTR_DEFER_ENROLL=1` with a deployment or enrollment token; the script writes `/etc/remotr/enroll.env` (mode `0600`) and enables both units. The enroll unit waits for `GET /healthz` before calling `remotr-agent enroll --no-sync`.

```bash
REMOTR_YES=1 \
REMOTR_DEFER_ENROLL=1 \
REMOTR_SERVER_URL=https://remotr.example:8443 \
REMOTR_DEPLOYMENT_TOKEN='...' \
bash <(curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh)
```

**Golden image + cloud-init:** bake the agent with `REMOTR_SKIP_ENROLL=1` (binary and systemd only). On first boot, cloud-init writes `/etc/remotr/enroll.env` and runs `systemctl enable --now remotr-agent-enroll.service`. Use a **deployment token** (reusable) in the enroll env file.

**Immediate install (default):** omit `REMOTR_DEFER_ENROLL`; enrollment runs during the install script, then the sync service starts.

Put secrets in `/etc/remotr/enroll.env` with `REMOTR_ENROLL_TOKEN=...` or `REMOTR_ENROLL_TOKEN_FILE=...`. Optional: `REMOTR_ENDPOINT_ID=...` for a stable endpoint name. Remove or rotate the token file after successful enroll.

## Agent upgrades

### In-band (operator taint)

Operators can request an agent version from the server. On the next sync (even when the fleet artifact digest is unchanged), the server returns an `agentUpgrade` instruction; the agent downloads the release tarball from GitHub, reinstalls the binary, and restarts `remotr-agent.service`.

```bash
# One endpoint
remotr --server-url https://remotr.example:8443 \
  endpoint agent upgrade <endpoint-id> --version v0.1.15

# Whole fleet
remotr --server-url https://remotr.example:8443 \
  fleet agent upgrade --fleet engineering --version v0.1.15
```

Global flags (`--server-url`, `--config`, `--state-dir`, and others) may appear before the subcommand. If `~/.config/remotr/config.yaml` is set up, omit repeated flags.

The server clears the taint when the agent reports a matching version with phase `completed`. Check progress with `remotr endpoint show <id>` (JSON fields `desired_agent_version`, `reported_agent_version`, `agent_upgrade`).

**Requirements:**

- Server v0.1.13+ with migration `003_agent_upgrade.sql` applied on Postgres
- Target agents on **v0.1.15+** for reliable in-band self-upgrade (v0.1.13–v0.1.14 could fail with `text file busy` while replacing the running binary)
- GitHub release assets must exist for the requested tag (`remotr-agent_<version>_linux_<arch>.tar.gz`)

Override install path on the endpoint with `REMOTR_BIN_DIR` (default `/usr/local/bin`) if the binary is not in the default location.

### Manual (install script)

Re-run the install script on each machine (keeps enrollment and systemd layout):

```bash
REMOTR_YES=1 \
REMOTR_SKIP_ENROLL=1 \
REMOTR_VERSION=v1.2.0 \
REMOTR_SERVER_URL=https://remotr.example:8443 \
bash <(curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh)
```

`REMOTR_SKIP_ENROLL=1` replaces `/usr/local/bin/remotr-agent` and restarts `remotr-agent.service` without touching `/var/lib/remotr/`.

Pin `REMOTR_VERSION` in production; avoid `latest` without `jq`. After upgrading, confirm sync: `systemctl status remotr-agent` and `remotr endpoint show <id>`.

## Endpoint overrides

If a machine needs configuration different from its fleet, add `endpoints/<endpoint-id>/desired.yaml` in the configuration repository. The endpoint ID is in `/var/lib/remotr/state.json` after enrollment.

The override **replaces** the fleet artifact for that endpoint — it does not merge with the fleet file.

## Remove from the server

Operators unregister endpoints with the admin CLI (does not touch files on the endpoint):

```bash
remotr endpoint remove --server-url https://remotr.example:8443 <endpoint-id>
```

On the machine, disable the agent and optionally delete `/var/lib/remotr/`.

## Re-enrollment

When rotating endpoint certificates or moving a machine to another fleet:

1. Create a new enrollment token for the target fleet.
2. Run `remotr-agent enroll --force ...` with the new token.
3. Confirm sync and inventory with `remotr endpoint show`.

See [CA rotation](../runbooks/ca-rotation.md) for CA-wide rotation.

## Docker Compose reference

The dev stack in `compose/docker-compose.yml` models production enrollment:

- Agents wait for server health
- Enroll if `state.json` is missing
- Run sync loop with enrolled credentials
- No pre-baked agent certificates

Use it as a reference for entrypoint scripting and health checks.

## Troubleshooting

See [Troubleshooting](troubleshooting.md) for enrollment failures, sync errors, and permission issues.
