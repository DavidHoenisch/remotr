# Fleet osquery Queries and Runtime Management

Remotr supports GitOps-managed, read-only osquery queries across fleets with
explicit control over **if**, **when**, and **where** `osquery` is installed or
reused—so endpoints running third-party agents (such as Elastic Agent) are never
clobbered and regular users never gain unwanted access to `osqueryi`.

## 1. Configure the `osquery` runtime (`kind: osquery`)

Add a `kind: osquery` resource to a fleet module (`schemaVersion: 1`). Remotr
never falls back to `$PATH` implicitly.

### Option A: Private managed install (recommended default)

Installs a checksum-verified release into a `root:root` `0700` private directory
outside `$PATH` (`/usr/lib/remotr/osquery` by default), touching zero system
paths or systemd units:

```yaml
- kind: osquery
  name: private-osquery
  lifecycle: present
  mode: managed
  visibility: private
  version: "5.14.1"
  sourceUrl: https://downloads.example.internal/osquery-5.14.1-linux-amd64.tar.gz
  sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
```

### Option B: Reuse an existing binary (e.g., Elastic Agent)

Reuses an already-installed `osqueryi` or `osqueryd` binary (invoking `osqueryd`
with `-S --json --ephemeral --disable_database --disable_events`) after verifying symlinks, owner, and non-writable
permissions. Remotr never modifies or removes external binaries:

```yaml
- kind: osquery
  name: elastic-osquery
  lifecycle: present
  mode: existing
  allowedOwner: root
  binaryPaths:
    - /opt/Elastic/Agent/data/elastic-agent-*/components/osqueryd
    - /usr/bin/osqueryi
```

### Option C: Global managed install or disabled

- Set `mode: managed` + `visibility: global` to install under
  `/opt/remotr-osquery` and create `/usr/local/bin/osqueryi` (refusing to
  overwrite any existing unmanaged binary at that path).
- Set `mode: disabled` to ensure nothing is installed and endpoints report
  `unsupported` for dispatched queries.

## 2. Author fleet queries (`kind: queries`)

Create a `queries/security.yaml` file and reference it from `queries:` in your
fleet manifest (`fleets/prod/manifest.yaml`):

```yaml
kind: queries
schemaVersion: 1
queries:
  - name: listening-ports
    description: Non-loopback listening TCP/UDP sockets
    schedule: "*/30 * * * *"
    timezone: UTC
    mode: diff              # snapshot (default) | diff
    timeout: 15s
    maxRows: 500
    retentionRuns: 20
    targetDistros: [Ubuntu, Debian, Arch]
    sql: |
      SELECT lp.port, lp.protocol, lp.address, p.name AS process
      FROM listening_ports lp
      JOIN processes p ON lp.pid = p.pid
      WHERE lp.address NOT IN ('127.0.0.1', '::1')
```

Validate and render before committing:

```bash
remotr config validate ./remotr-config
remotr config render --fleet prod ./remotr-config
```

## 3. Inspect and run queries (`remotr query`)

```bash
# List fleet queries and coverage
remotr query list --fleet prod

# Inspect latest fleet results (flattened table, auto-card on narrow terminals)
remotr query results listening-ports --fleet prod

# Spot rare outliers across large fleets
remotr query results listening-ports --fleet prod --group-by

# Interactive Bubble Tea viewer (/ filter, g toggle raw/grouped, s sort)
remotr query results listening-ports --fleet prod --tui

# Export filtered CSV/JSON/NDJSON
remotr query results listening-ports --fleet prod --where port=443 --format csv --save ports.csv

# View server-computed +/- diffs
remotr query diff listening-ports --fleet prod --since 24h

# Run an ad-hoc named query (query_runner role) or inline SQL (query_sql_runner role)
remotr query run listening-ports --endpoint <endpoint-id>
remotr query run --sql "SELECT version FROM os_version" --fleet prod
```

## RBAC roles

| Built-in role | Allowed operations |
| --- | --- |
| `query_reader` (and `read_only`) | `list`, `status`, `results`, `diff` (`GET /v1/admin/queries*`) |
| `query_runner` | Read operations + ad-hoc runs of GitOps-named queries (`POST /v1/admin/queries/run`) |
| `query_sql_runner` (and `global_admin`) | All above + ad-hoc inline read-only SQL (`POST /v1/admin/queries/run-sql`) |
