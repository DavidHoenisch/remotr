# Resource kinds

Canonical schema 1 supports 48 typed desired-state resource kinds. Each item
below is valid inside a module configuration's `resources:` list.

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: example
    resources:
      - kind: package
        name: curl
        lifecycle: present
```

The table is an index. Follow each link for fields, examples, provider
limitations, safety behavior, and removal semantics.

## Package and host foundations

| Kind | Default risk | Purpose |
| --- | --- | --- |
| [`package`](configuration-format.md#package-resources) | `normal` | Native, Flatpak, PWA, or Remotr catalog package state. |
| [`aptSigningKey`](configuration-format.md#apt-signing-key-resources) | `normal` | Fingerprinted APT signing key in a dedicated keyring. |
| [`aptRepository`](configuration-format.md#apt-repository-resources) | `normal` | Named APT source, preference, and protected credentials. |
| [`pacmanSigningKey`](configuration-format.md#pacman-signing-key-resources) | `normal` | Fingerprinted provider-native trust identity in the Pacman keyring. |
| [`pacmanRepository`](configuration-format.md#pacman-repository-resources) | `normal` | Named Remotr-owned Pacman repository fragment. |
| [`sysctl`](configuration-format.md#sysctl-resources) | `normal` | Runtime and persistent kernel sysctl state. |
| [`kernelModule`](configuration-format.md#kernel-module-resources) | `boot` | Loaded, persistent, parameter, and blacklist state. |
| [`hostname`](configuration-format.md#hostname-resources) | `normal` | Static and transient hostnames. |
| [`hostLocale`](configuration-format.md#host-locale-resources) | `normal` | Timezone, locale variables, and console keymap. |
| [`timeSync`](configuration-format.md#time-synchronization-resources) | `normal` | Time-sync provider state and server/pool configuration. |
| [`mount`](configuration-format.md#mount-resources) | `boot` | Runtime mount and owned fstab entry. |
| [`swap`](configuration-format.md#swap-resources) | `boot` | Swap file/device activation and persistence. |

## Files, users, and access

| Kind | Default risk | Purpose |
| --- | --- | --- |
| [`file`](configuration-format.md#file-resources) | `normal` | Whole-file or bounded line-oriented content. |
| [`directory`](configuration-format.md#directory-resources) | `normal` | Directory presence, metadata, and bounded cleanup. |
| [`link`](configuration-format.md#link-resources) | `normal` | Symbolic or hard link presence. |
| [`group`](configuration-format.md#group-resources) | `access` | Local group account. |
| [`user`](configuration-format.md#user-resources) | `access` | Local user account and selected attributes. |
| [`authorizedKey`](configuration-format.md#authorized-key-resources) | `access` | Named, fingerprinted SSH authorized-key block. |
| [`knownHost`](configuration-format.md#known-host-resources) | `normal` | Named SSH known-host record. |
| [`sudo`](configuration-format.md#sudo-resources) | `access` | Structured, validated sudoers fragment. |
| [`userFile`](configuration-format.md#interactive-user-file-resources) | `normal` | File content below selected interactive-user homes. |

## Desktop and applications

| Kind | Default risk | Purpose |
| --- | --- | --- |
| [`desktopSetting`](configuration-format.md#typed-desktop-settings) | `normal` | Typed dconf/GSettings value. |
| [`sessionPolicy`](configuration-format.md#structured-session-policy) | `normal` | Lock, idle, proxy, restriction, and MIME policy. |
| [`browserPolicy`](configuration-format.md#managed-browser-policy) | `normal` | Typed Chromium, Chrome, or Firefox managed policy. |
| [`download`](configuration-format.md#download-resources) | `normal` | Verified remote file download to a fixed path. |
| [`agentInstall`](configuration-format.md#agent-install-resources) | `sensitive` | Install and enroll a third-party fleet agent from an artifact. |

## Services, schedules, and orchestration

| Kind | Default risk | Purpose |
| --- | --- | --- |
| [`service`](configuration-format.md#service-resources) | `normal` | Provider-neutral enabled, active, and masked service state. |
| [`systemd`](configuration-format.md#service-resources) | `normal` | Legacy system-scoped systemd unit state. |
| [`systemdUser`](configuration-format.md#service-resources) | `normal` | Legacy interactive-user systemd unit state. |
| [`systemdUnit`](configuration-format.md#systemd-unit-resources) | `normal` | Complete systemd unit or named drop-in content. |
| [`endpointSchedule`](configuration-format.md#endpoint-schedule-resources) | `normal` | OS-native cron or systemd-timer schedule. |
| [`reboot`](configuration-format.md#coordinated-reboot-resources) | `boot` | Generation-bound, guarded reboot intent. |
| [`bootstrap`](configuration-format.md#bootstrap-resources) | `boot` | Ordered one-shot systemd/argv steps behind a path condition. |
| [`command`](configuration-format.md#command-resources) | `destructive` | Explicit check/apply/revert argv escape hatch. |

## Network and name resolution

| Kind | Default risk | Purpose |
| --- | --- | --- |
| [`firewall`](configuration-format.md#firewall-resources) | `connectivity` | Audited or guarded firewall rule/fragment ownership. |
| [`hostsEntry`](configuration-format.md#hosts-entry-resources) | `connectivity` | One marked `/etc/hosts` entry. |
| [`dnsResolver`](configuration-format.md#dns-resolver-and-route-resources) | `connectivity` | Configured and effective resolver state. |
| [`route`](configuration-format.md#dns-resolver-and-route-resources) | `connectivity` | Configured and effective route state. |
| [`networkProfile`](configuration-format.md#network-profile-providers) | `connectivity` | Audited or guarded NetworkManager, Netplan, or networkd profile. |

## Security, policy, and logging

| Kind | Default risk | Purpose |
| --- | --- | --- |
| [`certificate`](configuration-format.md#certificate-resources) | `sensitive` | Transactional certificate/private-key pair from references. |
| [`trustAnchor`](configuration-format.md#ca-trust-anchor-resources) | `sensitive` | Fingerprinted CA certificate in the OS trust store. |
| [`appArmorProfile`](configuration-format.md#apparmor-profile-resources) | `sensitive` | Validated Ubuntu AppArmor profile and mode. |
| [`auditRules`](configuration-format.md#audit-rule-resources) | `sensitive` | Structured Linux audit rules fragment. |
| [`accountLimit`](configuration-format.md#account-limit-resources) | `access` | Structured PAM limits fragment. |
| [`loginPolicy`](configuration-format.md#login-policy-resources) | `access` | Structured Debian/Ubuntu PAM policy profile. |
| [`journald`](configuration-format.md#journald-resources) | `sensitive` | Structured journald retention and forwarding drop-in. |
| [`logrotate`](configuration-format.md#logrotate-resources) | `sensitive` | Structured, validated log rotation fragment. |
| [`ubuntuPro`](configuration-format.md#ubuntu-pro-resources) | `sensitive` | Ubuntu Pro attachment and explicitly declared service lifecycle. |

## Shared fields

Every resource has `kind` and `name`. These metadata fields are parsed across
the canonical vocabulary, but a field is useful only when the selected
provider implements its behavior:

| Field | Meaning |
| --- | --- |
| `lifecycle` | Requested state: `present`, `absent`, `disabled`, or `purged`, restricted per kind. |
| `dependsOn` | Complete resource addresses that must succeed first. |
| `providerOptions` | Provider-namespaced settings. Use only when documented by that provider. |
| `policy` | Resource-level `auto` or `report` remediation policy when supported. |
| `ownership` | `named`, `fragment`, `merge`, or `authoritative` ownership boundary. |
| `validation` | Structured argv validation rules. |
| `notifications` | Post-change activation requests such as reload or restart. |
| `preApplyValidation` | Commands that must exit successfully immediately before Apply. |
| `risk` | Explicit effective risk: `normal`, `sensitive`, `connectivity`, `access`, `boot`, or `destructive`. |
| `authorizationGroup` | Groups related high-risk resources in a review plan. |
| `enforce` | Explicit opt-in required by many non-normal-risk providers. |
| `lockDomains` | Additional agent-local exclusive operation locks. |

Do not add shared fields speculatively. The detailed kind section identifies
which fields actually converge.

## Choosing between similar kinds

- Prefer `service` for new service state; `systemd` and `systemdUser` remain
  compatibility forms.
- Prefer `systemdUnit` over a generic `file` plus daemon-reload command.
- Prefer `endpointSchedule` for a native schedule that works while offline;
  use `kind: crons` for server-dispatched work on agent check-in.
- Prefer typed access, network, policy, and logging resources over `file` or
  `command`; typed providers have validation, bounded ownership, and safer
  recovery behavior.
- Use `command` only when no typed contract exists. You own its idempotent
  check, apply behavior, and recovery.

Run `remotr config validate` after every authoring change. A kind being valid
in the schema does not guarantee that every endpoint advertises its provider
capability; runtime mismatches report `unsupported` and are not applied.
