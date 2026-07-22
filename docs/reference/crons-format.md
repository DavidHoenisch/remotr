# Crons format reference

**Cron sources** (`kind: crons`) define **time-driven jobs** the server schedules and agents execute on check-in. Crons complement desired state: desired state converges on drift; crons run on a schedule regardless of drift.

The server composes cron artifacts from manifest `crons:` references, evaluates schedules, and tracks execution history in Postgres. Agents never write system crontab entries.

## Repository paths

| Path | `kind` | Purpose |
|------|--------|---------|
| `crons/<path>.yaml` | `crons` | Cron job list (inline or `use:` templates) |
| `fleets/<fleet>/manifest.yaml` | `manifest` | Lists cron source paths in `crons:` |
| `endpoints/<id>/manifest.yaml` | `manifest` | Optional `crons:` override (replaces fleet, no merge) |
| `crons/builtin/<name>.yaml` | — | Optional shared templates in Git (`use: crons/...`) |

Crons are optional. Fleets without scheduled jobs omit the `crons:` field.

Reference cron files from the fleet manifest:

```yaml
kind: manifest
modules:
  - modules/base-packages.yaml
crons:
  - crons/modules/weekly-upgrade.yaml
```

Validate with `remotr config validate` from the configuration repository root.

## Cron source file (`kind: crons`)

```yaml
kind: crons
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

Cron jobs currently use grouped resource collections such as `commands:` and
`packages:`. They are not schema-1 module documents and do not accept a
top-level `resources:` list. Resource addresses use
`<cron-job-name>/<resource-name>`.

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes* | Unique job name within the composed artifact (*optional when using `use:` only) |
| `description` | no | Human-readable text |
| `schedule` | yes** | Standard 5-field cron: `minute hour dom month dow` |
| `timezone` | no | IANA timezone (default `UTC`) |
| `targetDistros` | no | Same exact values as desired state (`Debian`, `Ubuntu`, `Arch`, `PopOS`) |
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
kind: crons
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

Overrides merge onto the template: `schedule`, `timezone`, `targetDistros`, `targetArch`, and any resource stanzas replace template fields when set. Builtin `use:` references are resolved by the server at sync time after composition.

## Repository templates

Share org-specific jobs under `crons/` in the configuration repository:

```yaml
kind: crons
crons:
  - use: crons/builtin/nightly-backup
    schedule: "0 3 * * *"
```

Path is relative to the repository root; `.yaml` is appended automatically if omitted.

## Custom jobs

Define resources inline without `use:`:

```yaml
kind: crons
crons:
  - name: rotate-logs
    schedule: "0 4 * * 0"
    commands:
      - name: vacuum-journal
        apply: [journalctl, --vacuum-time=30d]
```

Resource metadata (`dependsOn`, `preApplyValidation`) follows the same rules as desired state. Addresses use `cron-name/resource-name`.

`commands` are apply-only when dispatched: server crons are imperative
scheduled work, not drift-checked desired state. Make every job safe to retry
after an uncertain client/server acknowledgement.

## Server scheduling behavior

On each agent sync:

1. Server loads the composed crons artifact (endpoint override → fleet fallback) and resolves `use:` templates.
2. Server filters jobs by endpoint labels (`distro`, `arch` reported at sync).
3. For each applicable job, server compares the schedule to the last run in Postgres.
4. If a slot was missed while the endpoint was offline, **one** run is dispatched on the next check-in (no catch-up storm).
5. Server returns `dueCrons[]` in the sync response; agent executes apply-only (no drift check).
6. Agent reports `cronResults[]` on the following sync.

Crons are returned even when the desired artifact is unchanged.

## Server cron or endpoint schedule?

| Need | Use |
| --- | --- |
| Run only after the server decides a slot is due | `kind: crons` |
| Run after an offline endpoint checks in, with at most one missed dispatch | `kind: crons` |
| Run from the operating system while the endpoint cannot reach Remotr | `kind: endpointSchedule` in a schema-1 module |
| Continuously verify that the native schedule remains installed | `endpointSchedule` |

See [Endpoint schedule resources](configuration-format.md#endpoint-schedule-resources)
for cron and systemd-timer backends.

## Related docs

- [Manifest format reference](manifest-format.md)
- [Configuration repository guide](../guides/configuration-repository.md)
- [HTTP API — sync and cron reports](http-api.md#post-v1sync)
- [Endpoint management — cron reports](../guides/endpoint-management.md#cron-job-status)
- [Architecture — crons](../explanation/architecture.md#server-managed-crons)
