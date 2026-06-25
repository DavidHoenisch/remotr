# Role-based access control

When Postgres is enabled, operator API access is governed by RBAC roles in addition to mTLS.

## Built-in roles

| Role | Typical use |
|------|-------------|
| `global_admin` | First bootstrap operator; full administrative access |
| `read_only` | Monitoring dashboards, inventory review |
| `security_logger` | SIEM collectors and audit review automation |

The bootstrap operator automatically receives `global_admin`.

## Issue a read-only operator

```bash
remotr admin credential stamp \
  --label monitoring \
  --role read_only \
  --out ./monitoring-creds
```

## Use stamped credentials on a new computer

`remotr admin credential stamp` writes a portable credential bundle you can move to another machine. This is how you give a teammate, laptop, or automation host access without sharing your personal bootstrap credential.

### What the admin creates

On a machine that already has a working operator session:

```bash
remotr admin credential stamp \
  --label alice-laptop \
  --role global_admin \
  --out ./alice-creds
```

The output directory contains:

| File | Purpose |
|------|---------|
| `cert.pem` | Operator client certificate |
| `key.pem` | Operator private key |
| `ca.pem` | Remotr CA (trust anchor) |
| `state.json` | Operator UUID (`operator_id`) |

Copy the entire directory to the new computer over a secure channel (SSH `scp`, encrypted USB, secrets manager). Treat `key.pem` like a password.

### What the recipient installs

1. Install the `remotr` CLI ([Installing the CLI](installing-cli.md)).
2. Place credentials where the CLI expects them. The CLI looks for these names under `~/.config/remotr/` by default:

| Stamped file | CLI filename |
|--------------|--------------|
| `cert.pem` | `operator.crt` |
| `key.pem` | `operator.key` |
| `ca.pem` | `ca.crt` |
| `state.json` | `state.json` (unchanged) |

Example install script on the new computer:

```bash
#!/usr/bin/env bash
set -euo pipefail

STAMPED_DIR="${1:?path to stamped creds directory}"
STATE_DIR="${2:-$HOME/.config/remotr}"

mkdir -p "$STATE_DIR"
chmod 700 "$STATE_DIR"
install -m 600 "$STAMPED_DIR/cert.pem" "$STATE_DIR/operator.crt"
install -m 600 "$STAMPED_DIR/key.pem" "$STATE_DIR/operator.key"
install -m 600 "$STAMPED_DIR/ca.pem" "$STATE_DIR/ca.crt"
install -m 600 "$STAMPED_DIR/state.json" "$STATE_DIR/state.json"
```

3. Point the CLI at your server:

```bash
remotr config init \
  --server-url https://remotr.example:8443 \
  --state-dir ~/.config/remotr \
  --fleet default
```

`config init` writes `~/.config/remotr/config.yaml`. The `ca` field defaults to `ca.crt` inside `state_dir`, so you usually do not need a separate `--ca` flag.

4. Verify access:

```bash
remotr endpoint list
remotr rbac role-list    # requires global_admin (or another role with permission)
```

### Scoped credentials (read-only, SIEM, etc.)

Stamp with only the roles the new machine needs:

```bash
# Monitoring laptop — inventory only
remotr admin credential stamp --label monitoring --role read_only --out ./monitoring-creds

# SIEM collector — audit export only
remotr admin credential stamp --label siem-collector --role security_logger --out ./siem-creds
```

On the new host, follow the same install steps above. A `read_only` credential can list endpoints but cannot delete them or create tokens.

### Environment variables instead of config file

You can skip `config.yaml` and pass settings per command or via env:

```bash
export REMOTR_SERVER_URL=https://remotr.example:8443
export REMOTR_OPERATOR_STATE_DIR=~/.config/remotr
remotr endpoint list
```

### Revoking access

Remotr does not currently expose operator cert revocation from the CLI. To remove access, delete the operator's role assignments in Postgres or rotate the Remotr CA and re-issue credentials (see [CA rotation runbook](../runbooks/ca-rotation.md)).

## Manage roles and assignments

```bash
remotr rbac role-list
remotr rbac role-create fleet_observer --description "Endpoint inventory only"
remotr rbac rule-add fleet_observer --method GET --path /v1/admin/endpoints/*
remotr rbac operator-list
remotr rbac operator-set-roles <operator-id> --role read_only --role fleet_observer
```

Custom roles make it straightforward to add new access patterns without pulling in an external authorization library.

## Related

- [HTTP API — RBAC](../reference/http-api.md#role-based-access-control-rbac)
- [Audit logging](audit-logging.md) — `security_logger` role for SIEM export
