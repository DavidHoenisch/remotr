## Why

Enrolled Ubuntu 24.04 LTS amd64 endpoints cannot receive fleet artifacts that require Remotr's core delivery resources because those exact provider rows are not qualified. The server consequently returns `artifact unavailable` even though endpoint enrollment and configuration composition are valid.

## What Changes

- Qualify `bootstrap-v1`, `command-v1`, and `systemd-v1` through their public provider contracts on a pinned Ubuntu 24.04 LTS amd64 VM.
- Publish only the exact passing Ubuntu 24.04 amd64 provider rows in the production capability catalog.
- Keep other Ubuntu releases, architectures, derivatives, user-scoped systemd, and alternate backends unadvertised.
- Record deterministic capability-document, provider, traceability, and release evidence for the new support boundary.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-provider-conformance`: Add an exact Ubuntu 24.04 LTS amd64 qualification scenario for the three core delivery contracts.

## Impact

- The provider matrix and generated endpoint capability document gain three exact Ubuntu 24.04 amd64 rows.
- Provider VM fixtures gain a pinned Ubuntu 24.04 qualification selector.
- Ubuntu 24.04 support documentation and traceability gain the new OS-LPC-029 evidence boundary.
- A newly released agent can satisfy artifacts requiring these core delivery resources without re-enrollment.
