# Configuration format reference

Remotr deployable desired-state artifacts use strict `schemaVersion: 1` YAML.
Each configuration contains one `resources` list; every resource has an explicit
`kind` and a name that is unique across all kinds in that configuration.

Configuration repositories normally store `kind: module` source files and let
the server compose them. `remotr config render` previews the canonical artifact
without writing `desired.yaml` or `crons.yaml` into the repository.

## Canonical structure

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: workstation-base
    description: Portable workstation baseline
    targetDistros: [Debian, Ubuntu, Arch]
    targetArch: [x86, ARM]
    resources:
      - kind: package
        name: curl
        present: true
      - kind: file
        name: motd
        path: /etc/motd
        content: |
          Managed by Remotr
```

`kind: module` identifies a repository source file. Composed deployable output
starts with `schemaVersion: 1` and omits the repository-file `kind`.

Valid targeting values are `Debian`, `Ubuntu`, and `Arch` for
`targetDistros`, and `x86` and `ARM` for `targetArch`. Omitted targeting fields
match every currently supported endpoint.

## Resource identity and dependencies

A resource address is `<configuration>/<resource-name>`, such as
`workstation-base/curl`. Names are unique across package, file, service, and all
other kinds in one configuration. Dependencies may cross configurations and
must use complete stable addresses:

```yaml
- kind: systemd
  name: ssh-running
  dependsOn:
    - ssh-policy/sshd-config
  unit: ssh.service
  enabled: true
  active: true
```

Missing dependencies and dependency cycles are rejected by `config validate`.

The following shared execution fields are implemented:

| Field | Meaning |
| --- | --- |
| `dependsOn` | Stable addresses that must succeed before this resource may apply. |
| `preApplyValidation` | Commands that must exit successfully immediately before Apply. |
| `risk` | `normal`, `sensitive`, `connectivity`, `access`, `boot`, or `destructive`. |
| `enforce` | Explicitly permits enforcement of a non-normal-risk resource after preflight. |
| `lockDomains` | Additional agent-local exclusive operation locks. |

The schema reserves shared `lifecycle`, `providerOptions`, `policy`,
`ownership`, `validation`, `notifications`, and `authorizationGroup` metadata.
Do not author those fields yet unless a later resource-specific section says
they are convergent; parsing a reserved field is not an advertisement that a
provider enforces it.

## Provider and capability validation

Use `remotr config discover --fleet <name>` to see canonical resource kinds and
the artifact's capability requirements. `config validate` rejects combinations
that are statically impossible. A local backend mismatch that is knowable only
on an endpoint is reported as `unsupported`, not ordinary drift, and is never
applied.

Current native package targeting is:

| Provider | Target |
| --- | --- |
| `apt` | Debian and Ubuntu |
| `pacman` | Arch |
| omitted | Select APT or Pacman from normalized endpoint facts |

DNF/RPM-family providers are deferred and canonical validation rejects `dnf`
with a roadmap diagnostic. Flatpak, PWA, and Remotr catalog packages remain
separate providers.

## Package resources

```yaml
- kind: package
  name: curl
  lifecycle: present
  packageManager: apt
  version: 8.10.1-1ubuntu2.3
  allowUpgrade: true
```

Implemented fields:

| Field | Meaning |
| --- | --- |
| `name` | Logical resource name and package identifier. |
| `lifecycle` | Canonical `present` or `absent`; APT also supports `purged`. Schema 0 maps legacy `present: true/false` to this field. |
| `packageManager` | Optional `apt`, `pacman`, `flatpak`, `pwa`, or `remotr`. `yay` and `dnf` are rejected until truthful providers exist. |
| `arch` | Optional resource-level `x86` or `ARM` filter. |
| `version` | Exact native version for APT/Pacman; required catalog version for `remotr`. |
| `allowUpgrade`, `allowDowngrade` | Explicit native-version transition policy. Downgrades default to denied. |
| `hold` | APT native hold state. Rejected for providers without check/apply support. |
| `refreshCache` | Refresh APT/Pacman metadata before a drifted installation. |
| `removeDependencies` | Use provider-supported dependency cleanup when removing a package. |
| `nonInteractive` | May be omitted or `true`; interactive transactions are rejected. |
| `flatpakRemote`, `flatpakRemoteURL` | Flatpak remote selection; custom remotes require a URL. |
| `pwaURL`, `pwaTitle`, `pwaIcon`, `pwaBrowser`, `pwaUsers` | PWA launcher fields; `pwaURL` is required and `pwaUsers` is `interactive`. |

Package transactions share the `package-database` lock, use sanitized
noninteractive environments, and report activation/reboot requirements without
rebooting. Existing schema-0 `present` input remains readable during the
compatibility window; new schema-1 resources must use `lifecycle`.

## File resources

Whole-file content:

```yaml
- kind: file
  name: motd
  path: /etc/motd
  content: |
    Managed by Remotr
```

Line-oriented edit of an existing file:

```yaml
- kind: file
  name: disable-root-login
  path: /etc/ssh/sshd_config
  updateExisting: true
  withRegx: '(?m)^PermitRootLogin no$'
  replaceRegx: '(?m)^#?PermitRootLogin.*$'
  content: PermitRootLogin no
  preApplyValidation:
    - sshd -t
```

`path`, `content`, `updateExisting`, `withRegx`, and `replaceRegx` are the
currently convergent file fields. File lifecycle, kind replacement, ownership,
and metadata convergence are introduced by later filesystem slices. In
particular, do not treat `mode` on a system file as fully convergent yet.

## Interactive-user file resources

```yaml
- kind: userFile
  name: app-settings
  users: interactive
  path: .config/example/settings.conf
  content: |
    enabled=true
```

`users` is currently `interactive`; `path` is relative to each selected home
and may not escape it. Content and line-edit fields match the system-file
resource. Per-user mode convergence and broader selectors are not advertised.

## Download resources

```yaml
- kind: download
  name: helper
  url: https://example.com/helper
  dest: /usr/local/bin/helper
  checksum: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  mode: [493]
```

`url`, absolute `dest`, optional SHA-256 `checksum`, and `mode` are checked and
applied. Signature, authentication-reference, redirect, timeout, lifecycle,
and shared notification behavior are not advertised yet. Legacy
`notifySystemd` and `reloadExec` remain compatibility input until their shared
activation migration is complete.

## User resources

```yaml
- kind: user
  name: developer-account
  username: developer
  present: true
```

Only `username` and `present` are currently advertised as convergent. `uid` and
the broader account model remain unavailable until their Check and Apply slices
are complete.

## Systemd resources

```yaml
- kind: systemd
  name: ssh-running
  unit: ssh.service
  enabled: true
  active: true
  masked: false
```

`enabled`, `active`, and `masked` are independently optional managed fields.
The endpoint must report the `systemd` init backend; otherwise Check returns
`unsupported`.

For interactive users:

```yaml
- kind: systemdUser
  name: desktop-agent
  unit: desktop-agent.service
  users: interactive
  linger: true
  enabled: true
  active: true
```

## Firewall resources

Firewall rules default to audit/report mode. Explicit `audit: false` enables
the currently implemented firewalld or nftables mutation path after applicable
safety policy.

```yaml
- kind: firewall
  name: allow-ssh
  action: allow
  protocol: tcp
  ports: [22]
  backend: nftables
  table: filter
  chain: input
  family: inet
```

Current fields are `audit`, `action`, `protocol`, `ports`, `sources`,
`destinations`, `services`, `zones`, `backend`, `table`, `chain`, `family`,
`rule`, and `protectRemotr`. Rule absence, authoritative ownership, and timed
rollback acknowledgement are not advertised yet.

## Bootstrap, agent-install, and command resources

The existing `bootstrap`, `agentInstall`, and `command` kinds are available in
canonical `resources` lists with their existing field contracts. Commands use
argv arrays and remain an escape hatch; authors own their idempotency and
rollback behavior. Prefer a typed resource when one exists.

## Default ordering

Absent dependency edges, resources are ordered as packages, ordinary files,
downloads, critical files, users, user files, firewall, systemd, systemd-user,
bootstrap, agent-install, and commands. Registry ordering is deterministic;
`dependsOn` edges take precedence.

## Legacy schema 0 compatibility

An unversioned artifact is interpreted as schema 0 during the compatibility
window. Plural collections such as `packages`, `files`, and `systemd` remain
readable, but validation and discovery emit `legacy_schema_0`. Composition
renders their schema-1 `resources` equivalent without changing requested
behavior.

Schema 0 is retained for at least two minor releases and 90 days after schema
1 ships, and cannot be removed while fleet telemetry still reports schema-0
endpoints. New configuration should use schema 1.

## Tooling

```bash
remotr config validate <repo>
remotr config discover --fleet <fleet> <repo>
remotr config render --fleet <fleet> <repo>
```

`validate` reports errors and non-fatal migration diagnostics. `discover`
reports source file kinds, canonical resource kinds, and capability
requirements. `render` writes only to stdout unless `--output` is explicitly
provided; it never creates composed artifacts in the source repository by
default.
