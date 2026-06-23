# remotr operator CLI — command reference for AI agents

Run `remotr <command> --help` for flags. Global flags (`--config`, `--server-url`, `--state-dir`, `--fleet`) may appear before or after the subcommand.

## Setup and diagnostics

| Command | Purpose |
|---------|---------|
| `remotr doctor` | Check config, credentials, CA, server reachability |
| `remotr doctor --json --skip-network` | Machine-readable local checks |
| `remotr bootstrap --token TOKEN --server-url URL --ca ca.crt` | One-time operator credentials |
| `remotr config show` / `config path` | Inspect operator config |
| `remotr config validate [dir]` | Validate configuration repository |
| `remotr version` | CLI version |

## Inventory

| Command | Purpose |
|---------|---------|
| `remotr endpoint list` | All enrolled endpoints |
| `remotr endpoint show <id>` | Labels, drift, agent upgrade, check-in |
| `remotr endpoint label set <id> key=value` | Set operator-managed label |
| `remotr endpoint label unset <id> key` | Remove label |
| `remotr endpoint label list <id>` | List labels on endpoint |
| `remotr endpoint remove <id> --confirm <id>` | Unregister endpoint (destructive) |
| `remotr fleet list` | Configured fleets |

## Enrollment tokens

| Command | Purpose |
|---------|---------|
| `remotr enroll token create --fleet FLEET --ttl 168h` | One-time enroll token |
| `remotr deployment create --label LABEL --fleet FLEET --ttl 8760h --out path` | Reusable deployment token |
| `remotr deployment list` / `show LABEL` / `revoke LABEL --confirm LABEL` | Deployment token lifecycle |

## Fleet operations

| Command | Purpose |
|---------|---------|
| `remotr git sync` | Trigger server config repo fetch |
| `remotr fleet state report --fleet FLEET` | Compliance summary (exit 4 on drift) |
| `remotr fleet cron report --fleet FLEET` | Cron job status across fleet |
| `remotr endpoint state report --endpoint ID` | Single-endpoint compliance |
| `remotr endpoint cron report --endpoint ID` | Single-endpoint cron status |
| `remotr fleet agent upgrade --fleet FLEET --version vX.Y.Z` | Taint fleet for agent upgrade |
| `remotr endpoint agent upgrade --endpoint ID --version vX.Y.Z` | Taint one endpoint |

## GitOps scaffold

| Command | Purpose |
|---------|---------|
| `remotr init -fleet FLEET ./config-repo` | Scaffold configuration repository |

## Output

| Flag | Effect |
|------|--------|
| `--json` | JSON stdout |
| `--format table\|plain\|json` | List formatting |
| `--quiet` | Suppress secrets on stdout |
| `--verbose` | Detailed errors |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | API/runtime error |
| 2 | Usage or configuration error |
| 4 | Compliance drift detected (state report commands) |
