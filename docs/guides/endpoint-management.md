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

## Export asset inventory

`endpoint list` is registry identity; `inventory` joins every endpoint with its
latest reported system-information snapshot:

```bash
remotr inventory
remotr inventory --json
remotr inventory --json --save
```

Inventory includes OS, CPU, RAM, kernel, primary IP/MAC, block-device
encryption summary, TPM, agent version, and last check-in when reported. A
blank field means no usable snapshot was available; it does not prove the
feature is absent.

`--save` writes a timestamped file in the current directory with mode `0644`.
Because inventory contains network and hardware metadata, run it from an
appropriately protected directory or redirect JSON to a protected store.

## Review compliance state

Latest report for one endpoint:

```bash
remotr endpoint state report <endpoint-id>
remotr endpoint state report <endpoint-id> --json
```

Fleet summary and full evidence:

```bash
remotr fleet state report engineering
remotr fleet state report engineering --verbose
remotr fleet state report engineering --verbose --json
```

Interpret results separately:

- `compliant` means the provider's Check matched the requested state;
- `drifted` means Check found a supported mismatch;
- `unsupported` means the endpoint/provider cannot truthfully implement that
  combination and it was not applied as ordinary drift;
- `failed` means check, preflight, apply, activation, or verification failed;
- reboot or logout requirements are activation evidence and can remain after
  configuration itself is compliant.

State report commands exit `4` for compliance drift. Runtime/API failures exit
`1`; do not collapse both into a single “non-zero” CI result. Use JSON and
inspect resource addresses, provider, reason code, release ref, artifact
digest, and last check-in before deciding remediation.

An empty report immediately after enrollment normally means the first sync has
not completed. Check agent logs and wait one configured sync interval.

### Distinguish artifact sync from compliance

The agent log message `sync unchanged` means the server returned the same
release ref and artifact digest that the agent already holds. It confirms that
no new artifact needs to be downloaded; it does not mean the endpoint is
compliant. The agent continues to check the current artifact and can report
drift while sync remains unchanged.

For example, firewall resources default to `audit: true`. Their Check result is
`drifted` with reason code `audit_plan` because the provider reports the change
it would make without enforcing it. That expected, persistent drift makes the
whole endpoint report `in_compliance: false`. See
[Firewall resources](../reference/configuration-format.md#firewall-resources).

The fleet remediation policy is a separate control. `report` skips Apply for
drifted resources; `auto` permits Apply, but an audit-only firewall Apply still
does not enforce the rule or become compliant. The authoritative fleet policy
is stored in the server registry, not in the configuration repository's
`remotr.yaml`; see
[`kind: remotr-config-repo`](../reference/repository-kinds.md#kind-remotr-config-repo).

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

After removal, sync from that machine fails with **unknown endpoint** until it is re-enrolled. Remove any `endpoints/<id>/manifest.yaml` override from the configuration repository in a normal Git change.

## Diagnose setup

```bash
remotr doctor
remotr doctor --json --skip-network
```

Checks operator config, credentials, CA path, configuration repository context, and server reachability.

![remotr doctor](../assets/demo/doctor.gif)

## Labels

Endpoints report **labels** in the sync request body (for example `distro=Debian`, `arch=x86`, `site=berlin`). Labels appear in admin queries; v1 does **not** use labels to select configuration paths. Assignment is fleet enrollment plus optional `endpoints/<id>/manifest.yaml` override only.

The server uses `distro` and `arch` labels to decide which cron jobs apply to an endpoint.

Manage operator-owned labels:

```bash
remotr endpoint label set <endpoint-id> site=berlin
remotr endpoint label set --endpoint <endpoint-id> --key owner --value platform
remotr endpoint label list <endpoint-id> --json
remotr endpoint label unset <endpoint-id> owner
```

Agent sync can overwrite labels it also reports. Reserve names such as
`ops.example.com/site` for operators and do not manually edit fact keys such as
`distro` or `arch`.

## Cron job status

Inspect server-managed scheduled jobs (from composed fleet crons or endpoint overrides):

```bash
remotr endpoint cron report <endpoint-id>
remotr endpoint cron report <endpoint-id> --json

remotr fleet cron report --fleet engineering
remotr fleet cron report --fleet engineering --verbose --json
```

Output includes each job's schedule, whether it applies to the endpoint's distro/arch, and the last run status (`success`, `failed`, `running`, or `never`).

Treat a failed applicable job as an operational failure and inspect JSON rather
than assuming the desired-state report covers scheduled execution.

Author crons in Git; see [Crons format reference](../reference/crons-format.md)
and [Configuration repository — scheduling](configuration-repository.md#choose-the-right-scheduling-model).

`kind: crons` is server-dispatched at check-in. Native
`endpointSchedule` resources have separate configuration compliance and, where
available, runtime history. Choose the model explicitly; see [Crons or endpoint
schedule](../reference/crons-format.md#server-cron-or-endpoint-schedule).

After upgrading the server, apply migration `007_cron_executions.sql` (`make migrate` or `make migrate-compose`) before cron scheduling is active.

## Firewall inspection

Inspect live firewall rules and audit logs for an endpoint:

```bash
remotr firewall report <endpoint-id>
remotr firewall report <endpoint-id> --json
```

Shows the detected backend (`firewalld` or `nftables`) and current rules. For firewalld, zone targets, services, ports, and sources are listed. For nftables, the raw ruleset is shown.

Review the audit log (rules processed in audit or enforced mode):

```bash
remotr firewall logs <endpoint-id>
remotr firewall logs <endpoint-id> --json
```

Export rules for compliance review or offline analysis:

```bash
remotr firewall export <endpoint-id> --output rules.csv
remotr firewall export --fleet engineering --output fleet-rules.csv
```

Firewall report and audit evidence are observations, not proof that the
management control path is recoverable. Before enforced firewall changes, use
the provider's guarded transaction and an out-of-band recovery path.
