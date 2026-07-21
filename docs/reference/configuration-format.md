# Configuration format reference

Remotr deployable desired-state artifacts use strict `schemaVersion: 1` YAML.
Each configuration contains one `resources` list; every resource has an explicit
`kind` and a name that is unique across all kinds in that configuration.

Configuration repositories normally store `kind: module` source files and let
the server compose them. `remotr config render` previews the canonical artifact
without writing `desired.yaml` or `crons.yaml` into the repository.

For a compact list of all 47 kinds, see [Resource kinds](resource-kinds.md).
For the difference between `manifest`, `module`, `application`, and `crons`,
see [Repository file kinds](repository-kinds.md).

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

The schema also parses shared `lifecycle`, `providerOptions`, `policy`,
`ownership`, `validation`, `notifications`, and `authorizationGroup` metadata.
Author them only where the resource-specific section documents convergent
behavior; parsing a shared field is not an advertisement that every provider
enforces it.

## Field exposure classifications

Every accepted resource field has a registry-owned exposure class: `public`,
`sensitive-metadata`, or `secret`. Registration and `config validate` fail if a
strictly decoded field lacks a valid descriptor. Nested lists, maps, provider
options, validation commands, and secret references are covered by the same
rule; collection members do not inherit a public default.

Public fields may retain their typed value. Sensitive metadata is limited to
an approved metadata, fingerprint, count, presence, or omission projection.
Secret fields are limited to reference metadata, presence/count metadata, or
omission—never raw values. This schema policy is the source for safe reports,
diagnostics, persistence, and backup projections; provider code cannot make an
unclassified field safe by calling it “redacted.”

## Provider and capability validation

Use `remotr config discover --fleet <name>` to see canonical resource kinds and
the artifact's capability requirements. `config validate` rejects combinations
that are statically impossible. A local backend mismatch that is knowable only
on an endpoint is reported as `unsupported`, not ordinary drift, and is never
applied.

Current native package targeting is:

| Provider | Qualified target |
| --- | --- |
| `apt` | Debian 12 amd64 and Ubuntu 24.04 amd64 |
| `pacman` | Arch 2026-07-06 amd64 |
| `yay` | Arch 2026-07-06 amd64, using a validated unprivileged build identity |
| omitted | Select APT or Pacman from normalized endpoint facts when an exact qualified row matches |

DNF/RPM-family providers are deferred and canonical validation rejects `dnf`
with a roadmap diagnostic. Flatpak, PWA, and Remotr catalog packages remain
separate providers.

### Ubuntu 24.04 qualification boundary

Ubuntu support is published for exact capability/backend/revision/environment
rows, not for resource families. The qualified platform tuple is Ubuntu 24.04
amd64. A resource being accepted by the schema or a related backend passing
does not make another release, architecture, provider, field, or risk behavior
supported.

See [Ubuntu 24.04 applicator support](ubuntu-2404-applicator-support.md) for
the exact 44 non-package rows, their evidence environments, and the 10
explicit non-claims. Package and APT repository evidence is governed
separately by the exact package rows above. DNF/RPM, APK, Zypper, Snap, and
unlisted releases or architectures are not advertised.
Capabilities needed for future CMMC or Hub content that are not in those exact
rows remain in the
[Ubuntu security-control capability roadmap](https://github.com/DavidHoenisch/remotr/blob/master/engineering/plans/ubuntu-cmmc-capability-roadmap.md).

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
| `packageManager` | Optional `apt`, `pacman`, `yay`, `flatpak`, `pwa`, or `remotr`. `yay` is qualified only for the pinned Arch 2026-07-06 amd64 row; release validation rejects other tuples. `dnf` remains deferred. |
| `arch` | Optional resource-level `x86` or `ARM` filter. |
| `version` | Exact native version for APT/Pacman; required catalog version for `remotr`. |
| `allowUpgrade`, `allowDowngrade` | Explicit native-version transition policy. Downgrades default to denied. |
| `aurBuildUser` | Required only for `yay`; names the validated unprivileged local build identity. Arbitrary commands, PKGBUILD bodies, build flags, and generic `providerOptions` are rejected. |
| `hold` | APT native hold state. Rejected for providers without check/apply support. |
| `refreshCache` | Refresh APT/Pacman metadata before a drifted installation. |
| `removeDependencies` | Use provider-supported dependency cleanup when removing a package. |
| `nonInteractive` | May be omitted or `true`; interactive transactions are rejected. |
| `flatpakRemote`, `flatpakRemoteURL` | Flatpak remote selection; custom remotes require a URL. |
| `pwaURL`, `pwaTitle`, `pwaIcon`, `pwaBrowser`, `pwaUsers` | PWA launcher fields; `pwaURL` is required and `pwaUsers` is `interactive`. |

Package transactions use mandatory provider-aware locks: APT resources share
`package-manager:apt`, while Pacman and AUR resources share
`package-manager:pacman`. Authored `lockDomains` are additive and cannot remove
the native package-manager lock. Lock acquisition is bounded; cancellation,
timeout, and provider-native contention are reported as distinct sanitized
failures. Transactions use sanitized noninteractive environments and report
activation/reboot requirements without rebooting.
Existing schema-0 `present` input remains readable during the compatibility
window; new schema-1 resources must use `lifecycle`.

## APT signing-key resources

`aptSigningKey` owns one keyring below Remotr's APT keyring boundary and
verifies the complete OpenPGP fingerprint before activation:

```yaml
- kind: aptSigningKey
  name: example-vendor
  lifecycle: present
  source: https://packages.example.test/keys/archive.asc
  fingerprint: 0123456789ABCDEF0123456789ABCDEF01234567
```

| Field | Meaning |
| --- | --- |
| `name` | Lowercase keyring identity using letters, digits, `.`, `_`, and `-`. |
| `lifecycle` | `present` (default) or `absent`. |
| `source` | Unauthenticated HTTPS URL; required when present. Embedded URL credentials are rejected. |
| `fingerprint` | Required 40- or 64-hex-character OpenPGP fingerprint. Spaces are ignored and comparison is uppercase-normalized. |

Removal deletes only the named Remotr-owned keyring. A repository that still
uses it should declare a dependency and will fail safely if its key is absent.

## APT repository resources

Declare the key and repository separately so verification and ordering remain
visible:

```yaml
- kind: aptRepository
  name: example-tools
  lifecycle: present
  dependsOn:
    - repositories/example-vendor
  url: https://packages.example.test/debian
  suites: [stable]
  components: [main]
  architectures: [amd64, arm64]
  signingKey: example-vendor
  priority: 600
  credentialRef: remotr:repositories/example-tools@active
```

| Field | Meaning |
| --- | --- |
| `lifecycle` | `present` (default), `disabled`, or `absent`. Disabled retains the source as inactive; absent removes owned source, preference, and auth fragments. |
| `url` | HTTP(S) base URL without credentials, query, or fragment. |
| `suites` | One or more distribution suite tokens. |
| `components` | One or more component tokens. |
| `architectures` | Optional architecture tokens written into the source definition. |
| `signingKey` | Required name of the `aptSigningKey` resource/keyring. |
| `priority` | Optional APT preference from `-10000` through `10000`. |
| `credentialRef` | Optional protected secret reference; credentials never belong in the URL. |

Suites, components, and architectures reject duplicates and whitespace-bearing
tokens. A repository name and signing-key name use the same lowercase safe
name grammar. Put the key's complete resource address in `dependsOn`; the
`signingKey` field itself names the keyring identity.

## Pacman signing-key resources

`pacmanSigningKey` imports and locally signs one verified key inside Pacman's
trust database:

```yaml
- kind: pacmanSigningKey
  name: example-vendor
  lifecycle: present
  source: https://packages.example.test/keys/repository.asc
  fingerprint: 0123456789ABCDEF0123456789ABCDEF01234567
```

| Field | Meaning |
| --- | --- |
| `name` | Lowercase trust identity using letters, digits, `.`, `_`, and `-`. |
| `lifecycle` | `present` (default) or `absent`. |
| `source` | Unauthenticated HTTPS URL without credentials, query, or fragment; required when present. |
| `fingerprint` | Required 40- or 64-hex-character OpenPGP fingerprint. Spaces are ignored and comparison is uppercase-normalized. |

The provider verifies the complete fingerprint before importing and locally
signing the key. It records only Remotr-owned trust identities, and removal is
limited to a key previously marked as owned.

## Pacman repository resources

Declare the signing key and repository separately so trust and ordering remain
explicit:

```yaml
- kind: pacmanRepository
  name: example-tools
  lifecycle: present
  dependsOn:
    - repositories/example-vendor
  servers:
    - https://packages.example.test/arch/$arch
  architecture: auto
  signatureLevel: required
  signingKeys: [example-vendor]
```

| Field | Meaning |
| --- | --- |
| `lifecycle` | `present` (default), `disabled`, or `absent`. Disabled keeps the owned fragment inactive; absent removes it. |
| `servers` | One to 16 HTTP(S) repository URLs without embedded credentials. |
| `architecture` | Required Pacman repository architecture value, such as `auto` or `x86_64`. |
| `signatureLevel` | Required `required` or `required-database-optional`. |
| `signingKeys` | One to 16 `pacmanSigningKey` names required by the repository. |
| `credentialRef` | Reserved protected-secret reference. Omit it for the currently qualified provider; authenticated repository transport is not advertised. |

The provider owns one repository fragment below
`/etc/pacman.d/remotr-repositories` and one marked include boundary in
`/etc/pacman.conf`. It stages and validates changes with `pacman-conf` and
preserves unrelated repositories and configuration. Put every key's complete
resource address in `dependsOn`; `signingKeys` names the Pacman trust
identities. Current support is limited to Arch 2026-07-06 amd64.

## Sysctl resources

```yaml
- kind: sysctl
  name: ipv4-forwarding
  key: net.ipv4.ip_forward
  value: "1"
  runtime: true
  persistent: true
  activation: single-key
```

`key` must be a dotted kernel key and `value` must be one non-empty line. At
least one scope is required:

| Field | Meaning |
| --- | --- |
| `runtime` | Observe and update the live kernel value. |
| `persistent` | Own a named `/etc/sysctl.d` drop-in for boot. |
| `activation` | `single-key` (default), `reload`, or `next-boot`. |

`single-key` updates only the declared live key. `reload` asks the provider to
reload persistent configuration. `next-boot` requires `persistent: true` and
forbids `runtime: true`, so the live kernel is intentionally left alone.

## Hostname resources

```yaml
- kind: hostname
  name: workstation-hostname
  static: workstation-042.example.internal
  transient: workstation-042
```

`static` and `transient` are independently optional, but at least one is
required. Omitted state is unmanaged. Values may contain letters, digits,
dots, and hyphens, cannot begin or end with a dot, and must begin and end with
an alphanumeric character. The provider changes hostname state only; it does
not add `/etc/hosts` entries. Use a separate `hostsEntry` when needed.

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
variables; and `keymap` is a console keymap name. The host provider uses
`timedatectl` for timezone and `localectl` for locale variables, without owning
`/etc/hosts` or an unrelated locale variable. On Ubuntu, it validates keymaps
with `ckbcomp` and atomically updates only `XKBLAYOUT` in
`/etc/default/keyboard`, preserving the file's other settings, mode, and
owner. A locale update reports `logout-required`; a console-keymap update
reports `reboot-required`. These are visible activation signals only — Remotr
does not end sessions or reboot the host as an incidental effect.

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
active. Check distinguishes whether the selected unit is installed and
configured from whether it is active and exposed as the host's effective NTP
service. An absent or masked `systemd-timesyncd` unit, or a provider that cannot
manage requested servers or pools, is unsupported. Combined changes persist
the fragment before changing enablement and restore the prior fragment if
native enablement fails; Remotr never accepts a partial enablement-only result.

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
Options are validated, sorted, and deduplicated before use. Runtime and
persistent additions both check the source, local filesystem support, target
directory, and whether either the source or target overlaps live Remotr state.
When both scopes change, the provider persists the boot declaration before
activation and restores the exact prior fstab content and mode if the native
mount fails. To remove only the boot declaration, use `persistent: false` and
omit `mounted`. Unmounting is normal by default; `unmountMode: lazy` is
explicit, while `unmountMode: force` also requires `enforce: true`
authorization.

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

Swap files are capacity-checked, zero-filled and fsynced, truncated to the
exact declared size, protected to mode `0600`, formatted, and then activated.
Existing files must be regular, non-symlink paths with the same size and mode.
Existing devices use `type: device`, must resolve to a block device, and omit
`sizeBytes`. Active priority and the precisely owned fstab entry are observed
independently. Combined changes persist before activation; activation failure
restores the prior fstab and newly created file state, while failed priority
changes reactivate the previous priority. Disabling active swap requires
`allowRemove: true`; this prevents an accidental lifecycle edit from
exhausting host memory.

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
launcher state under `/var/lib/remotr/schedules`. Launchers and environment
files are owned by the declared user beneath a protected traversable directory;
`overlap: forbid` also receives a pre-created owned lock under
`/run/remotr/schedules`. `argv` and explicit `shell` forms are mutually
exclusive. The launcher preserves argv boundaries, changes to the optional
working directory, resolves environment references outside the world-readable
cron fragment, applies GNU `timeout` when requested, and uses a non-blocking
native lock. Use `lifecycle: disabled` to retain protected launcher state
without an active cron entry, or `absent` to remove only this schedule's owned
fragment and protected files.

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

## Directory resources

Manage one directory without implying ownership of its contents:

```yaml
- kind: directory
  name: telemetry-state
  lifecycle: present
  path: /var/lib/telemetry
  mode: [488] # 0750
  owner: telemetry
  group: telemetry
  allowTypeReplacement: false
```

`path` must be an absolute non-root path. `lifecycle` is `present` or
`absent`; unlike some resource kinds, it must be explicit. `mode` accepts one
decimal integer representing permissions from `0000` through `0777`.
`allowTypeReplacement` must be true before a non-directory object at the path
may be replaced.

Recursive management is bounded and opt-in:

```yaml
- kind: directory
  name: managed-cache
  lifecycle: present
  ownership: authoritative
  path: /var/cache/example
  recursive: true
  purge: true
  crossFilesystem: false
  exclusions: [keep/**, '*.lock']
  maxDepth: 8
  maxEntries: 10000
```

Setting `recursive: true` requires positive `maxDepth` and `maxEntries`.
`purge` additionally requires `ownership: authoritative`. Exclusions are
relative shell-style path patterns and may not be absolute or escape with
`..`. `crossFilesystem: false` keeps traversal within the starting filesystem.
Use conservative bounds and preview on a canary; recursive deletion has a much
larger ownership surface than ordinary directory presence.

## Link resources

Symbolic link:

```yaml
- kind: link
  name: current-config
  lifecycle: present
  path: /etc/example/current
  target: /etc/example/releases/2026-07
  linkType: symbolic
  owner: root
  group: root
```

Hard link:

```yaml
- kind: link
  name: helper-alias
  lifecycle: present
  path: /usr/local/bin/helper-alias
  target: /usr/local/libexec/helper
  linkType: hard
```

`path` must be absolute and non-root. `lifecycle` is `present` or `absent`.
When present, `target` and `linkType` are required. A hard-link target must be
absolute, and `owner`/`group` are rejected because hard links share inode
metadata. Set `allowTypeReplacement: true` only when the provider may replace
an existing object of another filesystem type. With `absent`, only the named
link path is removed.

## Interactive-user file resources

```yaml
- kind: userFile
  name: app-settings
  selector:
    mode: all-interactive
  ownership: authoritative
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

Selector `ownership` defaults to `merge`, which leaves prior managed state in
place when a user leaves the selector. `authoritative` records only users whose
state Remotr actually changed and removes that resource's owned file or resets
its desktop setting when such a user later leaves. Ownership records are
bounded, private agent state; pre-existing matching user state is never
silently claimed.

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
  ownership: authoritative
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
  ownership: merge
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
A successful change reports `logout-required`; Remotr does not log users out
or start their graphical sessions.

`sessionPolicy` supports `merge` and `authoritative` selector ownership for
its dconf/GSettings fields. Default-application cleanup is not safely
portable, so policies managing `defaultApplications` must use merge ownership.

## Managed browser policy

`browserPolicy` writes one supported, typed policy to the browser's native
system managed-policy location. Chromium and Google Chrome accept mandatory
and recommended policy fragments; Firefox currently accepts mandatory policy
in its `policies.json` document. Unsupported policy names, native types, and
levels report `unsupported` and are never written.

```yaml
- kind: browserPolicy
  name: homepage
  browser: chromium
  policyName: HomepageLocation
  scope: system
  level: mandatory
  lifecycle: present
  trustAnchors: [security/corporate-root]
  value:
    type: string
    value: https://intranet.example.test
```

The value model recognizes `boolean`, `string`, `integer`, `double`,
`string-list`, and bounded JSON-compatible `object`, but a value is supported
only when its type matches the selected name in the versioned policy allowlist.
No current allowlisted policy accepts `double`. Unknown names, mismatched or
unknown types, and unsupported levels are not advertised and are never written.
Set `lifecycle: absent` and omit `value` to remove a Remotr-owned policy key.
Chromium-family and Firefox updates preserve unrelated keys in their documents.

Only `chromium`, `google-chrome`, and `firefox` system scope are registered.
Edge and other browsers, user scope, and Firefox recommended policy remain
future-roadmap behavior.

A successful browser policy change reports
`application-restart-required` with the selected browser as its target. This
signal is informational: Remotr does not terminate or restart browser
processes.

`trustAnchors` contains stable `configuration/resource-name` references to
present `trustAnchor` resources. Remotr turns those references into dependency
edges, verifies the system trust anchor first, and never copies certificate or
private-key material into browser or desktop policy JSON. The same field is
available on `sessionPolicy`.

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
as transactional when the agent supplies its protected transaction identity.
The prior fragment is encrypted and armed before activation, survives an agent
restart, and is restored through the central bounded rollback store. A provider
constructed without that transaction identity honestly reports no rollback.

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

When `audit` is omitted or set to `true`, Check always returns `drifted` with
reason code `audit_plan`. Apply may record what the provider would change, but
it does not change the firewall and cannot make the resource compliant. This
persistent drift is intentional: audit evidence must not be reported as proof
that a firewall rule is enforced. Because endpoint compliance requires every
applicable resource to be compliant, one audit-only firewall resource makes
the endpoint report `in_compliance: false`.

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
terminate existing sessions. Before mutation, the Ubuntu provider validates
the complete effective `/etc/security/limits.conf` and lexically ordered
`limits.d/*.conf` tree with the candidate fragment substituted in memory.
Invalid managed or unmanaged syntax therefore fails without arming rollback or
changing active policy. `lifecycle: absent` omits entries and removes only that
fragment. Account limits are access-risk resources and therefore use the normal
explicit preflight/authorization gate.

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
arguments. One profile may contain ordinary `session` rules or
`session-interactive` rules, but not both: `pam-auth-update` cannot represent
the two scopes independently in a single profile. Apply first verifies every
declared recovery principal, resolves every authored module, and validates an
isolated copy of the complete profile and active PAM service trees. Required
missing modules fail before rollback is armed or active state changes; existing
PAM rules that explicitly ignore a missing module or use the native `optional`
control retain their PAM semantics.

The Ubuntu 24.04 provider renders native `Auth-Type`, `Account-Type`,
`Password-Type`, and `Session-Type` headers, atomically changes only the named
provider profile, and runs exactly `pam-auth-update --package`; activation
failure restores both the prior profile and generated stack. Noble supports the
qualified `pam_pwquality.so`, `pam_pwhistory.so`, and `pam_faillock.so` rules but
does not ship `pam_lastlog.so`. A Remotr-authored last-login rule is therefore
rejected before mutation, while Ubuntu's pre-existing optional last-login hook
is preserved. Access-risk policy remains report-only unless `enforce: true` is
declared. VM evidence verifies native PAM authentication and recovery operation
but does not claim a graphical or remote human login. `authselect` is rejected
as roadmap-only until an RPM-family provider and its recovery evidence exist.

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
`systemd-analyze --root=<stage> cat-config systemd/journald.conf`, checks every
staged `.conf` file for NUL bytes, valid `[Journal]` section placement, and
well-formed directives, atomically changes only the named drop-in, and emits
`restart systemd-journald.service`. The additional syntax pass is required
because `cat-config` composes and displays the tree but does not reject every
malformed line. Validation errors identify only the staged file and line,
without echoing policy content.
`lifecycle: absent` removes only that drop-in after validating the resulting
effective tree and emits the same activation.

Ubuntu qualification verifies that the native service remains active after
restart and that a locally submitted record can be retrieved from journald.
The four forwarding booleans manage only journald's local forwarding switches;
they do not define a remote destination, transport, trust policy, queue, retry,
or remote delivery-health contract.

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

Managed fragments always include `missingok`, allowing configuration to be
installed before the producer creates its first matching log file. This is a
fixed provider safety behavior rather than an additional schema field.

Apply copies the complete main config and fragment directory, redirects the
owned `include /etc/logrotate.d` boundary into that isolated tree, and runs
exactly `logrotate --debug <staged-main>`. Failed validation leaves active
state untouched and returns a content-free diagnostic; native stderr is not
echoed because it can contain authored paths or script values. `lifecycle:
absent` validates the effective tree without the named fragment before
removing only that fragment.

Ubuntu qualification additionally forces a real rotation and verifies
compressed output, create ownership and mode, and execution of all four script
phases. The debug pass remains the mutation gate for the complete staged tree.

## Ubuntu Pro resources

`ubuntuPro` declares subscription attachment plus partial ownership of named
Ubuntu Pro services:

```yaml
- kind: ubuntuPro
  name: primary-subscription
  lifecycle: attached
  tokenRef: remotr:ubuntu-pro/production@active
  services:
    - name: esm-apps
      state: enabled
    - name: ros
      state: disabled
      disableMode: retain-packages
```

### Attachment and token lifecycle

`lifecycle` is `attached` or `detached`. An attached resource requires a
selected `tokenRef`; a detached resource forbids both `tokenRef` and services.
Use a versioned Remotr secret reference such as
`remotr:ubuntu-pro/production@active`. Upload and activate it through the
[secret-management workflow](../guides/secret-management.md); do not place the
token itself in a configuration repository.

The token is an enrollment-only credential. Remotr resolves it only after the
endpoint passes exact Ubuntu and client preflight and reports itself
unattached. An already attached endpoint does not resolve the token and is not
detached or reattached when the selected secret version changes. Attachment
uses protected JSON stdin with automatic service enablement disabled. Token
material is excluded from argv, environment variables, temporary files,
effective hashes, plans, logs, reports, rollback records, and retained test
artifacts.

`lifecycle: detached` is an explicit destructive request. Removing the
resource from desired state only stops Remotr management; it does not detach
the endpoint or alter services.

### Service catalog and options

Only services listed in the following table are authorable. `state` is always
`enabled` or `disabled`. Omitted services are outside Remotr ownership and are
never enabled or disabled implicitly.

| Service | Enable modes | Variants | Disable modes | Risk when enabled | Recovery |
| --- | --- | --- | --- | --- | --- |
| `esm-infra` | `full`, `access-only` | — | `retain-packages`, `purge` | sensitive | best-effort control state |
| `esm-apps` | `full`, `access-only` | — | `retain-packages`, `purge` | sensitive | best-effort control state |
| `livepatch` | `full` | — | `retain-packages`, `purge` | sensitive | best-effort control state |
| `usg` | `full`, `access-only` | — | `retain-packages`, `purge` | sensitive | best-effort control state |
| `fips` | `full`, `access-only` | — | `retain-packages`, `purge` | boot | no automatic rollback |
| `fips-updates` | `full`, `access-only` | — | `retain-packages`, `purge` | boot | no automatic rollback |
| `realtime-kernel` | `full`, `access-only` | `intel-iotg`, `raspi` | `retain-packages`, `purge` | boot | no automatic rollback |
| `ros` | `full`, `access-only` | — | `retain-packages`, `purge` | sensitive | best-effort control state |
| `ros-updates` | `full`, `access-only` | — | `retain-packages`, `purge` | sensitive | best-effort control state |
| `anbox-cloud` | `full`, `access-only` | — | `retain-packages`, `purge` | sensitive | best-effort control state |

Omitting `enableMode` selects `full`. Omitting `disableMode` selects
`retain-packages`; `purge` is always destructive. Validation rejects options
that do not belong to the selected service. It also rejects enabled
combinations of Livepatch, FIPS, FIPS Updates, and real-time kernel that the
catalog marks incompatible.

`usg` manages access to Canonical's tooling. It does not apply or report a CIS
or DISA-STIG hardening profile. The historical names `cis`, `cc-eal`,
`esm-infra-legacy`, and `esm-apps-legacy` are not authorable; validation uses
them only to return a specific migration or unsupported-release diagnostic.

### API and convergence boundary

The provider uses only these versioned Ubuntu Pro Client endpoints:

- `u.pro.version.v1`
- `u.pro.status.is_attached.v1`
- `u.pro.status.enabled_services.v1`
- `u.pro.services.dependencies.v1`
- `u.pro.attach.token.full_token_attach.v1`
- `u.pro.services.enable.v1`
- `u.pro.services.disable.v1`
- `u.pro.security.status.reboot_required.v1`
- `u.pro.detach.v1`

Parameterized requests use `/usr/bin/pro api <endpoint> --data -` with bounded
typed JSON on protected stdin. Remotr does not fall back to ordinary `pro`
commands, `--args`, shell execution, or localized command output. A missing or
incompatible endpoint makes the affected tuple unsupported.

Before mutation, Remotr compares Canonical's dependency and incompatibility
graph with declared state. A disabled dependency must already be satisfied or
be explicitly declared enabled. An enabled conflict must be explicitly
declared disabled. Unexpected native changes fail the operation rather than
silently expanding ownership.

Apply is not successful until a second Check observes the declared result. An
enable response alone cannot qualify an option that the status API cannot
distinguish later. Such tuples remain unadvertised even though their syntax is
accepted.

### Reporting, risk, and rollback

Fleet state is limited to bounded attachment state, declared service state,
API-established contract or entitlement outcomes, stable warning codes, last
outcome, rollback class, residual-effects class, and reboot requirement. It
does not contain subscription or account names, contract identifiers, token
material, raw third-party output, or undeclared service details.

Attachment and ordinary enablement are `sensitive`. Disabling a service,
purging packages, and explicit detachment are destructive. FIPS, FIPS Updates,
and real-time-kernel enablement use boot risk. Authors may raise these risk
classes but cannot lower them.

Ordinary attachment and service changes have best-effort control-state
rollback. If a newly attached endpoint cannot converge its declared services,
Remotr restores applicable service state and detaches only the attachment it
created. Explicit detach, purge, FIPS stream replacement, and full real-time
kernel operations have no automatic inverse. Reports distinguish restored
Ubuntu Pro control state from packages, snaps, repositories, kernels, or boot
artifacts that may remain. Remotr reports reboot requirements but never
reboots the endpoint.

### Qualification and supported tuples

Schema admission is not a runtime support claim. Qualification is tracked per
release, architecture, API revision, service, mode, variant, and disable
behavior. The current inventory is:

| Ubuntu release | Architecture | API revision | Base | Services | Default `full` behavior | Variants | Explicit modes / disable behavior |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 20.04 LTS | amd64 | `ubuntu-pro-api-v32` | advertised | advertised | advertised | advertised | unadvertised |
| 22.04 LTS | amd64 | `ubuntu-pro-api-v32` | advertised | advertised | advertised | advertised | unadvertised |
| 24.04 LTS | amd64 | `ubuntu-pro-api-v32` | advertised | advertised | advertised | advertised | unadvertised |
| 26.04 LTS | amd64 | `ubuntu-pro-api-v32` | advertised | advertised | advertised | advertised | unadvertised |

Rows are promoted independently; a passing base attachment row never enables
all services or sibling options. Ubuntu derivatives, interim releases,
non-amd64 architectures, conflicting identity sources, absent clients, and
unknown future releases remain unsupported.

Qualification uses pinned Ubuntu VMs and deterministic API doubles with a
synthetic token. It exercises the public provider seam, exact request and JSON
envelope contracts, service and variant convergence, idempotent second Apply,
secret redaction, reboot signaling, and VM cleanup without consuming a live
Canonical subscription. It therefore does not claim that CI attached a live
subscription or observed entitled package, snap, repository, kernel, or
compliance-tool effects. Explicit mode assertions and package retention versus
purge remain unadvertised until a durable, independently reviewable
observation seam exists. The passing `full` capability represents the omitted
default's protected request behavior, not an assertion that Check can recover
an earlier invocation mode from enabled-service state.

### Out of scope

`ubuntuPro` does not manage Landscape, Ubuntu Pro Client installation, APT
News, proxy settings, refresh timers, telemetry policy, contract replacement,
security fixes, `pro fix`, unattended-upgrade policy, general package upgrades,
hardening execution, or reboot execution. These require separately typed
resources or event workflows.

## Bootstrap, agent-install, and command resources

### Bootstrap resources

`bootstrap` runs ordered steps while exactly one path condition is unmet:

```yaml
- kind: bootstrap
  name: initialize-signature-database
  when:
    pathMissing: /var/lib/example/signatures.db
  steps:
    - systemd:
        unit: example-updater.service
        active: false
    - exec: [/usr/local/sbin/example-update, --initialize]
    - systemd:
        unit: example-updater.service
        enabled: true
        active: true
  enforce: true
```

`when.pathMissing` is drift while the path is absent;
`when.pathExists` is drift while the path exists. Each step is either one
`systemd` object or one `exec` argv array. Steps run in order and stop at the
first error. A systemd step may independently set `enabled` and `active` and
performs daemon reload before the action. Revert is a no-op.

`bootstrap` has default `boot` risk. The provider relies on the path condition
for idempotency and does not validate that the steps make it compliant. Use a
typed resource when possible, and make the condition an observable result of
the steps.

### Agent-install resources

`agentInstall` installs and enrolls a third-party agent tarball. The current
provider is shaped around installers such as Elastic Agent:

```yaml
- kind: agentInstall
  name: elastic-agent
  present: true
  version: 9.1.0
  artifactURL: https://artifacts.example.test/elastic-agent-${version}.tar.gz
  extractDir: elastic-agent-${version}-linux-x86_64
  installBinary: elastic-agent
  fleetURL: https://fleet.example.internal:8220
  enrollmentTokenSecret: file:/run/secrets/elastic-enrollment-token
  runningCheck:
    process: elastic-agent
  enforce: true
```

`${version}` is expanded in `artifactURL` and `extractDir`. The provider uses
`pgrep -f` for `runningCheck.process`, downloads with `curl`, extracts a gzip
tarball, and calls the install binary with fleet URL and token. The token is
read from the legacy `file:/absolute/path` reference expected by this provider
and is not put in desired-state content. `installBinary` defaults to
`elastic-agent`.

Omitted `present` means true. `present: false` is treated as already compliant;
the current provider does not uninstall the third-party agent. Revert is a
no-op, artifact checksum/signature verification is not part of this provider,
and arguments are tailored to the supported install command. This is a
`sensitive`-risk resource; prefer a verified `download`, package, or dedicated
typed provider when those can express the installation.

### Command resources

```yaml
- kind: command
  name: rebuild-example-index
  check: [/usr/bin/test, -f, /var/lib/example/index.ready]
  apply: [/usr/local/sbin/example-index, rebuild]
  revert: [/usr/local/sbin/example-index, restore]
  enforce: true
```

All commands are argv arrays; Remotr passes argument boundaries directly and
does not invoke a shell. A zero exit from `check` means compliant. A non-zero
exit means drift and causes `apply` to run; if drift exists and `apply` is
omitted, Apply fails. `revert` is optional and a missing revert is a no-op.

`command` is a `destructive`-risk escape hatch. Remotr cannot infer its
idempotency, secret handling, ownership boundary, side effects, or rollback
quality. Never put secret bytes in argv. Prefer a typed resource and test both
the check and recovery commands independently before rollout.

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
