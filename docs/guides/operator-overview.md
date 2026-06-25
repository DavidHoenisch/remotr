# Operator overview

The `remotr` binary is the **operator CLI** (also called the Admin CLI). Operators change desired state through Git (the configuration repository). Server registry operations — bootstrap, enrollment tokens, endpoint inventory — go through the operator CLI over mTLS using **operator credentials**, not endpoint credentials.

There is no web UI in v1.

Terminal recordings in these guides use [demo mode](../reference/environment-variables.md#demo-mode-and-vhs-recordings) (`REMOTR_DEMO`): fixture data and a fictional server URL, not a live deployment.

## Credential model

| Credential | Purpose | Stored on |
|------------|---------|-----------|
| Operator bootstrap token | One-time; creates first operator | Server stdout + `REMOTR_BOOTSTRAP_FILE` |
| Operator credential | Admin API (`/v1/admin/*`) | `~/.config/remotr/` or `REMOTR_OPERATOR_STATE_DIR` |
| Enrollment token | One-time; binds endpoint to fleet | Created by operator; delivered to installer |
| Endpoint credential | Ongoing sync (`/v1/sync`) | Agent `/var/lib/remotr/` |

All TLS identities are issued by the **Remotr CA** (`REMOTR_CA_CERT` / `REMOTR_CA_KEY` on the server).

## Operator guides

| Task | Guide |
|------|-------|
| First operator credential | [Bootstrap operator](bootstrap-operator.md) |
| Enrollment and deployment tokens | [Enrollment tokens](enrollment-tokens.md) |
| Inventory, upgrades, cron reports | [Endpoint management](endpoint-management.md) |
| Git sync and release ref | [Git sync workflow](git-sync-workflow.md) |
| Roles and stamped credentials | [RBAC](rbac.md) |
| Audit events and SIEM export | [Audit logging](audit-logging.md) |
| Validate YAML, CLI globals | [Config validation](config-validation.md) |

## Related guides

- [Installing the CLI](installing-cli.md)
- [Installing the agent](installing-agent.md)
- [Agent deployment](agent-deployment.md)
- [Configuration repository](configuration-repository.md)
- [Production deployment](production-deployment.md)

## CLI layout

The operator CLI uses [urfave/cli](https://github.com/urfave/cli). Global flags apply to all subcommands:

```bash
remotr --help
remotr endpoint --help
remotr fleet agent upgrade --help
```

Common globals: `--config`, `--server-url`, `--state-dir`, `--ca`, `--fleet`. Precedence: **flags > environment > config file**.

## Environment summary

Operator credentials default to `~/.config/remotr/` (`REMOTR_OPERATOR_STATE_DIR` or `--state-dir`).

Server-side Postgres is required for bootstrap, enrollment tokens, drift telemetry, agent upgrade taints, audit logging, and dynamic release ref. See [Environment variables](../reference/environment-variables.md).
