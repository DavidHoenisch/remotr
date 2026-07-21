## Why

Remotr can manage many Ubuntu primitives but cannot safely attach an endpoint to Ubuntu Pro or converge its entitled services without falling back to an unqualified command resource. Ubuntu 26.04 is now the latest LTS, and Ubuntu-derived distributions can resemble Ubuntu closely enough that a loose family check could run subscription and package mutations on an unsupported host.

## What Changes

- Add a first-class `ubuntuPro` desired-state resource for subscription attachment and declarative, partially owned service lifecycle.
- Consume the enrollment token through an authorized Remotr secret reference and the Ubuntu Pro Client's machine-readable API without placing token bytes in Git, argv, environment variables, logs, reports, or durable endpoint state.
- Fail closed before secret resolution or mutation unless the endpoint proves exact Canonical Ubuntu identity, an explicitly qualified LTS release and architecture, and a compatible Ubuntu Pro Client API.
- Explicitly reject Ubuntu-derived distributions, including Pop!_OS, even when `ID_LIKE`, APT, systemd, package names, or other surface facts resemble Ubuntu.
- Add exact Ubuntu Pro provider qualification rows through Ubuntu 26.04 LTS. Because repeatable tests cannot consume a live Canonical subscription, each release remains unadvertised until its pinned VM identity contract, deterministic mock API contract, negative derivative fixture, secret-canary checks, capability advertisement, and public composition evidence pass.
- Manage every stable Ubuntu Pro service applicable to at least one qualified Ubuntu 20.04-through-26.04 row: `esm-infra`, `esm-apps`, `livepatch`, `usg`, `fips`, `fips-updates`, `realtime-kernel`, `ros`, `ros-updates`, and `anbox-cloud`. Recognize historical `cis`, `cc-eal`, and legacy ESM names so validation can return precise unsupported-release guidance rather than an unknown-field fallback.
- Model service-specific lifecycle options declaratively, including full versus repository-only enablement, qualified variants, and retain-versus-purge disablement. Reject options a selected service or exact platform row does not support.
- Prefer Canonical's versioned Ubuntu Pro API for version, attachment, detachment, service discovery, dependency/incompatibility discovery, enabled state, enable/disable, and reboot status. Non-Python Remotr code SHALL use the documented `pro api` JSON envelope, send parameterized calls as JSON on protected stdin, and never parse normal `pro` command output or construct generic `pro enable`/`pro disable` argument lists.
- Require a durable API or separately reviewed native observation for each service mode and specialized integration. A successful enable response alone does not qualify declarative support; tuples that cannot be distinguished on a later Check remain unadvertised.
- Preflight the complete declared service graph so Canonical's native automatic dependency enablement or incompatible-service disablement can never change an omitted service silently. Block the plan unless every required transition is already satisfied or explicitly owned.
- Treat service disablement and subscription detachment as explicit destructive lifecycle requests; removing the resource stops management and does not implicitly detach the machine.
- Report bounded attachment, contract-validity, entitlement, enabled-service, warning, and reboot-required state without returning subscription names, account data, token material, or unbounded Ubuntu Pro output.
- Keep beta/unknown services unadvertised until independently modeled and qualified. Treat APT News, Pro proxy/client configuration, contract refresh, CVE/USN fixes, unattended-upgrade policy, and generic package upgrades as separate configuration or event capabilities rather than smuggling them into subscription service state.

## Capabilities

### New Capabilities

- `ubuntu-pro-management`: Defines the Ubuntu Pro resource schema, strict platform and client preflight, secret-safe attachment, the stable service catalog and service-specific options, dependency-safe convergence, lifecycle and rollback behavior, and structured fleet-visible state.

### Modified Capabilities

- `linux-provider-conformance`: Requires exact Canonical Ubuntu identity, explicit LTS release qualification through 26.04, derivative-distro rejection, and pinned Ubuntu VM plus deterministic external-boundary evidence before Ubuntu Pro capability advertisement.

## Impact

- Adds a canonical resource model/parser entry, checked-in service contract catalog, field-sensitivity policy, service/option-specific capability requirements, provider registration, applicators, and documentation for `ubuntuPro`.
- Extends endpoint platform facts so exact distribution identity is preserved separately from family compatibility and cannot be inferred from `ID_LIKE`.
- Extends Remotr secret authorization with the `ubuntu-pro-token` purpose and binds resolution to the active artifact, endpoint, fleet, and resource address.
- Integrates with the versioned Ubuntu Pro Client API for version detection, attachment, status, dependency discovery, service enable/disable, reboot status, and detachment through its stable JSON envelope; the provider requires the exact API endpoints and durable observation seam used by each desired service contract.
- Adds exact service/release/architecture/mode/variant provider-matrix rows and isolated Ubuntu LTS VM fixtures through 26.04, plus non-Ubuntu derivative fixtures, deterministic Ubuntu Pro API doubles, incompatible-service cases, fault injection, token canaries, and traceability records. Qualification does not claim that CI consumed a live Canonical subscription or observed entitled package effects.
- No existing desired-state behavior changes. Existing Ubuntu 24.04 capability claims are not generalized to 26.04; only the new provider's independently passing rows are advertised.
