# Operator CLI reference

`remotr` is the administrative CLI for operator setup, enrollment, inventory,
GitOps configuration, security workflows, and custom packages.

```text
remotr [global options] <command> [command options] [arguments]
```

Use `remotr <command> --help` for the exact flags supported by the installed
build. This page documents every public command family.

## Settings and precedence

Settings resolve in this order, highest priority first:

1. explicit flag;
2. environment variable;
3. operator config file;
4. built-in default.

Global flags may appear before or after a subcommand.

| Global flag | Environment | Config key | Meaning |
| --- | --- | --- | --- |
| `--config PATH` | `REMOTR_CONFIG` | — | Operator config path; default `~/.config/remotr/config.yaml`. |
| `--server-url URL` | `REMOTR_SERVER_URL` | `server_url` | HTTPS server base URL. |
| `--state-dir PATH` | `REMOTR_OPERATOR_STATE_DIR` | `state_dir` | Directory containing operator mTLS credentials. |
| `--ca PATH` | `REMOTR_CA` | `ca` | Remotr CA certificate. |
| `--fleet NAME` | `REMOTR_FLEET` | `fleet` | Default fleet for commands that need one. |
| `--verbose` | — | — | Include detailed API/runtime errors. |
| `--color auto\|always\|never` | `NO_COLOR` disables | — | Stderr label color. |

Operator config example:

```yaml
server_url: https://remotr.example.internal:8443
state_dir: ~/.config/remotr
ca: ~/.config/remotr/ca.pem
fleet: workstations
```

`remotr.yaml` from a configuration repository is not an operator config file.
If `REMOTR_CONFIG` points to one, the CLI reports the mismatch rather than
silently using repository metadata.

## Output and automation

Commands that support common output flags accept:

| Flag | Meaning |
| --- | --- |
| `--json` | Indented JSON; equivalent to `--format json`. |
| `--format table\|plain\|json` | Output mode; default `table`. |
| `--no-headers` | Omit table headers. |

Human progress and informational lines are written to stderr where practical;
data is written to stdout. Prefer `--json` in automation. Destructive commands
require `--confirm` with the exact resource identifier shown below.

Exit codes:

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | Runtime, connectivity, or API error. |
| `2` | Usage or configuration error. |
| `4` | Compliance drift. |

When an endpoint or fleet argument is omitted in an interactive terminal, some
commands open a selector. Non-interactive use must supply the argument or its
`--endpoint`/`--fleet` alternative.

## Setup commands

### `doctor`

```text
remotr doctor [--skip-network] [output flags]
```

Checks operator config, state directory, certificate/key material, CA, and
server reachability. `--skip-network` performs only local checks. A failed
report exits 2, including in JSON mode.

### `bootstrap`

```text
remotr bootstrap --token TOKEN
remotr bootstrap --token -
```

Exchanges the one-time server bootstrap token for operator mTLS credentials,
writes them below `--state-dir`, and saves operator settings. `--token -` reads
the token from stdin. The token is invalidated after a successful exchange.
Also supports `--quiet` and `--json`.

### `init`

```text
remotr init [DIRECTORY] [--fleet NAME] [--policy auto|report]
remotr init DIRECTORY --register-server [--database-url URL]
  [--enroll] [--enroll-ttl 168h] [--enroll-out PATH] [--quiet] [--json]
```

Creates a configuration repository only in an absent or empty destination.
`--register-server` writes fleet settings through direct Postgres access using
`--database-url` or `REMOTR_DATABASE_URL`. `--enroll` requires registration
and creates a one-time token. `--enroll-out` writes it mode `0600`.

### `version`, `docs`, and `upgrade`

| Command | Options | Purpose |
| --- | --- | --- |
| `remotr version` | — | Print build version and available commit/date metadata. Alias: `remotr v`. |
| `remotr docs` | — | Open the published docs with `xdg-open`, or print the URL. |
| `remotr upgrade` | `--check`, `--version TAG`, `--install-path PATH`, `--repo OWNER/REPO`, `--force`, output flags | Check for or install a GitHub release over the current binary. |

### AI skill installation

| Command | Options |
| --- | --- |
| `remotr ai setup` | `--agent claude\|cursor\|pi` (required), `--scope user\|project`, `--force` |
| `remotr ai upgrade` | Setup options plus `--version TAG`, `--repo OWNER/REPO`; downloads the release bundle. |
| `remotr ai list` | `--scope user\|project`, output flags |

`setup` uses the skill bundle embedded in the CLI. `upgrade` retrieves a newer
bundle independently of the CLI binary.

## Operator configuration and repository commands

### Operator settings

| Command | Options | Result |
| --- | --- | --- |
| `remotr config path` | — | Print the default operator config path. |
| `remotr config show` | `--format json\|plain` | Print resolved settings. |
| `remotr config init` | Global connection flags, `--json` | Write the operator config file mode `0600`. |

### Repository validation and composition

```text
remotr config validate [DIRECTORY] [--skip-render-check] [output flags]
remotr config discover [DIRECTORY] --fleet NAME [output flags]
remotr config render [DIRECTORY] [--fleet NAME | --endpoint ID]
  [--output PATH] [output flags]
```

- `validate` checks kinds, schemas, references, and full composition.
  `--skip-render-check` stops after source/schema checks and is weaker than the
  normal CI gate.
- `discover` lists selected manifests, modules, applications, crons, resource
  kinds, and capability requirements for one fleet.
- `render` previews all targets when no selector is supplied, or one fleet or
  endpoint. `--fleet` and `--endpoint` are mutually exclusive. JSON embeds
  content and digests. `--output` requires one rendered artifact; a fleet that
  produces both desired and cron artifacts cannot be written to one file.

When `DIRECTORY` is omitted, the CLI searches upward for a configuration
repository and otherwise uses the current directory.

### Hub snippets

```text
remotr hub snippet import [ENTRY-ID]
  [--out PATH] [--hub-root PATH] [--catalog PATH]
  [--catalog-url URL] [--json]
```

Copies a catalog entry into the current configuration repository. Without an
entry ID, an interactive terminal shows a selector; non-interactive use must
supply it. A `kind: module` entry defaults to `modules/<entry-id>.yaml`; a
`kind: crons` entry defaults to `crons/<entry-id>.yaml`.

## Enrollment commands

### One-time enrollment token

```text
remotr enroll token create --fleet NAME
  [--ttl 168h] [--out PATH] [--quiet] [--json]
```

The token can enroll once before expiry. `--out` writes mode `0600`. When
stdout is not a terminal, secret token output is suppressed unless the command
explicitly permits it; use `--out` in automation.

### Reusable deployment tokens

| Command | Options |
| --- | --- |
| `remotr deployment create` | `--label LABEL`, `--fleet FLEET`, `--ttl 8760h`, `--out PATH`, `--quiet`, `--json` |
| `remotr deployment list` | Output flags |
| `remotr deployment show [LABEL]` | `--label LABEL`, output flags |
| `remotr deployment revoke [LABEL]` | `--label LABEL`, `--confirm LABEL` |

A deployment token is shown only at creation. Labels are unique administrative
identities, not secret values. Revocation prevents future enrollments and does
not unregister endpoints already enrolled with the token.

## Inventory and endpoint commands

### Fleet inventory

```text
remotr inventory [--save] [output flags]
```

Fetches all enrolled endpoints and their latest system-information snapshots.
Rows include OS, CPU, RAM, primary IP, disk-encryption count, TPM, agent
version, and last check-in. `--save` creates
`remotr-inventory-YYYYMMDD-HHMMSS.txt` or `.json` in the current directory;
the file mode is `0644`, so choose the working directory accordingly.

### Endpoint registry

| Command | Options / arguments |
| --- | --- |
| `remotr endpoint list` | Output flags |
| `remotr endpoint show [ID]` | `--endpoint ID`, output flags |
| `remotr endpoint remove [ID]` | `--endpoint ID`, `--confirm ID` |

`remove` unregisters the server record. It does not stop or uninstall the
agent on the host.

### Labels

| Command | Options / arguments |
| --- | --- |
| `remotr endpoint label set ID key=value` | Alternatives: `--endpoint ID --key KEY --value VALUE` |
| `remotr endpoint label unset ID KEY` | Alternatives: `--endpoint ID --key KEY` |
| `remotr endpoint label list ID` | `--endpoint ID`, output flags |

Labels are stored server-side. An agent sync may overwrite a key that the agent
also reports, so reserve namespaces for operator-owned and fact-owned labels.

### Compliance, schedules, and upgrades

| Command | Options / arguments |
| --- | --- |
| `remotr endpoint state report ID` | `--endpoint ID`, output flags |
| `remotr endpoint cron report ID` | `--endpoint ID`, output flags |
| `remotr endpoint agent upgrade ID` | `--endpoint ID`, `--version TAG` |
| `remotr fleet list` | Output flags |
| `remotr fleet state report FLEET` | `--fleet FLEET`, `--verbose`, output flags |
| `remotr fleet cron report FLEET` | `--fleet FLEET`, `--verbose`, output flags |
| `remotr fleet agent upgrade FLEET` | `--fleet FLEET`, `--version TAG` |

The non-verbose fleet reports provide summaries; `--verbose` includes each
endpoint's complete report. Upgrade commands record an in-band request that
the agent receives on a later sync.

### Diagnostics

```text
remotr diagnostics collect [ENDPOINT-ID]
  [--endpoint ID] [--since RFC3339] [--until RFC3339]
  [--collectors ID ...] [--timeout 5m]
  [--stdout | --save PATH] [output flags]
```

Creates a request, waits for the endpoint, downloads the tar.gz bundle, and
verifies server metadata. `--collectors` is repeatable and accepts only
allowlisted IDs. The default time range is the prior 24 hours. `--save` writes
mode `0600`; `--stdout` emits binary bytes and must not be combined with human
table processing. Bundle entries contain classified presence, byte/line count,
and fingerprint metadata; raw journal, network, kernel, system-information,
and agent-state bytes are excluded.

### Firewall evidence

| Command | Options / arguments |
| --- | --- |
| `remotr firewall logs ID` | `--endpoint ID`, output flags |
| `remotr firewall report ID` | `--endpoint ID`, output flags |
| `remotr firewall export [ID]` | `--endpoint ID` or `--fleet FLEET`, `--output PATH`, `--format csv\|json` |

`logs` shows applicator audit records; `report` shows current live rules
reported by the endpoint. `export` can aggregate a fleet.

## Git and audit commands

### Git sync

```text
remotr git sync [--json]
```

Requests a server fetch/checkout/composition cycle. A successful HTTP request
does not mean agents have converged; inspect the returned release information
and later state reports. Composition failure leaves the prior release active.

### Audit log

```text
remotr logs list [--since 24h|RFC3339] [--until RFC3339]
  [--action ACTION] [--actor-type operator|endpoint|anonymous]
  [--limit 100] [--cursor CURSOR] [output flags]
remotr logs export-info [output flags]
```

`--limit` is capped at 1000. A response may include a next cursor for another
page. `export-info` returns the protected SIEM export path configured by the
server; it does not export secret material itself.

## Change control

| Command | Required / notable options |
| --- | --- |
| `remotr change list` | Output flags |
| `remotr change show ID` | Output flags |
| `remotr change regenerate LEGACY-ID` | Output flags; replacement plan is server-derived |
| `remotr change watch ID` | `--interval 2s`, `--timeout DURATION`, output flags |
| `remotr change authorize ID` | `--attempt-limit 1`, `--max-concurrency 1`, `--justification TEXT`, output flags |
| `remotr change pause ID` | Output flags |
| `remotr change resume ID` | Output flags |
| `remotr change revoke ID` | Output flags |
| `remotr change baseline-promote ID` | `--resource CONFIG/RESOURCE`, `--acknowledge-exceptions`, output flags |
| `remotr change baseline-adopt` | `--fleet FLEET`, output flags |

`watch` polls only when `--timeout` is positive; otherwise it prints one
snapshot. `authorize` requires a justification and adds an approval toward the
configured threshold. `regenerate` accepts only the legacy Change request ID;
the client cannot submit hashes, providers, effects, or a Fleet override. See
[Change control](../guides/change-control.md) for
the current enforcement boundary, durable-state behavior, and recovery
procedure before using these commands operationally.

## Secrets

```text
remotr secret upload LOGICAL-NAME --file PATH|-
  (--fleet FLEET | --endpoint ID) [output flags]
remotr secret list LOGICAL-NAME [output flags]
remotr secret activate LOGICAL-NAME VERSION [output flags]
remotr secret revoke LOGICAL-NAME VERSION
  --confirm LOGICAL-NAME@VERSION [output flags]
```

Secret bytes are never accepted as a command-line argument. A file input must
be a regular file owned by the invoking UID, not a symlink, with mode `0600`
or stricter. Stdin and file inputs are bounded. The CLI and Admin API expose
safe metadata, not plaintext reads.

Activation of high-risk `@active` uses creates change requests and resolution
remains blocked until authorization. Revocation blocks future resolution but
cannot erase copies already written to an endpoint.

## RBAC and operator credentials

### Roles and rules

| Command | Options / arguments |
| --- | --- |
| `remotr rbac role list` | Output flags |
| `remotr rbac role create NAME` | `--description TEXT`, `--json` |
| `remotr rbac role show NAME` | Alternative `--name`, output flags |
| `remotr rbac role delete NAME` | Alternative `--name`, `--confirm NAME` |
| `remotr rbac rule add ROLE` | Alternative `--role`, `--method METHOD|*`, `--path PATTERN`, `--json` |
| `remotr rbac rule remove ROLE RULE-ID` | Alternatives `--role`, `--id`, `--confirm RULE-ID` |
| `remotr rbac operator list` | Output flags |
| `remotr rbac operator set-roles OPERATOR-ID` | Alternative `--operator`, repeatable `--role ROLE`, `--json` |

`set-roles` replaces the complete role assignment; it does not append. Hidden
flat aliases such as `rbac role-list` remain for compatibility, but new scripts
should use nested commands.

### Stamp an automation credential

```text
remotr admin credential stamp [OUTPUT-DIRECTORY]
  [--out DIRECTORY] [--label LABEL] [--role ROLE ...] [--json]
```

Writes `cert.pem`, `key.pem`, `ca.pem`, and `state.json` mode `0600` for an
automation identity. Use a dedicated label and the minimum roles required.

## Custom application packages

Local package authoring:

| Command | Options |
| --- | --- |
| `remotr package create` | `--path DIR` (required), `--name NAME`, `--version VERSION`, `--mode binary\|script\|build`, `--force` |
| `remotr package build` | `--path DIR` (required), `--output/-o ZIP`, `--push`, `--s3-key KEY`, output flags |

Catalog operations:

| Command | Options / arguments |
| --- | --- |
| `remotr app package validate PATH.zip` | — |
| `remotr app publish PATH.zip` | `--s3-key KEY` |
| `remotr app list` | `--name PREFIX`, output flags |
| `remotr app show NAME VERSION` | — |
| `remotr app delete NAME VERSION` | `--delete-object`, `--confirm "NAME VERSION"` |

`package build --push` combines local build validation with authenticated
upload and registration. `app delete` removes catalog metadata; the S3 object
is retained unless `--delete-object` is explicit. See [Custom app
packages](../guides/custom-app-packages.md) and [Package manifest
reference](custom-package-format.md).

Although the current help text calls `--s3-key` an override, the upload API
accepts it only when it equals the server's canonical
`app-packages/<name>/<version>/<safe-name>-<version>.zip` key. Omit the flag in
normal use; an arbitrary custom path is rejected.

## Shell completion

The CLI enables the command framework's hidden
`--generate-shell-completion` flag. It prints completion candidates for a
partially entered invocation and is intended for a shell integration to call;
it is not a `generate-shell-completion` subcommand and it does not install a
completion script.

```bash
remotr --generate-shell-completion
```

The repository currently does not ship a supported completion installer. If
you wire the flag into local shell completion, test it again after CLI upgrades
because the integration surface belongs to the CLI framework.
