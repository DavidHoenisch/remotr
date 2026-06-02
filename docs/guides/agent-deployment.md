# Agent deployment

Each Linux endpoint runs `remotr-agent` as a long-lived service. The agent enrolls once with a **one-time enrollment token**, stores **endpoint credentials** under `/var/lib/remotr/`, then polls the server over **mTLS** on a fixed interval.

Agents do not listen for inbound connections. All traffic is outbound HTTPS to `remotr-server`.

## Requirements

- Linux with systemd (recommended)
- Root for apply operations (packages, `/etc`, `systemctl`, command resources)
- Outbound HTTPS to the Remotr server
- Remotr CA certificate to trust server TLS during enrollment
- One-time enrollment token from an operator

Supported distros for in-document targeting: **Debian**, **Ubuntu**, **Arch**. Package managers: `apt`, `pacman`, `yay`, `dnf`.

## Install the binary

Build or copy `remotr-agent` to the endpoint:

```bash
go build -mod=vendor -o /usr/local/bin/remotr-agent ./cmd/remotr-agent
```

Install the Remotr CA for enrollment trust:

```bash
install -d -m 0755 /etc/remotr
install -m 0644 ca.crt /etc/remotr/ca.crt
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
2. Compare artifact digest; skip download if unchanged
3. Resolve in-document targeting (`targetDistros`, `targetArch`) using local OS facts
4. Run **Check** across all applicable resources
5. Apply per fleet **remediation policy** (`auto` or `report`)
6. Report labels, drift, and apply failures in the sync request body

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

Enrollment is typically a **oneshot** before enabling the sync service:

```ini
[Unit]
Description=Remotr agent enrollment
ConditionPathExists=!/var/lib/remotr/state.json
After=network-online.target

[Service]
Type=oneshot
EnvironmentFile=-/etc/remotr/enroll.env
ExecStart=/usr/local/bin/remotr-agent enroll --no-sync
RemainAfterExit=yes
```

Put the enrollment token in `/etc/remotr/enroll.env` (mode `0600`, root-owned) with `REMOTR_ENROLL_TOKEN=...` or `REMOTR_ENROLL_TOKEN_FILE=...`, then remove or rotate after successful enroll.

## Endpoint overrides

If a machine needs configuration different from its fleet, add `endpoints/<endpoint-id>/desired.yaml` in the configuration repository. The endpoint ID is in `/var/lib/remotr/state.json` after enrollment.

The override **replaces** the fleet artifact for that endpoint — it does not merge with the fleet file.

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
