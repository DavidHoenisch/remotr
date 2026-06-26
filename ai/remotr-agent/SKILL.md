---
name: remotr-agent
description: >-
  Operate Remotr Linux MDM fleets using the remotr operator CLI: bootstrap,
  enrollment, endpoint inventory, GitOps config repos, compliance state reports,
  cron status, agent upgrades, and git sync. Use when managing enrolled Linux
  endpoints, fleet desired state, drift, enrollment tokens, deployment tokens,
  or when the user mentions remotr, MDM, fleet compliance, or endpoint labels.
disable-model-invocation: false
---

# Remotr operator CLI (AI skill)

Remotr is pull-based Linux MDM: operators edit desired state in a **Git configuration repository**; **remotr-server** serves config; **remotr-agent** on each machine syncs and applies.

This skill covers the **operator CLI** (`remotr`) — not the agent daemon. You help admins manage devices **through** the CLI and Git, not by SSHing to change packages directly (unless troubleshooting).

## Before you run commands

1. Confirm operator credentials exist: `remotr doctor` (or `remotr config show`).
2. Prefer config file `~/.config/remotr/config.yaml` over repeating flags. Precedence: **flags > environment > config file**.
3. Read bundled references in this skill directory:
   - `reference/commands.md` — full command table
   - `reference/workflows.md` — bootstrap, enroll, drift, decommission flows
4. Helper script: `scripts/fleet-summary.sh <fleet>` — list endpoints plus state and cron reports.

## Core workflows

### Diagnose setup

```bash
remotr doctor
remotr endpoint list
```

### Create enrollment capacity

```bash
remotr enroll token create --fleet engineering --ttl 168h
# or reusable:
remotr deployment create --label prod-2026 --fleet production --ttl 8760h --out /secure/deploy.token --quiet
```

Never echo tokens into chat logs or commit them to git.

### Inspect fleet health

```bash
remotr endpoint list
remotr endpoint show <endpoint-id>
remotr fleet state report --fleet engineering
remotr fleet cron report --fleet engineering
```

Exit code **4** from state report means drift was found — report it to the user, do not treat as CLI failure.

### Apply GitOps changes

```bash
remotr git sync
remotr fleet state report --fleet engineering
```

Validate and compose config repo changes locally when possible:

```bash
remotr config compose .
remotr config validate
```

Modular repos edit `modules/` and `manifest.yaml`, then compose before commit. See bundled `reference/workflows.md`.

### Destructive actions

Require explicit user intent. Use matching `--confirm`:

```bash
remotr endpoint remove <endpoint-id> --confirm <endpoint-id>
remotr deployment revoke <label> --confirm <label>
```

In an interactive terminal, `--confirm` may be omitted and the CLI will prompt.

### Agent upgrades

```bash
remotr fleet agent upgrade --fleet engineering --version v0.2.2
remotr endpoint agent upgrade <endpoint-id> --version v0.2.2
```

Upgrades apply on the endpoint's next sync.

## AI agent rules

- **Do not** invent server URLs, fleet names, or endpoint IDs — list them first.
- **Do not** store or repeat secrets (bootstrap, enroll, deployment tokens).
- Use `--json` when parsing output programmatically.
- Use `--quiet` with `--out` when writing tokens to files.
- For scripts, set `REMOTR_CONFIG` or use global flags consistently (flags work after subcommands).
- Configuration repository edits are normal git commits; registry ops use `remotr`.
- If TLS fails, check `ca` in operator config — Remotr uses a private CA, not public CAs.

## Updating this skill

Installed by `remotr ai setup --agent claude`, `cursor`, or `pi`. Refresh from the repo:

```bash
remotr ai upgrade --agent claude
remotr ai upgrade --agent pi
```

Bundled version: see `VERSION` in this directory.
