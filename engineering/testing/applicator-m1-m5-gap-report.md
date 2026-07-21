# Applicator M1-M5 exit-criteria and gap report

- **Audit date:** 2026-07-20
- **Qualification change:** `qualify-ubuntu-2404-applicators`
- **Platform:** Ubuntu 24.04 amd64
- **Machine audit:** `make ubuntu-2404-applicator-qualification-audit`
- **Result:** M1-M5 qualification passes with 44 exact non-package rows,
  10 explicit non-claims, no blocking rows, and all four shared workstreams
  accepted.

This report separates implementation, public composition, and qualified
distribution evidence. A resource kind existing in code or rendering through
the configuration CLI is not by itself an Ubuntu support claim. Only an exact
row emitted as `qualified` by the machine audit is supported.

## Composition coverage

The source-only repository at
`test/config-repos/ubuntu-2404-m1-m5` contains 44 stable representative
resource addresses: one for every non-package row intended for qualification.
The public operator CLI acceptance test proves all of the following:

- `config discover` reports every expected resource and provider requirement;
- `config validate` accepts the schema-1 source repository;
- `config render --fleet qualification-ubuntu` emits every expected stable
  address and preserves accepted fields, ownership, policy, dependencies, and
  guarded activation intent;
- repeated rendering is deterministic; and
- no generated `desired.yaml` or `crons.yaml` file is committed to the source
  repository.

The 10 unadvertised rows have no composed address because they are explicit
non-claims, not hidden omissions. Package, APT signing-key, and APT repository
qualification remains owned by `complete-core-package-providers`.

## Milestone decisions

| Milestone | Qualified | Explicitly unadvertised | Blocking | Decision |
| --- | ---: | ---: | ---: | --- |
| M1 | 2 | 0 | 0 | Complete: `file/posix` and `download/https` have pinned Ubuntu container evidence. |
| M2 | 8 | 0 | 0 | Complete: filesystem and access rows have container or Ubuntu access-recovery VM evidence selected by risk. |
| M3 | 7 | 0 | 0 | Complete: host, kernel, locale/time, mount, and swap rows have exact Ubuntu VM evidence. |
| M4 | 12 | 7 | 0 | Complete for the exact supported set; OpenRC, SysV, legacy systemd contracts, netplan/networkd activation, and firewalld enforcement remain explicit non-claims. |
| M5 | 15 | 0 | 0 | Complete: security, PAM, logging, desktop/session, and three exact browser backends have Ubuntu safety or desktop-session VM evidence. |

Three additional cross-cutting contracts are explicitly unadvertised:
`bootstrap/one-shot`, `agentInstall/binary-install`, and `command/argv`. They
are not typed convergent M1-M5 applicator claims and therefore are not assigned
to a milestone.

## Exact evidence snapshot

`test/qualification/ubuntu-2404-applicators.yaml` contains 54 exact registered
rows:

- **Qualified:** 44.
- **Explicitly unadvertised:** 10.
- **Blocked:** 0.
- **Planned, missing, skipped, failing, or untested:** 0.

`test/provider-matrix.yaml` contains 46 passing Ubuntu 24.04 amd64 rows used by
this release state: the 44 applicator rows plus the externally governed APT
package and repository rows. No broad Ubuntu family row is present.

The measured qualification selectors are:

| Selector | Exact rows |
| --- | ---: |
| `make provider-matrix-containers` | 6 |
| `make provider-matrix-vm-user-safety` | 6 |
| `make provider-matrix-vm-system-safety` | 9 |
| `make provider-matrix-vm-network-recovery` | 7 |
| `make provider-matrix-vm-desktop-session` | 7 |
| `make provider-matrix-vm-kernel-module-safety` | 1 |
| `make provider-matrix-vm-host-locale` | 1 |
| `make provider-matrix-vm-time-sync` | 1 |
| `make provider-matrix-vm-mount` | 1 |
| `make provider-matrix-vm-swap` | 1 |
| `make provider-matrix-vm-systemd-timer` | 1 |
| `make provider-matrix-vm-systemd-unit` | 1 |
| `make provider-matrix-vm-service` | 1 |
| `make provider-matrix-vm-login-policy-safety` | 1 |

The selector counts total 44. VM selectors include the applicable negative,
recovery, idempotence, and second-check behavior; container success never
substitutes for a VM required by access, connectivity, boot, storage,
authentication, kernel, service, or desktop risk.

## Dependency status

The audit requires both the provider-matrix dependency bit and a completed
OpenSpec task checklist for each shared workstream.

| Workstream | Status |
| --- | --- |
| `complete-applicator-execution-contract` | Accepted |
| `complete-capability-compatible-delivery` | Accepted |
| `complete-core-package-providers` | Accepted |
| `establish-testing-and-performance-foundation` | Accepted |

If any dependency becomes unaccepted, its exact name and reason are retained in
the umbrella decision and the audit exits non-zero.

## Deferred behavior and future capability gaps

The qualified rows do not imply Firefox recommended policy, user-scope browser
policy, Edge or other browsers, unknown policy names/types/levels,
authoritative default-application cleanup, firewalld enforcement,
netplan/networkd activation, OpenRC/SysV services, generic commands, or other
future providers.

They also do not claim a CMMC control, a ready-to-import Hub baseline, or
organization-specific values. Missing password aging, MFA/smart-card, GDM,
broader browser catalogs, authoritative repository governance, remote log
forwarding, Chrony, dynamic filesystem posture, storage encryption, TPM/FIPS,
security-tool telemetry, backup/restore, compliance scanning, and evidence
export remain tracked in the
[Ubuntu security-control capability roadmap](../plans/ubuntu-cmmc-capability-roadmap.md).

## Evidence-derived archive decision

The version-1 audit reports:

- all five milestone decisions complete;
- 44 exact supported targets and 10 exact explicit non-claims;
- zero row blockers and zero dependency blockers; and
- `umbrella.eligible: true`.

This makes the Ubuntu qualification child eligible to close and permits the
umbrella task 14.10 to proceed after the remaining exit commands and
documentation validation pass. It does not archive either change and does not
authorize any future-roadmap or compliance claim.
