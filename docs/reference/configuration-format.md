# Configuration format reference

Deployable artifacts are YAML files with a top-level `configurations` list. Each list entry is a **configuration slice** — a named group of resources. The agent builds **resolved desired state** by filtering slices and resources using `targetDistros` and `targetArch`, then runs Check and Apply.

## Top-level structure

```yaml
configurations:
  - name: <unique-configuration-name>
    description: optional human text
    lastUpdated: "2026-05-09T12:30:00Z"   # optional RFC3339
    targetDistros: [Debian, Ubuntu, Arch]
    targetArch: [x86, ARM]
    packages: [...]
    files: [...]
    users: [...]
    systemd: [...]
    commands: [...]
```

If `targetDistros` or `targetArch` is omitted on a slice, that slice applies to all distros or architectures the agent supports.

### Valid targeting values

| Field | Values |
|-------|--------|
| `targetDistros` | `Debian`, `Ubuntu`, `Arch` |
| `targetArch` | `x86`, `ARM` |
| `packageManager` | `apt`, `pacman`, `yay`, `dnf` |

## Resource metadata

All resource kinds support inline metadata:

```yaml
dependsOn:
  - other-config/resource-name
preApplyValidation:
  - sshd -t
```

| Field | Description |
|-------|-------------|
| `dependsOn` | Wait until named resources apply successfully. Address format: `configuration-name/resource-name`. |
| `preApplyValidation` | Commands that must exit 0 before apply (for example config syntax checks). |

## Packages

```yaml
packages:
  - name: curl
    present: true
    packageManager: apt
  - name: nmap
    present: false
    packageManager: pacman
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Logical name (also used in `dependsOn`) |
| `present` | yes | `true` install/ensure; `false` remove |
| `packageManager` | recommended | Which backend to use on multi-distro slices |
| `arch` | no | Architecture filter on the resource |

## Files

Two modes: **content** (create/replace file) or **regex** (line-oriented edit of existing file).

### Content mode

```yaml
files:
  - name: motd
    path: /etc/motd
    content: |
      Managed by Remotr
    mode: [0644]
```

### Regex mode (update existing)

```yaml
files:
  - name: disable root login
    path: /etc/ssh/sshd_config
    updateExisting: true
    withRegx: s/^#?PermitRootLogin.*/PermitRootLogin no/
```

| Field | Description |
|-------|-------------|
| `path` | Absolute path on the endpoint |
| `updateExisting` | When true, apply regex to existing file |
| `withRegx` | Substitution or match pattern (see applicator for semantics) |
| `content` | Full file body when not using regex |
| `mode` | Optional file mode (octal integers) |

Critical paths under `/etc` apply later in default **apply order** than non-critical files.

## Users

```yaml
users:
  - name: dev-user
    username: dev
    present: true
    uid: 1500
```

| Field | Description |
|-------|-------------|
| `username` | Linux account name |
| `present` | Create or remove local user |
| `uid` | Optional fixed UID |

## Systemd

```yaml
systemd:
  - name: sshd-enabled
    unit: sshd.service
    enabled: true
    active: true
    masked: false
```

| Field | Description |
|-------|-------------|
| `unit` | systemd unit name |
| `enabled` | Unit enabled at boot (`systemctl enable`) |
| `active` | Unit running (`systemctl start`) |
| `masked` | Unit masked |

Use **command resources** for timers, drop-ins, or `daemon-reload` edge cases.

## Commands (escape hatch)

Explicit check / apply / revert argv. Drift is detected when `check` is defined and exits non-zero.

```yaml
commands:
  - name: custom-banner
    check:
      - test -f /etc/remotr-banner-applied
    apply:
      - sh
      - -c
      - echo ok > /etc/remotr-banner-applied
    revert:
      - rm
      - -f
      - /etc/remotr-banner-applied
```

Authors own idempotency. Prefer builtin resource kinds when the operation is common.

## Apply order

Default order when `dependsOn` does not override:

1. Packages
2. Non-critical files
3. Critical files (under `/etc`)
4. Users
5. Systemd units
6. Commands

`dependsOn` edges override default ordering. Cycles are rejected.

## Remediation policy

Per-fleet setting on the server (`auto` or `report`):

- **auto** — after Check finds drift, Apply runs on sync
- **report** — drift is reported to the server; Apply is skipped

Policy is returned in each sync response; the agent does not read policy from YAML.

## Complete example

```yaml
configurations:
  - name: base-packages
    description: Baseline tools
    targetDistros:
      - Debian
      - Arch
    targetArch:
      - x86
    packages:
      - name: curl
        present: true
        packageManager: apt
      - name: curl
        present: true
        packageManager: pacman

  - name: ssh-hardening
    description: SSH baseline
    targetDistros:
      - Debian
      - Arch
    files:
      - name: disable root login
        path: /etc/ssh/sshd_config
        updateExisting: true
        withRegx: s/^#?PermitRootLogin.*/PermitRootLogin no/
      - name: sshd-config-valid
        path: /etc/ssh/sshd_config
        updateExisting: true
        withRegx: (?mi)^PasswordAuthentication\s+no\s*$
        preApplyValidation:
          - sshd -t
    systemd:
      - name: sshd-running
        unit: sshd.service
        enabled: true
        active: true
```

## Parser errors

Common validation failures:

- Empty file or missing `configurations`
- Invalid YAML syntax
- Duplicate configuration or resource names
- Invalid `dependsOn` address
- Dependency cycle at engine build time

Fuzz tests live under `internal/models/` — run `make fuzz-short` after parser changes.
