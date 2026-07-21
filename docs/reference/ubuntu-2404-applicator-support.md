# Ubuntu 24.04 applicator support

This reference lists the exact non-package applicator rows qualified for
Ubuntu 24.04 amd64. It is for configuration authors, operators, and release
reviewers deciding whether a resource/backend combination has complete
distribution evidence.

Support is exact across capability ID, backend, contract revision,
distribution, release, architecture, and evidence environment. A row in this
document does not imply support for a sibling backend, another Ubuntu release,
another architecture, or every field a related operating-system tool can
accept.

## Qualified rows

The release audit reports 44 qualified rows. The checked-in schema-1
qualification repository contains one stable composed address for each row and
passes public `config discover`, `config validate`, and deterministic
`config render` acceptance.

| Milestone | Qualified capability/backend rows | Evidence environment |
| --- | --- | --- |
| M1 (2) | `file/posix`, `download/https` | Pinned Ubuntu container |
| M2 (8) | `directory/posix`, `link/posix`, `group/shadow`, `user/shadow`, `authorizedKey/openssh`, `knownHost/openssh`, `sudo/sudoers`, `userFile/posix` | Container for ordinary POSIX behavior; Ubuntu access-recovery VM for account and access behavior |
| M3 (7) | `sysctl/procfs`, `kernelModule/kmod`, `hostname/systemd-hostnamed`, `hostLocale/systemd-localed`, `timeSync/systemd-timesyncd`, `mount/util-linux`, `swap/util-linux` | Ubuntu system-safety or capability-specific VM |
| M4 (12) | `endpointSchedule/cron`, `endpointSchedule/systemd-timer`, `service/systemd`, `systemdUnit/systemd`, `reboot/systemd`, `hostsEntry/hosts-file`, `dnsResolver/network-manager`, `route/network-manager`, `networkProfile/network-manager`, `firewall/nftables-audit`, `firewall/nftables-enforcement`, `firewall/firewalld-audit` | Container for cron; Ubuntu service, systemd, system-safety, or network-recovery VM for the other rows |
| M5 (15) | `certificate/pem-files`, `trustAnchor/update-ca-certificates`, `appArmorProfile/apparmor-parser`, `auditRules/auditd`, `accountLimit/pam-limits`, `loginPolicy/pam-auth-update`, `journald/systemd-journald`, `logrotate/logrotate`, `desktopSetting/dconf`, `desktopSetting/gsettings`, `sessionPolicy/dconf`, `sessionPolicy/gsettings`, `browserPolicy/chromium`, `browserPolicy/google-chrome`, `browserPolicy/firefox` | Ubuntu system/access safety VM or logged-in/logged-out desktop-session VM |

The exact contract revisions and accepted fields are authoritative in
`test/qualification/ubuntu-2404-applicators.yaml`. The published support claim
is further constrained by the matching passing row in
`test/provider-matrix.yaml` and by verified governing IDs in
`test/traceability.yaml`.

## Evidence selectors

The 44 rows are covered by these complete selectors. The count is the number
of exact qualification rows using the selector; it is not a test count.

| Selector | Rows | What it proves |
| --- | ---: | --- |
| `make provider-matrix-containers` | 6 | Ordinary container-safe provider behavior on pinned Ubuntu 24.04 amd64 |
| `make provider-matrix-vm-user-safety` | 6 | Account, key, sudo, and user-file access recovery |
| `make provider-matrix-vm-system-safety` | 9 | Host, reboot, certificate, security, audit, and logging safety/recovery |
| `make provider-matrix-vm-network-recovery` | 7 | Hosts, DNS, route, profile, and firewall control-path recovery |
| `make provider-matrix-vm-desktop-session` | 7 | Logged-in/logged-out dconf, GSettings, session, and browser behavior |
| `make provider-matrix-vm-kernel-module-safety` | 1 | Kernel-module protection, persistence, and recovery |
| `make provider-matrix-vm-host-locale` | 1 | Host locale convergence |
| `make provider-matrix-vm-time-sync` | 1 | systemd-timesyncd convergence |
| `make provider-matrix-vm-mount` | 1 | Mount safety and recovery |
| `make provider-matrix-vm-swap` | 1 | Swap safety and recovery |
| `make provider-matrix-vm-systemd-timer` | 1 | Durable systemd timer behavior |
| `make provider-matrix-vm-systemd-unit` | 1 | First-class systemd unit behavior |
| `make provider-matrix-vm-service` | 1 | Provider-neutral systemd service state |
| `make provider-matrix-vm-login-policy-safety` | 1 | PAM validation and access recovery |

Containers do not substitute for a VM selector when a row can affect access,
connectivity, boot, storage, authentication, kernel state, system services, or
desktop sessions.

## Explicitly unadvertised rows

These 10 registered rows remain visible in the qualification inventory but do
not advertise Ubuntu support:

| Scope | Unadvertised row | Reason |
| --- | --- | --- |
| M4 | `service/openrc`, `service/sysv` | No qualified Ubuntu provider implementation and recovery evidence |
| M4 | `systemd/systemd-legacy`, `systemdUser/systemd-user-legacy` | Superseded compatibility contracts are not independently qualified |
| M4 | `networkProfile/netplan`, `networkProfile/systemd-networkd` | No safety-equivalent activation/recovery evidence |
| M4 | `firewall/firewalld-enforcement` | Firewalld remains audit-only; enforcement is not advertised |
| Cross-cutting | `bootstrap/one-shot` | One-shot execution is not a typed convergent applicator capability |
| Cross-cutting | `agentInstall/binary-install` | Demand-specific agent upgrade behavior is outside the M1-M5 baseline |
| Cross-cutting | `command/argv` | Generic command execution cannot stand in for a typed provider contract |

Firefox recommended policy, user-scope browser policy, Edge and other browser
providers, unknown browser policy names/types/levels, authoritative default
application cleanup, and broader network/service backends are also not implied
by a qualified sibling. Future CMMC and Hub requirements are tracked in the
[Ubuntu security-control capability roadmap](https://github.com/DavidHoenisch/remotr/blob/master/engineering/plans/ubuntu-cmmc-capability-roadmap.md).

## Verify the checked-in release state

Run:

```console
make ubuntu-2404-applicator-qualification-audit
```

The command emits a versioned JSON report containing every qualified target,
every explicit non-claim, M1-M5 decisions, sibling dependency decisions, exact
blockers, and the umbrella archive decision. It exits non-zero if any required
row is blocked, planned, missing, skipped, failing, or untested; if an M1-M5
inventory is missing; or if a required sibling workstream is not accepted.
