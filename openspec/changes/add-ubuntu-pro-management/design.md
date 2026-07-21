## Context

Remotr currently normalizes `/etc/os-release` into a portable distribution and family, advertises resource/provider contracts only when an exact provider-matrix row passes, and resolves `remotr:` secrets for an authenticated endpoint only when the active artifact authorizes the resource address and purpose. Ubuntu Pro is not represented by a typed resource. An operator would have to use `command` or `bootstrap`, neither of which can express subscription state, service entitlement, safe token delivery, or provider qualification.

The existing distro reader maps exact `ID=ubuntu` to Ubuntu and otherwise may use `ID_LIKE=debian` for family compatibility. That behavior is useful for generic APT selection but is not a sufficient identity proof for Ubuntu-only mutation. Pop!_OS and other derivatives must remain family-compatible where appropriate without ever advertising the Ubuntu Pro provider.

Canonical documents Ubuntu Pro as an LTS-only service. Ubuntu 26.04 LTS was released on 2026-04-23 and is the latest LTS at design time. Canonical's versioned Ubuntu Pro Client API provides a stable JSON envelope for non-Python integrations through `pro api`; API version 32 introduced stdin-driven full-token attachment, service dependency discovery, and service enable/disable endpoints. Remotr already has a protected-stdin runner suitable for this boundary. The ordinary `pro enable`, `pro disable`, `pro status`, and human-oriented output formats are not integration contracts for this provider.

This change crosses endpoint facts, capability advertisement, desired-state parsing, secret authorization, process execution, rollback reporting, qualification infrastructure, and fleet-visible state. It therefore follows the existing public applicator, authenticated Sync, provider-contract, system-safety VM, and secret-canary seams.

## Goals / Non-Goals

**Goals:**

- Add a typed, convergent `ubuntuPro` resource that manages attachment and every checked-in stable service contract applicable to at least one qualified Ubuntu 20.04-through-26.04 row.
- Express service-specific enable mode, variant, disable cleanup, and dependency/incompatibility transitions without arbitrary commands or generic provider options.
- Support exact Ubuntu 20.04, 22.04, 24.04, and 26.04 LTS amd64 rows after each row passes its required evidence.
- Reject derivatives and ambiguous identity before resolving a token or invoking a mutating Ubuntu Pro operation.
- Reuse Remotr's server-backed and local-file secret providers with a new, narrowly authorized `ubuntu-pro-token` purpose.
- Use structured Ubuntu Pro Client APIs and protected stdin; never expose a token through authored state, argv, environment variables, temporary files, output, audit, rollback, or diagnostics.
- Preserve pre-attached cloud and manually attached machines and converge only the services explicitly owned by the resource.
- Use Canonical's versioned API endpoints and common JSON envelope wherever an endpoint exists, including JSON stdin for every parameterized call.
- Produce honest structured outcomes for unsupported service tuples, unavailable APIs, invalid contracts, entitlement failures, dependencies, incompatibilities, warnings, partial failure, rollback, residual packages, and reboot requirements.

**Non-Goals:**

- Broadening the existing Ubuntu 24.04 qualification claim for unrelated resource kinds. Every capability remains independently release-qualified.
- Supporting interim Ubuntu releases, Ubuntu derivatives, Ubuntu Core, containers, WSL, non-amd64 architectures, or LTS releases older than 20.04 in the first provider revision.
- Managing Canonical account creation, subscription purchase, token allocation/usage quotas, contract replacement, or proving that an already attached machine used the authored token.
- Automatically installing or upgrading `ubuntu-pro-client`; an absent or incompatible client is an unsupported provider boundary.
- Supporting beta or unknown services, historical `cc-eal`/`cis`/legacy-ESM releases outside the 20.04-through-26.04 platform boundary, or service tuples without exact passing evidence.
- Applying USG/CIS hardening profiles merely because the corresponding repository/tooling service is enabled.
- Managing Ubuntu Pro Client settings such as APT News, proxy configuration, refresh timers, or data collection policy; these are future typed client-configuration capabilities.
- Treating `pro fix`, CVE/USN remediation, contract refresh, package upgrades, or reboot as persistent service state; those remain explicit event/scheduling workflows.
- Air-gapped contract provisioning, generic passthrough arguments, arbitrary beta flags, or automatically rebooting an endpoint.
- Landscape enrollment or lifecycle management. It is outside this change rather than an unadvertised future row.

## Decisions

### 1. Preserve exact OS identity separately from distro family

`facts.Facts` will preserve the normalized raw `ID`, `VERSION_ID`, `ID_LIKE`, and the source consistency needed for an exact identity decision. Generic provider selection may continue to use the normalized distro family, but Ubuntu-only capability generation will require all of the following:

1. `/etc/os-release` resolves to `ID=ubuntu` and an exact supported `VERSION_ID`.
2. `/usr/lib/os-release`, when present, also reports the same `ID=ubuntu` and `VERSION_ID`; disagreement or malformed/duplicate required keys is ambiguous and fails closed.
3. `dpkg-vendor --query Vendor` returns exactly `Ubuntu` under the sanitized process environment.
4. The architecture and release match a passing Ubuntu Pro provider-matrix row.
5. `/usr/bin/pro` exposes every required API endpoint at the qualified revision.

`ID_LIKE`, the presence of APT/systemd, Ubuntu-named packages, kernel strings, and branding files are never accepted as Ubuntu identity. Pop!_OS fixtures must prove that a Debian/Ubuntu-like family remains useful for generic classification while `resource:ubuntu-pro` and `provider:subscription/ubuntu-pro-client` are absent.

This is defense against accidental derivative classification, not attestation against a malicious root user who can rewrite operating-system files and binaries.

Alternative considered: rely on the current `Distro == Ubuntu` value. That catches normal Pop!_OS today but leaves the security boundary implicit and does not verify conflicting vendor or on-disk identity at Check/Apply time.

Alternative considered: delegate identity entirely to the `pro` client. Remotr would then resolve a bearer token and enter a third-party mutating path before its own support boundary was established.

### 2. Support a frozen release allowlist, not a moving “latest LTS” rule

The initial qualification inventory contains Ubuntu 20.04, 22.04, 24.04, and 26.04 LTS on amd64. A future Ubuntu release is unsupported until a new pinned matrix row and its required VM evidence pass. Release metadata is not fetched from the Internet during validation or sync.

The Ubuntu Pro resource can therefore be advertised on 26.04 without implying that existing 24.04-only filesystem, account, network, security, or desktop rows work there. Capability-compatible delivery continues to require the complete artifact's requirement set.

Alternative considered: accept any even-year `.04` release at or below the current year. Naming cadence is not evidence that the client, service, or Remotr provider contract works.

### 3. Use a checked-in service contract catalog and partial ownership

The schema-1 shape is:

```yaml
- kind: ubuntuPro
  name: primary-subscription
  lifecycle: attached
  tokenRef: remotr:ubuntu-pro/production@active
  services:
    - name: esm-infra
      state: enabled
      enableMode: full
    - name: esm-apps
      state: enabled
    - name: livepatch
      state: enabled
    - name: realtime-kernel
      state: disabled
      disableMode: retain-packages
  enforce: true
  authorizationGroup: ubuntu-pro-production
```

`lifecycle` is required and accepts `attached` or `detached`. `tokenRef` is required for `attached` so an unattached endpoint can converge, and forbidden for `detached`. `services` is an optional unique list and is valid only with `attached`. Every entry requires `name` and `state: enabled|disabled`, and may use only the service-specific options declared by the checked-in catalog:

- `enableMode: full|access-only` only for catalog rows where Canonical supports repository-only enablement.
- `variant` only for a cataloged variant such as a qualified `realtime-kernel` target.
- `disableMode: retain-packages|purge` only where the native service supports purge. Retention is the default; purge is always destructive and has no transactional package rollback claim.

The initial stable catalog recognizes `esm-infra`, `esm-apps`, `livepatch`, `usg`, `fips`, `fips-updates`, `realtime-kernel`, `ros`, `ros-updates`, and `anbox-cloud`. It also recognizes `cis`, `cc-eal`, `esm-infra-legacy`, and `esm-apps-legacy` solely to return precise historical-release diagnostics; none receives a passing row inside the initial platform boundary unless independently proven applicable. New names discovered from a client update do not become authorable automatically.

The catalog records the canonical service identifier, any status aliases (notably the client's `usg`/`cis` representation), permitted modes and variants, disable/purge behavior, required API endpoints, ongoing observation contract, operation risk, lock domains, activation signals, and the exact provider-matrix row identities. Authored service choices derive separate requirements such as `provider:ubuntu-pro-service/fips-updates`, `provider:ubuntu-pro-option/realtime-kernel/access-only`, and `provider:ubuntu-pro-variant/realtime-kernel/intel-iotg`; the base `resource:ubuntu-pro` capability alone cannot satisfy them.

Only listed services are owned. An omitted service is observed for safe summary purposes but is never enabled or disabled. Removing the entire resource relinquishes ownership and performs no detach or service mutation. This prevents a Git deletion or module-selection change from silently consuming a subscription transition.

The resource manages attachment state, not subscription identity. If the machine is already attached to a valid contract, `tokenRef` is not resolved and a token rotation does not force detach/reattach. Contract replacement requires a separately reviewed explicit detach followed by attach.

Alternative considered: accept any service name returned by the installed client. That would allow an SRU or beta entitlement to expand Remotr's mutation surface without a schema, risk, compatibility, or VM-evidence review.

Alternative considered: make `services` an authoritative complete set. Canonical can add services, cloud images can enable services independently, and disabling every unspecified service would exceed the resource's declared ownership.

### 4. Classify risk dynamically and serialize the complete native transaction

An attached resource that only enables ordinary repository/tooling services defaults to `sensitive` because it resolves a credential and changes security-update sources. Full FIPS/FIPS Updates or real-time-kernel installation defaults to the catalog's boot/destructive safety class and requires the corresponding VM recovery evidence. Any service disablement, package purge, or `lifecycle: detached` defaults to `destructive`. Authors cannot lower the maximum computed risk with a `risk` override.

The provider requires the mandatory `ubuntu-pro` and `package-manager:apt` lock domains in addition to authored locks. Service catalog rows add `package-manager:snap` or boot domains when applicable. It uses bounded, cancellable operations and recognizes the native Pro lock as contention rather than drift. The high-risk plan describes attachment, dependency/incompatibility transitions, per-service modes/variants, possible package/repository/kernel effects, purge intent, rollback class, and possible reboot without including contract or token data.

### 5. Use the versioned Pro API as the integration boundary

Remotr is written in Go, so it uses Canonical's documented non-Python integration boundary: `/usr/bin/pro api` with literal endpoint names and the common bounded JSON envelope. It does not import private Python modules. It uses:

- `u.pro.version.v1`
- `u.pro.status.is_attached.v1`
- `u.pro.status.enabled_services.v1`
- `u.pro.services.dependencies.v1`
- `u.pro.attach.token.full_token_attach.v1` with `--data -`
- `u.pro.services.enable.v1`
- `u.pro.services.disable.v1`
- `u.pro.security.status.reboot_required.v1`
- `u.pro.detach.v1`

Every endpoint with parameters is called with `--data -`, and its bounded typed JSON object is passed through `executil.InputRunner.RunInput`. Remotr never uses `--args`, ordinary `pro attach TOKEN`, ordinary `pro enable`/`disable`, shell interpolation, or generic pass-through argv. Attachment input contains the resolved token and `auto_enable_services: false`. The process receives a sanitized environment. The provider zeroes token and request buffers on every return path as a defense-in-depth lifetime reduction; Go does not promise that all compiler/runtime copies can be erased.

The adapter validates the common API envelope's schema version, `result`, typed attributes, stable error/warning codes, version, and bounds. Human/localized titles are never used as control flow or copied into reports. Check uses only the versioned status APIs for attachment and enabled-state convergence. Entitlement is recorded when a versioned operation returns a stable entitlement result/error; the provider does not invoke or parse ordinary `pro status` merely to populate an entitlement column. Unknown JSON fields are tolerated; missing or invalid required fields are probe failures.

The enabled-services API exposes service name and variant, but it does not expose `access_only`, and not every specialized integration has equivalent state in that endpoint. Therefore a successful enable response is transition evidence, not durable compliance evidence. An access-only or specialized-service row is advertised only when a later Check can distinguish its desired state through a stable versioned API field or a separately reviewed provider-native observation. Otherwise the catalog can recognize the tuple and explain it, but capability generation leaves it unsupported.

Runtime client eligibility uses the documented `u.pro.version.v1` response plus Debian version comparison against the minimum required endpoint release. Exact endpoint calls remain the final feature check. An endpoint error indicating a missing API is `unsupported`, not permission to fall back to a legacy command.

Alternative considered: `pro attach TOKEN` or `pro status --simulate-with-token TOKEN`. Both place the bearer token in argv. An attach-config temporary file avoids argv exposure but creates a durable-filesystem cleanup and crash-recovery burden that stdin avoids.

Alternative considered: call normal `pro enable --format=json`. Although machine-readable, it still couples Remotr to command-specific argument parsing and bypasses the versioned endpoint contract, dependency object, and common API envelope.

### 6. Preflight precedes token resolution and every mutation

Check and Apply both rerun the exact identity, release, architecture, vendor, client/API, executable-path, service-catalog, option-capability, dependency-graph, and status-shape checks. Static configuration validation rejects impossible lifecycle/service/option combinations. Until these gates pass, the provider must not request any secret, attach, detach, or mutate a service.

Before mutation, the provider obtains `u.pro.services.dependencies.v1` and reconciles it with the checked-in catalog and current enabled state. Every disabled dependency needed by a desired enablement must either be declared enabled or already enabled outside Remotr ownership. Every enabled incompatible service that must be disabled must be explicitly declared disabled in the same resource and covered by the plan/authorization. Otherwise Apply is blocked. Transitions are ordered as explicit incompatible disables, dependency enables, then target enables; disables that are not prerequisites run afterward in reverse dependency order.

Canonical's enable API can itself enable dependencies and disable incompatible services. Remotr verifies the returned `enabled` and `disabled` sets against the preflight plan. An undeclared native side effect is a failure followed by applicable best-effort restoration; it never silently expands ownership.

On an unattached valid endpoint, Remotr can determine platform/service capability but cannot safely pre-prove token entitlement because Canonical's simulation CLI accepts the token in argv and no equivalent versioned endpoint is available. Apply therefore attaches with automatic services disabled, re-reads structured API state, and only then converges declared services. An invalid token leaves the endpoint unattached. If the new contract lacks a requested entitlement, Remotr detaches the attachment it just created and reports a bounded failure.

An attached but expired/invalid contract is `check_failed` with a stable reason; the provider does not silently replace it because an authored token does not identify the current contract and replacement consumes a subscription transition.

### 7. Make convergence transactional where native behavior permits and honest elsewhere

Before Apply, the provider records only non-secret prior attachment and managed-service states in centralized encrypted rollback storage. It does not store token bytes or a plaintext copy of Ubuntu Pro state.

For an originally attached endpoint, a later failure restores changed managed-service states in reverse order. For an originally unattached endpoint, a later failure first restores service state as possible and then detaches the newly created attachment. Post-rollback Check must prove the prior attachment and managed-service state.

Ubuntu Pro service operations can install packages, snaps, repositories, kernels, or compliance tooling that disablement does not necessarily remove. The provider therefore declares best-effort rollback for ordinary attachment/service convergence and no automatic rollback for explicit detach, purge, FIPS stream replacement, or full real-time-kernel installation unless that exact row proves a stronger contract. Reports distinguish restored control state from unremoved native artifacts and never claim transactional filesystem rollback.

Successful enablement that reports a reboot need emits `reboot-required`; Remotr does not reboot automatically. Successful detachment reports the API's reboot requirement in the same way.

### 8. Extend existing secret authorization rather than creating a new store

`tokenRef` is classified as `secret` with reference-only safe projection. Artifact authorization recognizes it only at the exact `ubuntuPro` resource address with purpose `ubuntu-pro-token`. Effective desired-state hashing substitutes only provider/version/activation/fingerprint metadata. A local-file token must pass existing root-owned, non-symlink, non-group/world-readable checks.

Resolution is deferred until an unattached, supported endpoint is ready to attach. Already attached endpoints and all unsupported/failed preflights perform zero secret-resolution requests. Token bytes and token-bearing stdin are excluded from mock-call rendering, errors, plan descriptors, state summaries, audit payloads, rollback payloads, and retained VM artifacts.

### 9. Advertise only after pinned release and deterministic boundary evidence passes

Each release row uses a disposable pinned Ubuntu LTS amd64 VM, not a container, so exact identity, release behavior, public provider composition, boot/recovery mechanics, and cleanup are exercised on the supported operating system. Tests do not consume a live Canonical subscription. The fixture uses independently specified deterministic API responses and a synthetic token supplied through the existing secret boundary, serializes execution, and verifies provider detach/recovery behavior plus VM destruction during cleanup.

Qualification is granular. The base attachment row does not advertise every service. Each service/release/architecture/mode/variant/disable behavior receives its own capability identity and required evidence environment. Ordinary repository services may share a VM selector, while FIPS/FIPS Updates, real-time-kernel variants, Livepatch, purge behavior, and Anbox Cloud use behavior-specific fixtures for their control-flow, boot/recovery, and residual-effect reporting contracts. These fixtures do not claim to observe entitled Canonical package, snap, repository, kernel, or compliance-tool effects.

Each applicable row proves public Check, Apply, second Check, idempotence, explicit disable, supported modes/variants, dependency and incompatibility planning, reboot reporting, fault recovery, and explicit detach interaction against deterministic external-boundary fixtures. Cross-row negative fixtures cover Pop!_OS, another Ubuntu-derived `ID_LIKE` identity, an interim Ubuntu release, conflicting `/etc` and `/usr/lib` identity, missing/incompatible client APIs, unknown/beta services, unsupported options, invalid/expired synthetic tokens, unentitled services, unexpected native side effects, native lock contention, cancellation, timeouts, malformed/oversized API envelopes, network loss, and secret canary absence from every retained artifact.

Rows remain `untested` and the capability remains unadvertised until all required selectors pass. A checked-in composition fixture proves `config discover`, `config validate`, deterministic `config render`, capability requirements, and secret-reference preservation without generated desired artifacts.

## Risks / Trade-offs

- [Canonical API behavior differs across SRU versions] → Require runtime version/endpoint checks and exact service-tuple VM evidence, pin the required API surface, and fail closed rather than falling back to normal CLI commands.
- [A token is valid but lacks a requested entitlement] → Attach with auto-enable disabled, inspect structured state, detach a newly created attachment on failure, and leave a previously attached contract untouched.
- [Service enablement has broad package or boot effects] → Classify every service/option in the catalog, require exact high-risk authorization and behavior-specific VM evidence, report reboot requirements, and never auto-reboot.
- [Canonical automatically changes dependencies or incompatible services] → Reconcile the versioned dependency API before Apply, require every necessary transition to be explicitly owned, and reject unexpected enabled/disabled response members.
- [The API can invoke a mode it cannot distinguish later] → Require an independently observable post-state for every advertised tuple and keep invocation-only modes unadvertised.
- [Best-effort rollback leaves installed packages or snaps] → Restore declared Pro control state, report residual artifacts explicitly, and do not use a transactional rollback label.
- [A cloned or pre-attached image belongs to an unexpected contract] → Treat token as bootstrap-only and report validity without asserting contract identity; require explicit detach/attach for replacement.
- [Mocked API qualification can diverge from Canonical's live service] → Pin independently specified API envelopes, run the exact public provider seam on each supported Ubuntu VM, fail closed on unknown responses, and document that live subscription and entitled native effects are not exercised.
- [Raw OS identity is locally forgeable by root] → Document the non-adversarial trust boundary; Remotr prevents accidental derivative targeting but does not claim remote attestation.
- [Ubuntu 26.04 support could be mistaken for general Remotr support] → Publish a resource-specific support table and retain exact per-capability delivery requirements.

## Migration Plan

1. Add exact identity facts and derivative regression tests without changing existing capability advertisement.
2. Add the resource schema, field policies, secret purpose, strict validation, canonical rendering, and capability requirements while keeping all Ubuntu Pro matrix rows unadvertised.
3. Add the checked-in stable service catalog and derive service/mode/variant capability requirements while every new row remains unadvertised.
4. Implement the API adapter and each provider contract test-first with fake OS/clock/network/process boundaries and secret canaries.
5. Add pinned Pop!_OS/derivative negative fixtures and Ubuntu 20.04, 22.04, 24.04, and 26.04 behavior-specific VM fixtures.
6. Promote one attachment or service-tuple provider-matrix row at a time only after its complete selector passes; verify mixed-fleet capability blocking before release.
7. Add reference documentation and a sample config repository, then run traceability, mutation, provider, public composition, and broader test gates.

Rollback of the Remotr release de-advertises the Ubuntu Pro rows so new artifacts are not delivered. Endpoints retain their last successfully applied Ubuntu Pro state; release rollback never detaches subscriptions or disables services. Operators can author an explicit, authorized resource transition if endpoint state must change.

## Open Questions

None. Expanding architectures, releases older than 20.04, beta/unknown services, contract identity pinning, client installation, Pro Client configuration, security-fix events, or air-gapped provisioning requires a later independently qualified change.
