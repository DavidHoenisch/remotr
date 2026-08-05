## Context

Remotr derives endpoint capability documents from exact passing provider-matrix rows. Ubuntu 24.04 LTS amd64 already supports the command, bootstrap, and systemd implementations, but the matrix contains no release-specific evidence rows, so authenticated Sync cannot match artifacts that require those resources.

This support claim crosses the production capability generator, provider evidence matrix, Vagrant safety fixture, OpenSpec traceability, and public support documentation. The repository requires TDD at a public seam and real VM evidence for provider claims.

## Goals / Non-Goals

**Goals:**

- Qualify the three core delivery contracts on an exact, pinned Ubuntu 24.04 LTS amd64 guest.
- Make the production capability document advertise only those exact passing rows.
- Preserve traceable red, green, provider, cleanup, and release evidence.

**Non-Goals:**

- Generalize the claim to ARM, derivatives, another Ubuntu release, user-scoped systemd, or alternate backends.
- Change enrollment credentials, Sync authentication, artifact composition, or provider semantics.
- Replace typed applicator resources with generic command execution.

## Decisions

- Add exact provider-matrix rows instead of a distribution-family rule. This keeps capability publication evidence-derived and prevents accidental inheritance by other targets.
- Reuse the established Ubuntu 26.04 provider-contract bodies behind release-specific test names. This tests identical public behavior while allowing the Vagrant selector to prove the guest identity independently.
- Pin `cloud-image/ubuntu-24.04@20260705.0.0` and assert `/etc/os-release` plus `dpkg` architecture inside the guest before running provider tests.
- Keep the three compatibility contracts outside the typed M1-M5 audit. OS-LPC-029 and the provider matrix own this separate core-delivery qualification.
- Register this change as a modifier of the canonical OS-LPC prefix so traceability retains one stable capability owner without altering archived changes.

## Risks / Trade-offs

- [A passing VM image can drift if an unpinned box is selected] → Pin the exact box version and assert identity in the guest.
- [Qualification could broaden beyond the tested target] → Use exact matrix keys and negative production-generator assertions for ARM and Ubuntu 22.04.
- [Generic command/bootstrap support can be mistaken for typed policy coverage] → Document these as compatibility and escape-hatch contracts outside the typed M1-M5 inventory.
- [The installed production agent remains blocked after merge] → Stamp and deploy a new release containing the updated embedded provider matrix; re-enrollment is unnecessary.
