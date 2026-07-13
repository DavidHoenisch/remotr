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
  lifecycle: present
  redirectPolicy: same-origin
  timeout: 30s
```

`url`, absolute `dest`, lifecycle, optional SHA-256 `checksum`, base64 Ed25519
`signature` with `trustedSigner`, `authenticationRef`, `redirectPolicy`,
`timeout`, `mode`, `owner`, and `group` are checked and applied. Canonical
`notifications` emit shared activation signals. Legacy `notifySystemd` and
systemctl-based `reloadExec` input map to the same activation queue.

## User resources

```yaml
- kind: user
  name: developer-account
  username: developer
  present: true
  primaryGroup: developers
  supplementaryGroups: [docker]
  supplementaryGroupsMode: merge
  home: /home/developer
  createHome: true
  shell: /bin/bash
  comment: Developer Account
```

`username` and `present` are required. Optional managed account fields are
`uid` (with `allowUIDReassignment: true` for an existing account),
`primaryGroup`, `supplementaryGroups`, `supplementaryGroupsMode` (`merge` or
`authoritative`), `home`, `createHome`, `shell`, `comment`, `system`,
`passwordHashRef`, `locked`, `expiry`, `removeHome`, and `forceRemoval`.
Omitted fields remain unmanaged. Password material is a reference, never an
inline hash or secret value.

## M2 local access resources

The M2 baseline manages local administrator access with first-class resources;
do not replace these resources with generic files or `command` resources.

### Group resources

```yaml
- kind: group
  name: developers
  lifecycle: present
  group: developers
  gid: 2200
  system: false
```

`group` is the account-database group name. `gid` and `system` are optional;
changing an existing GID requires `allowGIDReassignment: true`. Group
operations share the account-database lock.

### Authorized-key resources

```yaml
- kind: authorizedKey
  name: developer-admin-key
  lifecycle: present
  ownership: authoritative
  enforce: true
  user: developer
  recoveryPrincipals: [recovery]
  entries:
    - type: ssh-ed25519
      key: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W
      fingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4
      comment: developer admin
      restrictions: [no-agent-forwarding, from="10.0.0.0/8"]
```

Each entry requires its OpenSSH key `type`, base64 `key`, and matching
SHA-256 `fingerprint`; optional `comment`, `restrictions`, `principals`, and
`expiresAt` are rendered as structured entry metadata. `merge` adds entries to
the resource-owned block. `authoritative` replaces that block and therefore
requires `recoveryPrincipals` and `enforce: true` before the agent may apply
it. In both modes Remotr traverses the selected home without following
symlinked parents and never edits entries outside its marker.

Use `lifecycle: absent` to revoke the resource-owned block. It does not delete
unmanaged keys from `authorized_keys`.

### Known-host resources

```yaml
- kind: knownHost
  name: source-git
  lifecycle: present
  ownership: named
  scope: user
  user: developer
  hosts: [git.example.internal]
  type: ssh-ed25519
  key: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W
  fingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4
  hashing: hash
  replaceExisting: false
```

`scope` is `user` or `system`. A user scope requires `user`; system scope
uses the system known-host file. `hashing` is `plain` or `hash`; `hash` writes
and recognizes OpenSSH `|1|` hashed host patterns. Existing conflicting
host/key records are preserved unless `replaceExisting: true` is set. Remotr
only changes its named marker plus an explicitly selected conflicting record.

### Sudo resources

```yaml
- kind: sudo
  name: developer-admin
  lifecycle: present
  ownership: fragment
  enforce: true
  subjects: [developer]
  runAs: [ALL]
  commands: [/usr/bin/id]
  tags: [NOPASSWD]
  recoveryPrincipals: [recovery]
```

Sudo resources create one named Remotr-owned `/etc/sudoers.d` fragment.
`subjects`, optional `runAs`, `commands`, and optional sudo `tags` are rendered
as policy, rather than accepting raw sudoers text. `recoveryPrincipals` is
required. The agent stages the complete configured sudoers include tree and
runs `visudo` validation before atomically activating the fragment. A failed
validation leaves the active fragment unchanged. Current rollback is reported
as best-effort; it restores the prior fragment only while the local attempt is
still available.

Use `lifecycle: absent` to remove only that named fragment. See [Local
administrator access](../guides/local-administrator-access.md) for a complete
baseline, compatibility notes, and recovery procedure.

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
downloads, critical files, users, groups, SSH access resources, sudo fragments,
user files, firewall, systemd, systemd-user, bootstrap, agent-install, and
commands. Registry ordering is deterministic; `dependsOn` edges take
precedence.

## Legacy schema 0 compatibility

An unversioned artifact is interpreted as schema 0 during the compatibility
window. Plural collections such as `packages`, `files`, and `systemd` remain
readable, but validation and discovery emit `legacy_schema_0`. Composition
renders their schema-1 `resources` equivalent without changing requested
behavior. `authorizedKey`, `knownHost`, and `sudo` are schema-1-only resource
kinds: migrate generic file or command-based SSH/sudo edits to their named
resources before enabling M2 access management.

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
