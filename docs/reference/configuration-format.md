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

## Secret references

Secret-valued fields accept references only; inline passwords, private keys,
tokens, and credential material are rejected by canonical validation. The
implemented providers are:

- `local-file:/absolute/path` for material provisioned independently on the
  endpoint. The agent opens a root-owned regular file without following
  symlinks and rejects group- or world-accessible files.
- `remotr:<logical/name>@active` to follow audited activation, or
  `remotr:<logical/name>@<positive-version>` to pin an exact server-managed
  version. Omitting the selector is rejected rather than interpreted as
  latest. Resolution
  uses endpoint mTLS and is bound to the endpoint Fleet, active artifact
  digest, resource address, and declared purpose. The Admin API has no
  plaintext-read route.

The legacy `file:/absolute/path` spelling remains readable as `local-file`
during migration. A provider is never used as a fallback for another provider.
An exact version changes only when Git changes. An `active` reference changes
only through the separate audited server-registry activation operation.

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

## Kernel-module resources

```yaml
- kind: kernelModule
  name: loop
  module: loop
  loaded: true
  persistent: true
  parameters:
    max_loop: "64"
```

`loaded`, `persistent`, `parameters`, and `blacklisted` are independently
optional. `loaded: false` removes a live module; `persistent: false` removes
only this resource's `modules-load.d` declaration. Parameters and blacklist
state are rendered in one named `modprobe.d` fragment. A requested false is
different from an omitted field, which leaves that scope unmanaged. Parameters
apply to the live module only when `loaded: true` is also managed; otherwise
they define boot-time modprobe behavior without taking ownership of a module
that another system component already loaded.

Kernel modules are boot-risk resources. An unload, a live parameter reload, or
a blacklist first checks declared `protectedModules` and the currently active
root and network drivers. If the module backs one of those subsystems, Remotr
blocks the change before it invokes `modprobe` or changes an owned fragment.
Use the normal boot-risk authorization and enforcement workflow for permitted
changes; Remotr never reboots as a side effect of module management.

## Host-locale resources

```yaml
- kind: hostLocale
  name: berlin
  timezone: Europe/Berlin
  locale:
    LANG: de_DE.UTF-8
  keymap: de
```

`timezone`, `locale`, and `keymap` are independently optional: an omitted
field is neither queried nor changed. `timezone` must be an installed IANA
timezone; `locale` is a non-empty map of `LANG`, `LANGUAGE`, or `LC_*`
variables; and `keymap` is a console keymap name. The systemd provider uses
`timedatectl` and `localectl`, without owning `/etc/hosts` or an unrelated
locale variable. A locale update reports `logout-required`; a console-keymap
update reports `reboot-required`. These are visible activation signals only —
Remotr does not end sessions or reboot the host as an incidental effect.

## Time-synchronization resources

```yaml
- kind: timeSync
  name: primary-ntp
  provider: systemd-timesyncd
  enabled: true
  servers: [time.example.internal]
  pools: [pool.ntp.org]
```

Time synchronization has a provider-neutral resource contract. The currently
advertised provider is `systemd-timesyncd`: `enabled` is independent from the
optional `servers` and `pools` lists. Server configuration is placed only in a
named `/etc/systemd/timesyncd.conf.d` fragment; changing that fragment reports
a restart of `systemd-timesyncd.service` rather than silently treating it as
active. A provider that cannot manage requested servers or pools reports the
field as unsupported; Remotr never accepts a partial enablement-only result.

## Mount resources

```yaml
- kind: mount
  name: application-cache
  source: tmpfs
  target: /var/cache/application
  filesystemType: tmpfs
  options: [mode=0755, size=256m]
  mounted: true
  persistent: true
```

`mounted` and `persistent` are independently optional. `persistent` owns only
the fstab line marked with the mount name, preserving unrelated declarations.
Options are validated, sorted, and deduplicated before use. A runtime change
checks the source, filesystem support, target directory, and whether the path
would hide or detach Remotr state. To remove only the boot declaration, use
`persistent: false` and omit `mounted`. Unmounting is normal by default;
`unmountMode: lazy` is explicit, while `unmountMode: force` also requires
`enforce: true` authorization.

## Swap resources

```yaml
- kind: swap
  name: endpoint-swap
  path: /var/lib/remotr/swapfile
  type: file
  sizeBytes: 2147483648
  priority: 5
  active: true
  persistent: true
```

Swap files are created by a zero-filling, fsynced command, protected to mode
`0600`, formatted, and then activated. Existing devices use `type: device`
and omit `sizeBytes`. Disabling active swap requires `allowRemove: true`; this
prevents an accidental lifecycle edit from exhausting host memory.

## Endpoint schedule resources

```yaml
- kind: endpointSchedule
  name: nightly-backup
  lifecycle: present
  backend: cron
  schedule: "0 3 * * *"
  user: backup
  argv: [/usr/local/bin/backup, "daily archive"]
  workingDirectory: /var/lib/backup
  environment:
    - name: BACKUP_BUCKET
      value: archive
    - name: BACKUP_TOKEN
      secretRef: file:/run/secrets/backup-token
  timeout: 30m
  overlap: forbid
```

The `cron` backend owns one `/etc/cron.d/remotr-<name>` fragment and protected
launcher state under `/var/lib/remotr/schedules`. `argv` and explicit `shell`
forms are mutually exclusive. The launcher preserves argv boundaries, changes
to the optional working directory, resolves environment references outside the
world-readable cron fragment, applies GNU `timeout` when requested, and uses a
non-blocking native lock for `overlap: forbid`. Use `lifecycle: disabled` to
retain protected launcher state without an active cron entry, or `absent` to
remove only this schedule's owned fragment and protected files.

For `backend: systemd-timer`, `schedule` is an `OnCalendar` expression and
`persistent` is required to state whether a missed occurrence runs after the
endpoint returns. Remotr stages and verifies the paired
`remotr-schedule-<name>.service` and `.timer` units together, replaces only
those named units, performs one daemon reload, and converges timer enablement
and active state. Systemd's paired oneshot service prevents overlapping runs;
therefore this backend accepts omitted overlap policy or `overlap: forbid`,
but rejects `overlap: allow`.

Endpoint schedules are installed into the operating system's native scheduler.
Once applied, a due occurrence does not wait for an agent check-in or contact
the Remotr server; it runs with the cron or systemd behavior declared above
while the endpoint is offline. This differs from server-dispatched `crons`,
which become runnable only after an agent receives due work during Sync.

Compliance reports describe only the installed schedule definition and native
enablement state. When the backend exposes useful execution history, Remotr
adds a separate `schedule_runtime` collection with the last status, exit code,
and effective missed-run behavior. A failed last run does not make matching
configuration drift. Omitted runtime telemetry means the backend did not
provide reliable history; it does not mean the job succeeded. Systemd timers
currently provide this optional history, while cron configuration compliance
remains available without claiming portable cron execution history.

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
  selector:
    mode: all-interactive
  path: .config/example/settings.conf
  content: |
    enabled=true
```

Use `selector.mode: all-interactive` for every local interactive account, or
select a closed list without broadening missing targets:

```yaml
selector:
  mode: explicit
  usernames: [alice, bob]
```

An unmatched explicit username is reported as unresolved and blocks Apply.
Legacy `users: interactive` remains accepted during migration. `path` is
relative to each selected home and may not escape it. Content and line-edit
fields match the system-file resource. Check reports at most 32 redacted
per-user subresults and marks the result truncated when additional selected
users are omitted from detail; the aggregate still reflects every user.

## Typed desktop settings

`desktopSetting` manages one native dconf or GSettings value. User-scoped
settings work for logged-out accounts by using a transient private D-Bus bus;
Remotr never needs to start or terminate a graphical session.

```yaml
- kind: desktopSetting
  name: disable-animations
  provider: gsettings
  scope: user
  selector:
    mode: explicit
    usernames: [alice, bob]
  schema: org.gnome.desktop.interface
  key: enable-animations
  value:
    type: boolean
    value: false
```

Supported native types are `boolean`, `string`, `int32`, `int64`, `uint32`, `double`,
and `string-list`. A string containing `"true"` never satisfies a boolean.
The dconf provider also supports persistent system defaults and mandatory
locks for all interactive users:

```yaml
- kind: desktopSetting
  name: lock-animations
  provider: dconf
  scope: system
  level: mandatory
  selector: {mode: all-interactive}
  path: /org/gnome/desktop/interface/enable-animations
  value: {type: boolean, value: false}
```

## Structured session policy

`sessionPolicy` groups persistent lock and idle settings, GNOME proxy and
session restrictions, and MIME default applications without accepting shell
commands. It uses the same explicit or all-interactive selector and dconf or
GSettings backend as `desktopSetting`:

```yaml
- kind: sessionPolicy
  name: workstation-session
  provider: gsettings
  selector:
    mode: explicit
    usernames: [alice, bob]
  lockEnabled: true
  idleTimeoutSeconds: 300
  lockDelaySeconds: 5
  proxy:
    mode: manual
    httpHost: proxy.example.test
    httpPort: 8080
    ignoreHosts: [localhost]
  disableUserSwitching: true
  disableLogout: true
  disableCommandLine: true
  defaultApplications:
    text/html: browser.desktop
    application/pdf: viewer.desktop
```

Proxy mode is `none`, `manual`, or `automatic`; automatic mode requires an
absolute `automaticUrl`. Default applications are applied through `xdg-mime`
inside a transient per-user D-Bus session, so logged-out users are supported.
Remotr does not log users out or start their graphical sessions.

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

Canonical notifications support `daemon-reload`, `reload`, `try-restart`, and
`restart`. Service targets are required for every action except
`daemon-reload`:

```yaml
notifications:
- type: try-restart
  target: telemetry.service
```

These are post-change actions, not steady service state. The engine runs them
only after every producing resource succeeds, coalesces identical requests,
and orders daemon reload before reload, try-restart, and restart. If a producer
fails, its queued actions are not run. Action failures identify the provider,
unit, operation, and exit status while bounding and redacting command output.

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

## Service resources

Use `kind: service` for provider-neutral steady service state:

```yaml
- kind: service
  name: ssh-running
  provider: systemd
  scope: system
  service: ssh.service
  enabled: true
  active: true
  masked: false
```

`enabled`, `active`, and `masked` are independently optional managed fields.
Masking is distinct from stopping: a masked service cannot be enabled or
started. Remotr orders disable and stop before mask, and unmask before enable
or start. The provider is explicit. `systemd` is the only advertised provider;
OpenRC and SysV names are reserved but validation rejects them until their full
provider contract suites pass. In particular, providers without masking never
approximate `masked` by merely stopping the service.

Service provider capability status:

| Provider | Enabled | Active | Masked | User scope | Linger | Advertised |
| --- | --- | --- | --- | --- | --- | --- |
| systemd | yes | yes | yes | yes | yes | yes |
| OpenRC | contract defined | contract defined | no | no | no | no |
| SysV | contract defined | contract defined | no | no | no | no |

OpenRC and SysV becoming detectable endpoint init facts does not advertise
service enforcement. Their support gate requires both the complete provider
contract suite and a passing real-environment evidence row. Until both exist,
authoring and direct provider construction remain rejected.

### Reboot-required reporting

Successful applicators may report `reboot-required` without authorizing a
reboot. The agent persists that operational requirement in its state directory,
including the source resource address and provider. A later compliant cycle or
agent restart does not erase it, and the generic activation queue never turns
the signal into a reboot command.

The authenticated state-report API exposes this separately from the current
Apply results. The operator CLI shows it with:

```text
remotr endpoint state report <endpoint-id>
reboot_required: true
reboot_sources:
  - address: base/packages/kernel
    provider: apt
```

Configuration compliance can remain `compliant` while a reboot is pending.
Reboot execution requires a separately eligible coordinated reboot resource or
authorized maintenance job.

### Coordinated reboot resources

```yaml
- kind: reboot
  name: kernel-maintenance
  generation: kernel-6.12.1
  onlyIfRequired: true
  delay: 2m
  timeout: 15m
  deadline: 2026-07-13T05:00:00Z
  maintenanceWindow:
    weekdays: [Sunday]
    start: "02:00"
    duration: 2h
  requireACPower: true
  userInhibition: defer
  workloadInhibition: defer
  enforce: true
```

A reboot is a `boot`-risk, intent-only resource. `generation` is the durable
idempotency key and must change to authorize a new reboot. `timeout` is
required and may be at most one hour; `delay` is optional and may be at most
24 hours. `deadline` is RFC 3339. The UTC maintenance window names one or more
weekdays, an `HH:MM` start, and a bounded duration. `onlyIfRequired` keeps the
resource compliant until another successfully applied resource has reported
`reboot-required`.

Both inhibition policies default to `defer`. `userInhibition: defer` blocks
preparation while a logged-in user is visible to systemd-logind, and
`workloadInhibition: defer` honors blocking systemd inhibitors. `ignore` is an
explicit policy choice. `requireACPower` defers a battery-powered endpoint
until external power is available. An unmet window, power, user, workload,
delay, deadline, or connectivity condition never forces the endpoint down.

The agent first persists the intent and reports it during authenticated Sync.
Only a matching explicit server acknowledgement advances the durable attempt
generation, and that state is fsynced before the agent invokes exactly
`systemctl reboot`. After reconnect, the agent reads the kernel boot ID. A
changed ID before the attempt deadline completes the generation; the same ID
remains non-successful and exposes a stable reason such as
`boot_id_unchanged` or `reboot_timeout_same_boot_id`. Completed, failed, and
timed-out generations cannot self-repeat. The endpoint state-report command
shows the phase, generation, attempt generation, boot identities, and stable
reason without including command stderr.

For interactive user services:

```yaml
- kind: service
  name: desktop-agent
  provider: systemd
  scope: user
  service: desktop-agent.service
  users: interactive
  linger: true
  enabled: true
  active: true
  masked: false
```

User-scope services run `systemctl --user` through each selected user's runtime
directory. `linger: true` enables the existing systemd linger behavior before
unit convergence. System and user scopes retain independent enable, active,
and mask checks.

The older systemd-specific kinds remain accepted for compatibility:

```yaml
- kind: systemd
  name: ssh-running
  unit: ssh.service
  enabled: true
  active: true
  masked: false
```

The endpoint must report the `systemd` init backend; otherwise Check returns
`unsupported`. New configuration should use `kind: service`.

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

This legacy form retains its existing `users: interactive` and `linger: true`
semantics. It also shares the corrected independent enable, active, and mask
convergence used by provider-neutral systemd user services.

## Systemd unit resources

Manage a complete system unit without composing a generic file and command:

```yaml
- kind: systemdUnit
  name: telemetry-unit
  lifecycle: present
  unit: telemetry.service
  content: |
    [Unit]
    Description=Telemetry agent
    [Service]
    ExecStart=/usr/local/bin/telemetry
  mode: [420] # 0644
  owner: root
  group: root
```

Set `dropIn` to own one named drop-in instead of the full unit:

```yaml
- kind: systemdUnit
  name: telemetry-limits
  lifecycle: present
  unit: telemetry.service
  dropIn: 20-remotr.conf
  content: |
    [Service]
    TimeoutStartSec=30s
```

Remotr stages content under the system unit search path and runs
`systemd-analyze verify` before atomically replacing the active file. Owner and
group default to `root`; mode defaults to `0644`. A verification failure leaves
the prior file untouched and emits no activation. A successful change emits a
daemon-reload activation signal, which the engine coalesces and executes after
resource application succeeds. Add canonical `notifications` when the same
successful unit change must also reload, try-restart, or restart a service.

With `lifecycle: absent`, omit content and metadata. Remotr removes only the
named unit or drop-in; sibling drop-ins remain untouched. The now-empty drop-in
directory may remain because it is outside the file resource's ownership.

## Firewall resources

Firewall rules default to audit/report mode. Explicit `audit: false` enables
the nftables mutation path only after a complete control-path preflight and a
durable timed rollback are armed. The preflight records all resolved Remotr
destinations, selected routes, resolver/search dependencies, sync protocol and
port, and established control traffic. The next authenticated Sync must
acknowledge the transaction before `rollbackTimeout`; otherwise the prior
encrypted nftables ruleset is restored through protected stdin. Firewalld
remains available for audit/report and provider-contract checks, but enforced
firewalld authoring is rejected until it has an equivalent transactional
restore.

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

Individual rules use `ownership: named` (the default) and the top-level
`action`, `protocol`, `ports`, `sources`, `destinations`, `services`, and
`rule` fields. An owned nftables chain or firewalld zone uses
`ownership: fragment` with named entries under `rules`. Use
`ownership: authoritative` to remove stale entries inside that declared
chain/zone only; authoritative cleanup requires an explicit `cleanupLimit`
between 1 and 1000 and aborts before mutation if the bound would be exceeded.
Unrelated nftables rule identities and firewalld zones are preserved.

```yaml
- kind: firewall
  name: web-ingress
  ownership: authoritative
  audit: false
  backend: nftables
  family: inet
  table: filter
  chain: input
  cleanupLimit: 20
  rollbackTimeout: 2m
  rules:
    - name: https
      action: allow
      protocol: tcp
      ports: [443]
```

The remaining fields are `audit`, `backend`, `table`, `chain`, `family`,
`zones`, and `protectRemotr`. Timed rollback acknowledgement is documented
with the guarded transaction workflow.

## Hosts-entry resources

Use `hostsEntry` to own one marked line in `/etc/hosts` without taking
ownership of the complete file:

```yaml
- kind: hostsEntry
  name: artifact-api
  lifecycle: present
  ownership: named
  address: 203.0.113.10
  canonicalHost: artifacts.example.internal
  aliases: [artifacts, artifact-api]
```

The stable `name` becomes the ownership marker. Apply replaces only lines with
that exact marker, preserves unrelated addresses and comments byte-for-byte,
and uses an atomic same-directory replacement. `lifecycle: absent` removes the
owned line and requires `address`, `canonicalHost`, and `aliases` to be omitted.
Remotr rejects symlinked hosts files and refuses to change an entry naming the
active server destination outside a guarded network transaction.

## DNS resolver and route resources

DNS and routes are separate connectivity-risk resources. Each declares whether
Remotr manages provider configuration, effective runtime state, or both. A
DNS-only resource never owns interface addresses or routes.

```yaml
- kind: dnsResolver
  name: corporate-dns
  provider: network-manager
  interface: eth0
  servers: [192.0.2.53, 2001:db8::53]
  searchDomains: [corp.example]
  configured: true
  effective: true

- kind: route
  name: private-network
  provider: network-manager
  interface: eth0
  destination: 10.20.0.0/16
  gateway: 192.0.2.1
  metric: 50
  table: 254
  configured: true
  effective: true
```

`configured` observes and changes the active NetworkManager connection
profile. `effective` independently observes resolver/device state or the
kernel route table. Reports identify drift per scope. Use `lifecycle: absent`
to remove the declared values from each selected scope. Network resources have
connectivity risk and require explicit enforcement at the engine boundary;
guarded activation is described in the network profile section.

## Network profile providers

Network profiles begin in audit mode. Selectors may combine interface name,
permanent MAC address, and type; every populated field must identify exactly
one interface. Zero or multiple matches block the resource before profile
mutation. `provider` may be `network-manager`, `netplan`, or
`systemd-networkd`; the endpoint must advertise that same configuration owner.
DNS and route resources remain NetworkManager-backed.

```yaml
- kind: networkProfile
  name: office-wifi
  provider: network-manager
  selector:
    permanentMAC: 02:00:00:00:00:0a
    type: wifi
  profileName: office
  profileType: wifi
  autoConnect: true
  mtu: 1500
  ipv4Method: auto
  ipv6Method: ignore
  ssid: corp
  credentialRef: remotr:wifi/office@active
```

Every provider observes persistent configuration separately from effective
device and address state. NetworkManager never requests `--show-secrets`.
Netplan and systemd-networkd reports never include file contents. Reports expose
only safe credential references and reference fingerprints; unexpected secret
fields are discarded at the observation boundary. systemd-networkd supports
Ethernet profiles only. File-backed enforcement rejects credential references
until protected credential material can be applied without entering Git,
process arguments, or reports; those profiles remain available for audit.
Omitted `audit` defaults to `true`. Guarded `audit: false` activation requires
`enforce: true`, a `rollbackTimeout` from `30s` through `15m`, and durable agent
state. NetworkManager creates a native checkpoint. Netplan and systemd-networkd
atomically stage one Remotr-owned file only after encrypting the prior file in
the bounded rollback store; rollback reapplies the prior configuration. The
checkpoint or snapshot remains armed until a subsequent authenticated Sync
acknowledges the exact transaction; expiry restores the prior network state.

```yaml
- kind: networkProfile
  name: guarded-uplink
  provider: network-manager
  selector: {name: eth0}
  profileName: office
  profileType: ethernet
  ipv4Method: auto
  audit: false
  enforce: true
  rollbackTimeout: 2m
```

For file-backed providers, replace `provider` with `netplan` or
`systemd-networkd`. Remotr runs `netplan generate` before apply, or reloads and
reconfigures the selected networkd interface. It never edits unrelated files.

## Certificate resources

`certificate` manages one X.509 leaf/chain and matching private key as a
transactional pair. Git contains provider references only:

```yaml
- kind: certificate
  name: service
  certificatePath: /etc/service/tls.crt
  privateKeyPath: /etc/service/tls.key
  certificateRef: remotr:certificates/service@active
  privateKeyRef: remotr:private-keys/service@7
  chainRefs: [local-file:/run/secrets/service-chain.pem]
  subject: CN=service.example.test
  sans: [service.example.test]
  renewBefore: 720h
  renewalPolicy: provider
  owner: root
  group: service
  certificateMode: [416] # 0640
  privateKeyMode: [384]  # 0600
  notifications:
    - type: reload
      target: service.service
```

Check reports only the leaf SHA-256 fingerprint, subject, and expiry. Apply
resolves material over the protected provider seam, validates the staged
certificate/key match plus subject, SAN, fingerprint, and renewal policy, and
then replaces the two active paths. Private-key modes cannot grant group or
other access. Rollback retains the prior pair only in protected attempt state;
no adjacent plaintext key backup is created. `lifecycle: absent` removes both
managed paths without resolving provider material.

## CA trust-anchor resources

`trustAnchor` owns one named CA certificate and requires its SHA-256
fingerprint before installation:

```yaml
- kind: trustAnchor
  name: corporate-root
  anchorRef: remotr:trust-anchors/corporate-root@7
  fingerprint: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

Debian and Ubuntu install `remotr-<name>.crt` below
`/usr/local/share/ca-certificates` and refresh with
`update-ca-certificates`. Arch installs it below
`/etc/ca-certificates/trust-source/anchors` and refreshes with
`trust extract-compat`. Fingerprint validation completes before the active
anchor changes. Multiple anchor changes in one Apply run produce one
coalesced refresh per backend. `lifecycle: absent` omits `anchorRef` and
`fingerprint`, removes only the named Remotr anchor, and preserves unrelated
trust files.

## AppArmor profile resources

`appArmorProfile` is advertised only for Ubuntu endpoints that report the
AppArmor security backend. It owns one named file below `/etc/apparmor.d` and
one loaded profile identity:

```yaml
- kind: appArmorProfile
  name: service
  profile: usr.bin.service
  mode: enforce
  enforce: true
  content: |
    profile usr.bin.service {
      /usr/bin/service ix,
      /etc/service/** r,
    }
```

`mode` is `enforce`, `complain`, or `disabled`. Every change is written to a
same-directory stage and checked with `apparmor_parser -Q -T` before the
active file or loaded mode changes. Enforce and complain replace the named
profile through `apparmor_parser`; disabled unloads it and owns the matching
disable link. Parser stderr is redacted from diagnostics. AppArmor profiles
are sensitive-risk resources, so Apply also requires the normal explicit
high-risk authorization (`enforce: true`). Debian, Arch, SELinux, and hosts
without the AppArmor backend return unsupported rather than approximating the
policy with filesystem permissions.

## Audit rule resources

`auditRules` owns one `/etc/audit/rules.d/remotr-<name>.rules` fragment:

```yaml
- kind: auditRules
  name: identity
  enforce: true
  rules:
    - -w /etc/passwd -p wa -k identity
    - -a always,exit -F arch=b64 -S execve -k process
```

Rules are argv-free structured lines: embedded newlines and non-rule content
are rejected. Apply stages the named fragment, validates the effective ruleset
with `augenrules --check`, atomically persists it, and uses
`augenrules --load` only when the audit subsystem is mutable. Check reports
the persistent, loaded, and immutable states separately without returning raw
rules. If `auditctl -s` reports immutable mode, valid persistent changes are
kept for boot, live loading is skipped, and the result remains visibly drifted
with `reboot-required`; Remotr does not claim active convergence.

## Account-limit resources

`accountLimit` owns one `/etc/security/limits.d/90-remotr-<name>.conf`
fragment with structured entries:

```yaml
- kind: accountLimit
  name: build
  enforce: true
  entries:
    - {domain: "@build", type: soft, item: nofile, value: "65536"}
    - {domain: "@build", type: hard, item: nproc, value: "4096"}
```

Each entry declares a PAM limits domain, `soft`, `hard`, or `-` type, a known
limit item, and an integer, `unlimited`, or `infinity` value. Duplicate
domain/type/item entries and free-form lines are rejected. Apply atomically
replaces only its named fragment and returns `logout-required`; it does not
terminate existing sessions. `lifecycle: absent` omits entries and removes
only that fragment. Account limits are access-risk resources and therefore
use the normal explicit preflight/authorization gate.

## Login-policy resources

`loginPolicy` owns one named Debian/Ubuntu `pam-auth-update` profile under
`/usr/share/pam-configs`; it never edits generated `/etc/pam.d/common-*` files
as generic text:

```yaml
- kind: loginPolicy
  name: baseline
  provider: pam-auth-update
  enforce: true
  recoveryPrincipals: [recovery]
  rules:
    - section: auth
      control: required
      module: pam_faillock.so
      arguments: [preauth, deny=5, unlock_time=900]
    - {section: account, control: required, module: pam_faillock.so}
```

Rules are structured by PAM section, control, module, and whitespace-free
arguments. Apply first verifies every declared recovery principal and
validates an isolated copy of the complete profile and PAM service trees. It
then atomically changes only the named provider profile and runs exactly
`pam-auth-update --package`; activation failure restores both the prior profile
and generated stack. Access-risk policy remains report-only unless
`enforce: true` is declared. VM evidence verifies technical stack and recovery
operation but does not claim that a human login was tested. `authselect` is
rejected as roadmap-only until an RPM-family provider and its recovery evidence
exist.

## Journald resources

`journald` owns one `/etc/systemd/journald.conf.d/90-remotr-<name>.conf`
drop-in with structured storage, retention, disk, rate, and forwarding policy:

```yaml
- kind: journald
  name: retention
  storage: persistent
  maxRetention: 720h
  systemMaxUseBytes: 1073741824
  runtimeMaxUseBytes: 268435456
  rateLimitInterval: 30s
  rateLimitBurst: 10000
  forwardToSyslog: true
  forwardToKernelBuffer: false
  forwardToConsole: false
  forwardToWall: true
```

Durations use Go duration syntax and must be non-negative; byte and burst
limits are non-negative integers. Boolean fields are pointers in the schema so
an explicit `false` remains managed. Apply copies the complete main config and
drop-in tree into an isolated root, validates it with
`systemd-analyze --root=<stage> cat-config systemd/journald.conf`, atomically
changes only the named drop-in, and emits `restart systemd-journald.service`.
`lifecycle: absent` removes only that drop-in after validating the resulting
effective tree and emits the same activation.

## Logrotate resources

`logrotate` owns one `/etc/logrotate.d/remotr-<name>` fragment with structured
paths, cadence, retention, compression, create metadata, and optional script
hooks:

```yaml
- kind: logrotate
  name: remotr-agent
  paths: [/var/log/remotr/*.log]
  cadence: daily
  retention: 14
  compress: true
  create: {mode: "0640", owner: root, group: adm}
  sharedScripts: true
  preRotate: {command: [/usr/bin/test, -d, /var/log/remotr]}
  postRotate:
    command: [/usr/bin/systemctl, reload, remotr-agent.service]
```

Cadence is `hourly`, `daily`, `weekly`, `monthly`, or `yearly`; retention is
between 0 and 10,000 rotations. Paths must be clean absolute paths and may use
safe glob characters, but cannot contain whitespace, braces, or newlines.
Create modes are quoted octal strings. `preRotate`, `postRotate`,
`firstAction`, and `lastAction` accept bounded argv arrays whose executable is
absolute; Remotr shell-quotes every argument and rejects newlines and NULs.
Script argv must not contain secret material.

Apply copies the complete main config and fragment directory, redirects the
owned `include /etc/logrotate.d` boundary into that isolated tree, and runs
exactly `logrotate --debug <staged-main>`. Failed validation leaves active
state untouched. `lifecycle: absent` validates the effective tree without the
named fragment before removing only that fragment.

## Bootstrap, agent-install, and command resources

The existing `bootstrap`, `agentInstall`, and `command` kinds are available in
canonical `resources` lists with their existing field contracts. Commands use
argv arrays and remain an escape hatch; authors own their idempotency and
rollback behavior. Prefer a typed resource when one exists.

## Default ordering

Absent dependency edges, resources are ordered as packages, ordinary files,
downloads, critical files, users, groups, SSH access resources, sudo fragments,
user files, account limits, login policies, certificates, trust anchors, AppArmor profiles, audit rules, journald policy, logrotate fragments, firewall, hosts entries, DNS, routes, systemd, systemd-user,
bootstrap, agent-install, and commands. Registry ordering is deterministic; `dependsOn` edges take
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
