# Operator overview

The `remotr` binary remains the **operator CLI** (also called the Admin CLI) for
interactive terminal work, automation, recovery, and headless environments.
Operators change desired state through Git (the configuration repository).
Server registry operations—bootstrap, enrollment tokens, endpoint inventory—go
through the existing Admin API over mTLS using **operator credentials**, not
endpoint credentials.

Remotr Desktop is an additive native Linux application that uses the same Admin
API and credential layout through a narrow Go application-service boundary. It
does not invoke the operator CLI, host a web service, expose credentials to its
embedded frontend, or replace Git as the desired-state deployment path. There
is no browser-hosted Admin UI.

Desktop availability is release-evidence gated. A Linux architecture or package
format is not supported or advertised until its exact native artifact passes
build, launch, install, and removal checks. The operator CLI remains installed,
supported, and available as the fallback and recovery surface whether or not
the desktop artifact is published.

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
| High-risk request review | [Change control](change-control.md) |
| Encrypted secret versions | [Secret management](secret-management.md) |
| Roles and stamped credentials | [RBAC](rbac.md) |
| Audit events and SIEM export | [Audit logging](audit-logging.md) |
| Validate and preview YAML | [Config validation](config-validation.md) |
| Import Hub snippets into modules | [CLI reference — Hub snippets](../reference/cli.md#hub-snippets) |
| Every CLI command and flag | [Operator CLI reference](../reference/cli.md) |

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

Global flags may appear before or after the subcommand. Prefer config files or
environment variables in automation so endpoint IDs and other positional
arguments remain unambiguous.

## Environment summary

Operator credentials default to `~/.config/remotr/` (`REMOTR_OPERATOR_STATE_DIR` or `--state-dir`).

Server-side Postgres is required for bootstrap, enrollment tokens, drift telemetry, agent upgrade taints, audit logging, and dynamic release ref. See [Environment variables](../reference/environment-variables.md).
