# Endpoint management

List, inspect, upgrade, and remove enrolled endpoints from the operator CLI.

## List and inspect endpoints

Human-readable list:

```bash
remotr endpoint list --server-url https://remotr.example:8443
```

![remotr endpoint list](../assets/demo/endpoint-list.gif)

JSON for scripts:

```bash
remotr endpoint list --server-url https://remotr.example:8443 --json
```

Show one endpoint (labels, drift, apply failures, agent upgrade status):

```bash
remotr endpoint show <endpoint-id>
remotr endpoint show <endpoint-id> --json
```

![remotr endpoint show](../assets/demo/endpoint-show.gif)

Endpoint id may appear before flags (`remotr endpoint show phalanx --server-url ...`).

## Request in-band agent upgrades

Taint endpoints so the next sync delivers an `agentUpgrade` instruction (see [Agent deployment](agent-deployment.md#agent-upgrades)):

```bash
# All endpoints in a fleet
remotr fleet agent upgrade --fleet engineering --version v0.1.15

# Single endpoint
remotr endpoint agent upgrade <endpoint-id> --version v0.1.15
```

Monitor with `remotr endpoint show <id>`. Agents must run v0.1.15+ for reliable self-upgrade.

## Remove a decommissioned endpoint

Unregister an endpoint from the server (stops accepting its mTLS identity). This does not uninstall the agent on the host — stop `remotr-agent.service` there separately.

```bash
remotr endpoint remove --server-url https://remotr.example:8443 <endpoint-id> --confirm <endpoint-id>
```

After removal, sync from that machine fails with **unknown endpoint** until it is re-enrolled. Remove any `endpoints/<id>/manifest.yaml` (and composed `desired.yaml`) override from the configuration repository in a normal Git change.

## Diagnose setup

```bash
remotr doctor
remotr doctor --json --skip-network
```

Checks operator config, credentials, CA path, configuration repository context, and server reachability.

![remotr doctor](../assets/demo/doctor.gif)

## Labels

Endpoints report **labels** in the sync request body (for example `distro=Debian`, `arch=x86`, `site=berlin`). Labels appear in admin queries; v1 does **not** use labels to select configuration paths. Assignment is fleet enrollment plus optional `endpoints/<id>/manifest.yaml` (composed to `desired.yaml`) override only.

The server uses `distro` and `arch` labels to decide which cron jobs apply to an endpoint.

## Cron job status

Inspect server-managed scheduled jobs (from `fleets/<fleet>/crons.yaml` or endpoint overrides):

```bash
remotr endpoint cron report <endpoint-id>
remotr endpoint cron report <endpoint-id> --json

remotr fleet cron report --fleet engineering
remotr fleet cron report --fleet engineering --verbose --json
```

Output includes each job's schedule, whether it applies to the endpoint's distro/arch, and the last run status (`success`, `failed`, `running`, or `never`).

Exit code `1` when any applicable job last failed (useful in CI smoke checks).

Author crons in Git; see [Crons format reference](../reference/crons-format.md) and [Configuration repository — fleet crons](configuration-repository.md#fleet-crons).

After upgrading the server, apply migration `007_cron_executions.sql` (`make migrate` or `make migrate-compose`) before cron scheduling is active.
