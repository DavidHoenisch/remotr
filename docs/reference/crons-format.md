# Crons format reference

**Crons artifacts** are YAML files separate from `desired.yaml`. They define **time-driven jobs** the server schedules and agents execute on check-in. Crons complement desired state: desired state converges on drift; crons run on a schedule regardless of drift.

The server evaluates schedules and tracks execution history in Postgres. Agents never write system crontab entries.

## Repository paths

| Path | Purpose |
|------|---------|
| `fleets/<fleet>/crons.yaml` | Fleet baseline crons |
| `endpoints/<endpoint-id>/crons.yaml` | Optional override (replaces fleet file, no merge) |
| `crons/builtin/<name>.yaml` | Optional shared templates in Git (referenced with `use: crons/...`) |

`crons.yaml` is optional. Fleets without scheduled jobs omit the file.

Validate with `remotr config validate` from the configuration repository root.

## Top-level structure

```yaml
crons:
  - name: weekly-system-upgrade
    description: optional human text
    schedule: "0 0 * * 0"
    timezone: UTC
    targetDistros: [Debian, Ubuntu]
    targetArch: [x86]
    commands:
      - name: apt-update
        apply: [apt-get, update, -y]
      - name: apt-upgrade
        apply: [apt-get, upgrade, -y, -q]
        dependsOn: [weekly-system-upgrade/apt-update]
```

Each list entry is one **cron job** with a schedule and the same resource stanzas as [configuration format](configuration-format.md) (`commands`, `packages`, `files`, and so on).

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes* | Unique job name within the file (*optional when using `use:` only) |
| `description` | no | Human-readable text |
| `schedule` | yes** | Standard 5-field cron: `minute hour dom month dow` |
| `timezone` | no | IANA timezone (default `UTC`) |
| `targetDistros` | no | Same values as desired state (`Debian`, `Ubuntu`, `Arch`) |
| `targetArch` | no | `x86` or `ARM` |
| `use` | no | Reference a builtin or repo template (see below) |

\* When `use:` is set, names come from the template unless overridden.  
\*\* May be omitted when inherited from a template; an override schedule is common.

### Schedule syntax

Five fields, space-separated:

```text
minute hour day-of-month month day-of-week
```

Examples:

| Expression | Meaning |
|------------|---------|
| `0 0 * * 0` | Every Sunday at 00:00 |
| `0 2 * * *` | Every day at 02:00 |
| `*/15 * * * *` | Every 15 minutes |
| `0 9-17 * * 1-5` | Hourly 09:00–17:00, Monday–Friday |

Day-of-week uses `0` or `7` for Sunday.

## Builtin templates

Reference embedded library jobs with `use: builtin/<name>`:

```yaml
crons:
  - use: builtin/system-upgrade
    schedule: "0 0 * * 0"
```

| Template | Description |
|----------|-------------|
| `builtin/system-upgrade` | Debian/Ubuntu (apt) and Arch (pacman) upgrade jobs |
| `builtin/system-upgrade-debian` | Debian/Ubuntu only |
| `builtin/system-upgrade-arch` | Arch only |
| `builtin/clamav-scan` | Debian/Ubuntu and Arch ClamAV signature update + home directory scan |
| `builtin/clamav-scan-debian` | Debian/Ubuntu only |
| `builtin/clamav-scan-arch` | Arch only |

Overrides merge onto the template: `schedule`, `timezone`, `targetDistros`, `targetArch`, and any resource stanzas replace template fields when set.

## Repository templates

Share org-specific jobs under `crons/` in the configuration repository:

```yaml
crons:
  - use: crons/builtin/nightly-backup
    schedule: "0 3 * * *"
```

Path is relative to the repository root; `.yaml` is appended automatically if omitted.

## Custom jobs

Define resources inline without `use:`:

```yaml
crons:
  - name: rotate-logs
    schedule: "0 4 * * 0"
    commands:
      - name: vacuum-journal
        apply: [journalctl, --vacuum-time=30d]
```

Resource metadata (`dependsOn`, `preApplyValidation`) follows the same rules as desired state. Addresses use `cron-name/resource-name`.

## Server scheduling behavior

On each agent sync:

1. Server loads and resolves `crons.yaml` (fleet → endpoint override).
2. Server filters jobs by endpoint labels (`distro`, `arch` reported at sync).
3. For each applicable job, server compares the schedule to the last run in Postgres.
4. If a slot was missed while the endpoint was offline, **one** run is dispatched on the next check-in (no catch-up storm).
5. Server returns `dueCrons[]` in the sync response; agent executes apply-only (no drift check).
6. Agent reports `cronResults[]` on the following sync.

Crons are returned even when `desired.yaml` is unchanged.

## Related docs

- [Configuration repository guide](../guides/configuration-repository.md)
- [HTTP API — sync and cron reports](http-api.md#post-v1sync)
- [Endpoint management — cron reports](../guides/endpoint-management.md#cron-job-status)
- [Architecture — crons](../explanation/architecture.md#server-managed-crons)
