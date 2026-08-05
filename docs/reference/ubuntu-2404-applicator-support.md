# Ubuntu 24.04 applicator support

This reference lists the exact M1–M5 baseline non-package applicator rows
qualified for Ubuntu 24.04 amd64. Ubuntu Pro uses its own release-, service-,
mode-, variant-, and disable-behavior qualification inventory described under
[Separately qualified Ubuntu Pro rows](#separately-qualified-ubuntu-pro-rows).
This page is for configuration authors, operators, and release reviewers
deciding whether a resource/backend combination has complete distribution
evidence.

Support is exact across capability ID, backend, contract revision,
distribution, release, architecture, and evidence environment. A row in this
document does not imply support for a sibling backend, another Ubuntu release,
another architecture, or every field a related operating-system tool can
accept.

## Qualified rows

The typed M1-M5 release audit reports 44 qualified rows. The checked-in
schema-1 qualification repository contains one stable composed address for
each row and passes public `config discover`, `config validate`, and
deterministic `config render` acceptance. Three additional core-delivery
contracts are qualified separately below.

| Milestone | Qualified capability/backend rows | Evidence environment |
| --- | --- | --- |
| M1 (2) | `file/posix`, `download/https` | Pinned Ubuntu container |
| M2 (8) | `directory/posix`, `link/posix`, `group/shadow`, `user/shadow`, `authorizedKey/openssh`, `knownHost/openssh`, `sudo/sudoers`, `userFile/posix` | Container for ordinary POSIX behavior; Ubuntu access-recovery VM for account and access behavior |
| M3 (7) | `sysctl/procfs`, `kernelModule/kmod`, `hostname/systemd-hostnamed`, `hostLocale/systemd-localed`, `timeSync/systemd-timesyncd`, `mount/util-linux`, `swap/util-linux` | Ubuntu system-safety or capability-specific VM |
| M4 (12) | `endpointSchedule/cron`, `endpointSchedule/systemd-timer`, `service/systemd`, `systemdUnit/systemd`, `reboot/systemd`, `hostsEntry/hosts-file`, `dnsResolver/network-manager`, `route/network-manager`, `networkProfile/network-manager`, `firewall/nftables-audit`, `firewall/nftables-enforcement`, `firewall/firewalld-audit` | Container for cron; Ubuntu service, systemd, system-safety, or network-recovery VM for the other rows |
| M5 (15) | `certificate/pem-files`, `trustAnchor/update-ca-certificates`, `appArmorProfile/apparmor-parser`, `auditRules/auditd`, `accountLimit/pam-limits`, `loginPolicy/pam-auth-update`, `journald/systemd-journald`, `logrotate/logrotate`, `desktopSetting/dconf`, `desktopSetting/gsettings`, `sessionPolicy/dconf`, `sessionPolicy/gsettings`, `browserPolicy/chromium`, `browserPolicy/google-chrome`, `browserPolicy/firefox` | Ubuntu system/access safety VM or logged-in/logged-out desktop-session VM |

## Separately qualified core delivery rows

Ubuntu 24.04 LTS amd64 also advertises `bootstrap/bootstrap-v1`,
`command/command-v1`, and legacy `systemd/systemd-v1`. These compatibility and
escape-hatch contracts remain outside the typed M1-M5 inventory, but each is
bound to `make provider-matrix-vm-core-delivery-ubuntu-24-04`. The pinned VM
proves drift, Apply, compliant second Check, no-change replay, and cleanup
through the public provider contracts. This exact claim does not cover ARM,
another Ubuntu release, `systemdUser`, or another backend.

The exact typed contract revisions and accepted fields are authoritative in
`test/qualification/ubuntu-2404-applicators.yaml`. Core-delivery revisions and
all published support claims are constrained by matching passing rows in
`test/provider-matrix.yaml` and by verified governing IDs in
`test/traceability.yaml`.

## Separately qualified Ubuntu Pro rows

`ubuntuPro` is qualified independently of the 44 M1–M5 rows because its
support decision includes the Ubuntu Pro API revision and an exact service,
mode, variant, or disable-behavior tuple.

On Ubuntu 24.04 LTS amd64 at `ubuntu-pro-api-v32`, Remotr currently advertises
base attachment, all ten catalog services with default or explicit `full`
behavior, and the `realtime-kernel` variants `intel-iotg` and `raspi`.
`access-only` and all disable behaviors remain unadvertised. Their schema can
validate, but capability-compatible delivery blocks those tuples before token
resolution or mutation.

The authoritative inventory is `test/qualification/ubuntu-pro.yaml`. See the
[Ubuntu Pro resource reference](configuration-format.md#ubuntu-pro-resources)
for the exact support table and [Ubuntu Pro management](../guides/ubuntu-pro-management.md)
for the operator workflow.

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

The separate `make provider-matrix-vm-core-delivery-ubuntu-24-04` selector
qualifies the three core-delivery contracts and is not included in the M1-M5
count of 44.

Containers do not substitute for a VM selector when a row can affect access,
connectivity, boot, storage, authentication, kernel state, system services, or
desktop sessions.

## Remaining explicitly unadvertised rows

The typed M1-M5 inventory retains 10 historical nonqualification records. The
three core-delivery records are superseded by the separate OS-LPC-029 evidence
above; these 7 production contracts remain unadvertised for Ubuntu 24.04:

| Scope | Unadvertised row | Reason |
| --- | --- | --- |
| M4 | `service/openrc`, `service/sysv` | No qualified Ubuntu provider implementation and recovery evidence |
| M4 | `systemdUser/systemd-user-legacy` | The legacy user-service contract is not independently qualified |
| M4 | `networkProfile/netplan`, `networkProfile/systemd-networkd` | No safety-equivalent activation/recovery evidence |
| M4 | `firewall/firewalld-enforcement` | Firewalld remains audit-only; enforcement is not advertised |
| Cross-cutting | `agentInstall/binary-install` | Demand-specific agent upgrade behavior is outside the M1-M5 baseline |

Firefox recommended policy, user-scope browser policy, Edge and other browser
providers, unknown browser policy names/types/levels, authoritative default
application cleanup, and broader network/service backends are also not implied
by a qualified sibling. Future CMMC and Hub requirements are tracked in the
[Ubuntu security-control capability roadmap](https://github.com/DavidHoenisch/remotr/blob/master/engineering/plans/ubuntu-cmmc-capability-roadmap.md).

## Verify the checked-in release state

Run:

```console
make ubuntu-2404-applicator-qualification-audit
make provider-matrix-vm-core-delivery-ubuntu-24-04
```

The command emits a versioned JSON report containing every qualified target,
every explicit non-claim, M1-M5 decisions, sibling dependency decisions, exact
blockers, and the umbrella archive decision. It exits non-zero if any required
row is blocked, planned, missing, skipped, failing, or untested; if an M1-M5
inventory is missing; or if a required sibling workstream is not accepted.
