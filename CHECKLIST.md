# Remotr implementation and evidence checklist

Current as of 2026-07-21. This checklist summarizes implemented product
boundaries; the linked references are authoritative for exact fields, platform
rows, and release evidence. Run `make test` for the root regression suite and
select higher-risk evidence from the
[testing foundation](https://davidhoenisch.github.io/remotr/testing/foundation-operations/).

## Control plane and delivery

- [x] Four programs: `remotr-server`, `remotr-agent`, `remotr`, and the isolated
  Linux `remotr-desktop` module.
- [x] Pull-only mTLS Sync with endpoint identity derived from the certificate,
  CSR-first enrollment, separate Operator credentials, and Postgres registry.
- [x] GitOps source kinds (`manifest`, `module`, `application`, `crons`) with
  server-side composition, cached artifact variants, endpoint overrides, Git
  webhook/poll sync, and blocked release advancement on composition failure.
- [x] Authenticated, bounded endpoint capability documents and exact artifact
  requirement matching with separate target, offered, active, blocked, and
  unmanaged delivery state.
- [x] Structured, classified compliance and Apply reports; typed sensitivity
  projections prevent secret raw values from entering the report shape.
- [x] Fleet remediation policy, gzip Sync responses, inventory and labels,
  diagnostics, firewall evidence, server-managed crons, and in-band agent
  upgrades.
- [x] Versioned encrypted secrets with explicit active selection, authorized
  endpoint resolution, rotation/recovery, and no Operator plaintext readback.
- [x] Persisted high-risk change requests, approvals, rollout bounds,
  endpoint-specific preflight evidence, short-lived leases, baselines, and
  explicit current enforcement limitations.

See [Architecture](https://davidhoenisch.github.io/remotr/explanation/architecture/),
[HTTP API](https://davidhoenisch.github.io/remotr/reference/http-api/), and
[Capability-compatible delivery](https://davidhoenisch.github.io/remotr/reference/capability-compatible-delivery/).

## Agent execution and Linux providers

- [x] Canonical schema 1 admits 47 resource kinds through one typed resource
  registry, with legacy schema-0 compatibility only where behavior is
  lossless.
- [x] Deterministic dependency ordering, typed Check outcomes, explicit Apply
  eligibility, exact lock domains, ownership boundaries, activation evidence,
  and resource-scoped rollback classes.
- [x] Protected, bounded rollback storage and explicit transactional,
  best-effort, or unavailable recovery evidence.
- [x] Core package support is qualified for APT on Debian 12 and Ubuntu 24.04,
  Pacman on pinned Arch 2026-07-06, and AUR/Yay on that same pinned Arch row.
  DNF/RPM, APK, Zypper, and Snap remain deferred and unadvertised.
- [x] Native APT and Pacman repository/signing-key resources preserve unrelated
  configuration and verify trust material before activation.
- [x] Ubuntu 24.04 has 44 exact qualified non-package resource/provider rows;
  support outside those rows remains an explicit non-claim.
- [x] Connectivity, boot, storage, firewall, identity, locale/time, scheduling,
  desktop-session, and other system-administration resources use provider and
  VM safety/recovery evidence where required.

See [Resource kinds](https://davidhoenisch.github.io/remotr/reference/resource-kinds/),
[Applicator execution contract](https://davidhoenisch.github.io/remotr/reference/applicator-execution/),
[Ubuntu 24.04 support](https://davidhoenisch.github.io/remotr/reference/ubuntu-2404-applicator-support/), and
[Package provider qualification](https://davidhoenisch.github.io/remotr/testing/package-provider-qualification/).

## Operator surfaces

- [x] Admin CLI covers bootstrap, enrollment, endpoint/Fleet management,
  state and cron reports, Git sync, config validation/render/discovery, Hub
  import, secrets, RBAC, audit, diagnostics, upgrades, and change control.
- [x] Remotr Desktop is a native Linux Fleet workspace over purpose-specific
  typed bindings and the existing Admin API; it preserves Git as the only
  desired-state deployment boundary and keeps the CLI as fallback.
- [x] Desktop capability parity is machine-readable and fail-closed: a control
  is shown only when its backend action and authorization behavior exist.
- [x] Tagged release support is limited to the evidenced unsigned
  Linux/amd64 Flatpak. The DEB is an unsigned development artifact; other
  operating systems, architectures, and package formats are not advertised.

See [Use Remotr Desktop](https://davidhoenisch.github.io/remotr/guides/use-remotr-desktop/)
and the [Desktop support reference](https://davidhoenisch.github.io/remotr/reference/remotr-desktop/).

## Verification foundation

- [x] Public seams, OpenSpec verification IDs, and traceability are documented.
- [x] Unit, integration, Godog, fuzz, benchmark, mutation, authenticated load,
  provider-container, Vagrant safety/recovery, clean-checkout, and desktop
  native/package evidence have explicit ownership and invocation guidance.
- [x] CI contracts reject missing documentation navigation targets and require
  the resource reference to cover the registered vocabulary.
- [x] Evidence exceptions are centralized, reviewed, and expiring; passing unit
  coverage is not substituted for provider, safety, mutation, or performance
  evidence.

See [Testing foundation operations](https://davidhoenisch.github.io/remotr/testing/foundation-operations/),
[Public seams](https://davidhoenisch.github.io/remotr/testing/public-seams/), and
[OpenSpec traceability](https://davidhoenisch.github.io/remotr/testing/traceability/).
