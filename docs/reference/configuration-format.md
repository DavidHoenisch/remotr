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
    userFiles: [...]
    downloads: [...]
    users: [...]
    systemd: [...]
    systemdUser: [...]
    bootstrap: [...]
    agentInstall: [...]
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

### Line edit mode (update existing)

```yaml
files:
  - name: pass-max-days
    path: /etc/login.defs
    updateExisting: true
    withRegx: (?m)^PASS_MAX_DAYS[[:space:]]+90$
    replaceRegx: (?m)^#?PASS_MAX_DAYS[[:space:]].*
    content: PASS_MAX_DAYS 90
```

| Field | Description |
|-------|-------------|
| `path` | Absolute path on the endpoint |
| `updateExisting` | When true with `withRegx`, replace or append lines instead of overwriting the file |
| `withRegx` | Desired-state check: file must match this regex |
| `replaceRegx` | Optional line pattern to replace on apply; defaults to `withRegx` |
| `content` | Replacement line (line edit) or full file body (content mode) |
| `mode` | Optional file mode (octal integers) |

Critical paths under `/etc` apply later in default **apply order** than non-critical files.

## User files (interactive home directories)

Apply the same file operations as `files` under **each interactive user's home directory** (local accounts from `/etc/passwd` with UID ≥ 100, excluding `nobody`). Files are owned by that user after apply.

```yaml
userFiles:
  - name: app-config
    users: interactive
    path: .config/myapp/settings.conf
    content: |
      enabled=true
    mode: [0644]
```

Line edit in an existing dotfile:

```yaml
userFiles:
  - name: app-flag
    users: interactive
    path: .config/myapp/settings.conf
    updateExisting: true
    withRegx: (?m)^enabled=false$
    content: enabled=true
```

| Field | Description |
|-------|-------------|
| `users` | Must be `interactive` (v1) |
| `path` | Relative to each user's home directory (no leading `/`, no `..`) |
| Other fields | Same as `files` (`content`, `updateExisting`, `withRegx`, `replaceRegx`, `mode`) |

Runs after `users` resources in default apply order so accounts exist before home files are written.

## Downloads

Fetch a remote file to a fixed path (checksum optional).

```yaml
downloads:
  - name: audit-rules
    url: https://raw.githubusercontent.com/Neo23x0/auditd/master/audit.rules
    dest: /etc/audit/rules.d/audit.rules
    mode: [0640]
    checksum: sha256:abc123...
    notifySystemd: auditd.service
```

| Field | Description |
|-------|-------------|
| `url` | HTTP(S) source |
| `dest` | Absolute destination path |
| `checksum` | Optional `sha256:<hex>` |
| `notifySystemd` | Restart unit after a successful apply |

## Systemd user units

Enable/start a unit for each interactive user (UID ≥ 1000, excluding `nobody`), with optional linger.

```yaml
systemdUser:
  - name: soc2-idle-lock
    unit: soc2-idle-lock.service
    unitPath: /etc/systemd/user/soc2-idle-lock.service
    users: interactive
    linger: true
    enabled: true
    active: true
```

| Field | Description |
|-------|-------------|
| `users` | Must be `interactive` — all passwd accounts with UID ≥ 100 (excluding `nobody`) |
| `unitPath` | When set, the unit file must exist before apply |
| `linger` | Run `loginctl enable-linger` per user |

## Bootstrap

One-shot orchestration while a path condition holds.

```yaml
bootstrap:
  - name: clamav-db
    when:
      pathMissing: /var/lib/clamav/main.cvd
    steps:
      - systemd:
          unit: clamav-freshclam.service
          active: false
      - exec: [freshclam]
      - systemd:
          unit: clamav-freshclam.service
          active: true
          enabled: true
```

| Field | Description |
|-------|-------------|
| `when.pathMissing` | Run steps while this path does not exist |
| `when.pathExists` | Run steps while this path exists |
| `steps` | Ordered `systemd` or `exec` steps |

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

Use **command resources** for timers and drop-ins. The agent runs `systemctl daemon-reload` before enable/start on **systemd** and **bootstrap** steps.

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

## Agent install

Download a versioned agent tarball, install, and enroll with a fleet URL. Enrollment tokens are never stored in Git — use `enrollmentTokenSecret: file:/absolute/path`.

```yaml
agentInstall:
  - name: elastic-agent
    version: 9.3.0+build202602051825
    artifactURL: https://artifacts.elastic.co/downloads/beats/elastic-agent/elastic-agent-${version}-linux-x86_64.tar.gz
    extractDir: elastic-agent-${version}-linux-x86_64
    fleetURL: https://your-fleet.example:443
    enrollmentTokenSecret: file:/etc/remotr/elastic-agent.token
    runningCheck:
      process: elastic-agent
```

| Field | Description |
|-------|-------------|
| `artifactURL` / `extractDir` | Support `${version}` substitution |
| `enrollmentTokenSecret` | Must be `file:/absolute/path` on the endpoint |
| `runningCheck.process` | `pgrep -f` pattern while agent is healthy |

## Apply order

Default order when `dependsOn` does not override:

1. Packages
2. Non-critical files
3. Downloads
4. Critical files (under `/etc` or with `preApplyValidation`)
5. Users
6. User files (interactive home directories)
7. Systemd (system)
8. Systemd user
9. Bootstrap
10. Agent install
11. Commands

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
