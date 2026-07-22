## ADDED Requirements

### Requirement: Ubuntu Pro is a typed desired-state resource
The system SHALL provide a canonical schema-1 `ubuntuPro` resource with a stable `name`, required `lifecycle`, provider-neutral `tokenRef`, and an optional unique list of explicitly owned services. `lifecycle` SHALL accept only `attached` or `detached`; service state SHALL accept only `enabled` or `disabled`; and service names and options SHALL be accepted only when declared by the checked-in Ubuntu Pro service catalog. The catalog SHALL initially model `esm-infra`, `esm-apps`, `livepatch`, `usg`, `fips`, `fips-updates`, `realtime-kernel`, `ros`, `ros-updates`, and `anbox-cloud` where an exact provider row is qualified.

#### Scenario: Attached subscription is authored
<!-- verification-id: OS-UPM-001 -->
- **WHEN** an `ubuntuPro` resource declares `lifecycle: attached`, a valid versioned `tokenRef`, and unique supported service states
- **THEN** configuration validation accepts it and canonical rendering preserves the reference without resolving or copying secret material

#### Scenario: Resource contains an unknown service
<!-- verification-id: OS-UPM-002 -->
- **WHEN** an `ubuntuPro` resource names a beta or unknown service, repeats a service, supplies a generic argument list, or supplies an option not declared for that service
- **THEN** configuration validation rejects the resource with a bounded field-specific diagnostic

#### Scenario: Lifecycle fields are inconsistent
<!-- verification-id: OS-UPM-003 -->
- **WHEN** an attached resource omits `tokenRef`, or a detached resource declares `tokenRef` or services
- **THEN** configuration validation rejects the resource before composition or delivery

#### Scenario: Historical service name is authored on the supported platform range
<!-- verification-id: OS-UPM-034 -->
- **WHEN** a resource names `cis`, `cc-eal`, `esm-infra-legacy`, or `esm-apps-legacy` and no exact release row qualifies that service
- **THEN** validation returns a targeted historical-service or release diagnostic rather than silently mapping it to a current service or treating it as an arbitrary unknown value

#### Scenario: Service-specific options are authored
<!-- verification-id: OS-UPM-035 -->
- **WHEN** a service uses a cataloged `enableMode`, `variant`, or `disableMode` supported by its exact capability row
- **THEN** configuration validation accepts the option and rendering preserves its typed value without introducing generic command arguments

### Requirement: Platform eligibility requires exact Canonical Ubuntu identity
The provider SHALL treat distribution family compatibility separately from exact product identity. Before resolving a token or mutating the endpoint, it SHALL require consistent exact `ID=ubuntu` and `VERSION_ID` evidence from the operating-system release sources, exact Ubuntu vendor evidence, an explicitly qualified LTS release and architecture, and the required Ubuntu Pro Client API surface. `ID_LIKE`, APT, systemd, Ubuntu-named packages, or branding SHALL NOT satisfy exact identity.

#### Scenario: Pop OS resembles Ubuntu
<!-- verification-id: OS-UPM-004 -->
- **WHEN** an endpoint reports Pop!_OS or another derived distribution with Ubuntu or Debian lineage and otherwise exposes APT, systemd, and the `pro` executable
- **THEN** Check returns `unsupported` before secret resolution and Apply performs no process or filesystem mutation

#### Scenario: OS release sources disagree
<!-- verification-id: OS-UPM-005 -->
- **WHEN** `/etc/os-release` and `/usr/lib/os-release` disagree about the exact ID or release, or required values are malformed or ambiguous
- **THEN** Check fails closed with an identity-mismatch reason and no secret is requested

#### Scenario: Exact qualified Ubuntu is present
<!-- verification-id: OS-UPM-006 -->
- **WHEN** exact Ubuntu identity, vendor, LTS release, amd64 architecture, and client API evidence match a passing provider row
- **THEN** the endpoint may advertise the Ubuntu Pro resource and provider capabilities for that exact row

### Requirement: Latest-LTS support is explicit and evidence-backed
The initial Ubuntu Pro provider revision SHALL define exact amd64 qualification rows for Ubuntu 20.04, 22.04, 24.04, and 26.04 LTS. The base attachment row and each service/release/architecture/mode/variant/disable row SHALL remain unadvertised until its pinned Ubuntu VM selector, deterministic mock API contract, secret-canary checks where applicable, capability advertisement checks, and public composition evidence pass. Qualification SHALL disclose that no live Canonical token or entitled native package effects were exercised. A passing base attachment row SHALL NOT imply support for every service or option. No cadence-derived, range-derived, or remotely fetched release rule SHALL manufacture support for another release.

#### Scenario: Ubuntu 26.04 evidence passes
<!-- verification-id: OS-UPM-007 -->
- **WHEN** the exact Ubuntu 26.04 LTS amd64 provider row and all required selectors are passing
- **THEN** an eligible endpoint advertises only the passing Ubuntu Pro attachment and service-tuple capabilities without advertising unproven Ubuntu Pro services or unrelated 24.04-only resource capabilities

#### Scenario: A future LTS appears
<!-- verification-id: OS-UPM-008 -->
- **WHEN** an endpoint reports a newer even-year `.04` Ubuntu release with no passing row
- **THEN** it does not advertise Ubuntu Pro support and reports the resource unsupported without resolving its token

#### Scenario: Interim Ubuntu release is targeted
<!-- verification-id: OS-UPM-009 -->
- **WHEN** exact Ubuntu identity reports a non-LTS release
- **THEN** the provider returns `unsupported` and performs no attachment or service mutation

#### Scenario: Attachment is qualified but a requested service tuple is not
<!-- verification-id: OS-UPM-036 -->
- **WHEN** the endpoint advertises the base Ubuntu Pro attachment row but lacks the exact capability for a requested service, mode, variant, or disable behavior
- **THEN** artifact delivery or provider Check reports that tuple unsupported before resolving any service secret or invoking a mutation

### Requirement: The versioned Ubuntu Pro API is the primary integration contract
The provider SHALL use `/usr/bin/pro api` with literal versioned endpoint names and the documented common JSON envelope for version detection, attachment state, enabled services, service dependencies and incompatibilities, attachment, service enablement, service disablement, reboot-required state, and detachment. It SHALL send every parameterized endpoint a bounded typed JSON object through protected stdin using `--data -`; SHALL NOT use `--args`, a shell, ordinary `pro attach`, `pro status`, `pro enable`, or `pro disable`; and SHALL NOT parse localized or human-oriented command output. A missing or incompatible required endpoint SHALL make the exact provider tuple unsupported rather than trigger a legacy-command fallback.

#### Scenario: A parameterized service operation is invoked
<!-- verification-id: OS-UPM-037 -->
- **WHEN** the provider attaches a token or changes a service through `u.pro.attach.token.full_token_attach.v1`, `u.pro.services.enable.v1`, or `u.pro.services.disable.v1`
- **THEN** the exact process boundary is `/usr/bin/pro api <literal-endpoint> --data -`, all parameters are typed JSON on protected stdin, and no parameter or secret is constructed as generic CLI arguments

#### Scenario: Status is observed
<!-- verification-id: OS-UPM-038 -->
- **WHEN** Check determines client version, attachment state, enabled services, dependencies, or reboot-required state
- **THEN** it uses the applicable `u.pro.version.v1`, `u.pro.status.is_attached.v1`, `u.pro.status.enabled_services.v1`, `u.pro.services.dependencies.v1`, or `u.pro.security.status.reboot_required.v1` endpoint and does not invoke or parse ordinary `pro status`

#### Scenario: Common API envelope is invalid
<!-- verification-id: OS-UPM-039 -->
- **WHEN** an API response has an unsupported envelope schema, a missing or duplicate required member, malformed typed attributes, unstable text without a stable code, or a result exceeding configured bounds
- **THEN** the provider fails closed with a bounded probe or operation reason and does not use localized titles as control flow or copy raw output into a report

#### Scenario: Required endpoint is unavailable
<!-- verification-id: OS-UPM-040 -->
- **WHEN** the client version or an exact endpoint call shows that an API required by the resource tuple is unavailable
- **THEN** Check returns `unsupported` before mutation and the provider does not retry through an ordinary Ubuntu Pro command

### Requirement: Attachment consumes a scoped secret without exposure
An attached resource SHALL authorize `tokenRef` only for the authenticated endpoint, fleet or endpoint scope, active artifact digest, exact resource address, and `ubuntu-pro-token` purpose. The provider SHALL resolve it only after platform/client preflight and only when the endpoint is unattached, and SHALL pass the bounded token to the versioned full-token attach API through protected stdin with automatic service enablement disabled. Token bytes SHALL NOT appear in argv, environment variables, temporary files, desired state, effective hashes, plans, audit, logs, reports, errors, rollback records, or retained test artifacts.

#### Scenario: Supported endpoint is unattached
<!-- verification-id: OS-UPM-010 -->
- **WHEN** an eligible unattached endpoint applies an authorized attached resource
- **THEN** it resolves the token with purpose `ubuntu-pro-token` and invokes full-token attachment using protected JSON stdin and `auto_enable_services: false`

#### Scenario: Endpoint is already attached
<!-- verification-id: OS-UPM-011 -->
- **WHEN** an eligible endpoint is already attached to a valid Ubuntu Pro contract
- **THEN** Check and service convergence do not resolve `tokenRef` or replace the existing contract

#### Scenario: Unsupported endpoint carries a valid reference
<!-- verification-id: OS-UPM-012 -->
- **WHEN** a derived, unqualified, or client-incompatible endpoint receives an artifact containing a valid token reference
- **THEN** the provider returns `unsupported` without issuing a secret-resolution request

#### Scenario: Token canary traverses attachment
<!-- verification-id: OS-UPM-013 -->
- **WHEN** attachment succeeds or fails using a synthetic secret canary
- **THEN** the canary is present only in protected resolver material and process stdin and is absent from every safe projection and retained artifact

### Requirement: Attachment state is convergent but does not assert contract identity
The provider SHALL distinguish unattached, attached, a stable API operation failure attributable to an invalid or expired contract, and failed or ambiguous API state using bounded versioned results and stable codes. It SHALL NOT invent contract-validity or entitlement detail that the selected APIs do not expose. `tokenRef` SHALL be an enrollment credential rather than a steady-state contract identifier; changing its selected version SHALL NOT detach or reattach a machine that is attached and whose declared operations remain healthy.

#### Scenario: Valid attachment is already present
<!-- verification-id: OS-UPM-014 -->
- **WHEN** lifecycle is `attached`, `u.pro.status.is_attached.v1` reports attached, and declared service observations return no stable contract-health error
- **THEN** attachment state is compliant and only declared service drift remains eligible for Apply

#### Scenario: Attached contract is expired
<!-- verification-id: OS-UPM-015 -->
- **WHEN** a versioned API returns a stable expired- or invalid-contract error, or required state remains ambiguous
- **THEN** Check returns a typed failure and Apply does not silently detach or replace the contract

#### Scenario: Active token version rotates
<!-- verification-id: OS-UPM-016 -->
- **WHEN** an attached compliant endpoint's `remotr:...@active` token reference resolves to new safe version metadata
- **THEN** the effective desired hash may change but the provider performs no secret resolution, detach, or reattach solely because of that rotation

### Requirement: Declared Ubuntu Pro services converge with partial ownership
For an attached valid contract, the provider SHALL observe and converge each declared catalog service through the versioned Ubuntu Pro APIs. It SHALL enable or disable only declared services, leave omitted services untouched, reject unavailable or unentitled requested enablement without claiming compliance, normalize only checked-in status aliases, and treat a stable API warning state as non-compliant health rather than enabled success.

#### Scenario: Declared services are enabled
<!-- verification-id: OS-UPM-017 -->
- **WHEN** a catalog service is declared enabled, entitled, qualified, and available but currently disabled
- **THEN** Apply enables it and the next Check reports it enabled without another mutation

#### Scenario: Omitted service is enabled externally
<!-- verification-id: OS-UPM-018 -->
- **WHEN** a service absent from the resource is enabled by a cloud image, Canonical policy, or another administrator
- **THEN** Remotr leaves that service unchanged and excludes it from desired-state drift

#### Scenario: Declared service must be disabled
<!-- verification-id: OS-UPM-019 -->
- **WHEN** a declared service has state `disabled` but is enabled
- **THEN** an explicitly authorized destructive Apply disables that service using `retain-packages` by default and does not request native purge

#### Scenario: Service is unavailable or unentitled
<!-- verification-id: OS-UPM-020 -->
- **WHEN** a requested enabled service is unavailable for the platform or not entitled by the attached contract
- **THEN** Check or Apply reports a bounded unsupported/entitlement reason and never reports the service compliant

#### Scenario: Enabled service reports a warning
<!-- verification-id: OS-UPM-021 -->
- **WHEN** the common API envelope or typed result says a declared service is enabled but includes a stable warning or unhealthy state
- **THEN** Check returns a typed failure with a bounded redacted reason rather than treating the service as healthy

#### Scenario: USG status uses the historical CIS alias
<!-- verification-id: OS-UPM-042 -->
- **WHEN** the checked-in client contract reports `cis` as the enabled-service representation for an authored `usg` service
- **THEN** the provider normalizes that documented alias to `usg` without making `cis` independently authorable on an unqualified release

### Requirement: Service options are capability-scoped and declarative
The service catalog SHALL define, per exact release and architecture row, whether a service accepts `enableMode: full|access-only`, which named variants it accepts, and whether `disableMode: retain-packages|purge` is supported. The rendered artifact SHALL require separate capabilities for the selected service and each selected mode, variant, and disable behavior. The provider SHALL reject an option that is syntactically valid but not qualified for the exact endpoint row. A mode or specialized service row SHALL remain unadvertised unless Check can distinguish its compliant state through a stable versioned API field or a separately reviewed provider-native observation; one successful mutation response SHALL NOT be treated as permanent convergence evidence.

#### Scenario: Repository-only access is selected
<!-- verification-id: OS-UPM-043 -->
- **WHEN** `enableMode: access-only` is declared for a service and exact row that supports repository-only access
- **THEN** Apply supplies the typed API field and Check uses that row's qualified observable contract without assuming full package or kernel activation from the enabled-service name alone

#### Scenario: Real-time kernel variant is selected
<!-- verification-id: OS-UPM-044 -->
- **WHEN** a qualified `realtime-kernel` variant such as `intel-iotg` or `raspi` is declared
- **THEN** rendering requires that exact variant capability and Apply sends the variant only in the typed API request

#### Scenario: Option is valid for another release but not this one
<!-- verification-id: OS-UPM-045 -->
- **WHEN** a selected mode, variant, or purge behavior lacks a passing row for the endpoint's exact release and architecture
- **THEN** Check returns `unsupported` before any service mutation and does not downgrade to a default option

#### Scenario: API cannot distinguish a requested mode after Apply
<!-- verification-id: OS-UPM-060 -->
- **WHEN** the enabled-services API omits the requested mode and no separately reviewed native observation can distinguish it on the exact row
- **THEN** Remotr leaves that mode capability unadvertised and rejects the desired tuple rather than treating the last successful enable response as durable compliance

### Requirement: Dependencies and incompatibilities are explicit before mutation
Before changing services, the provider SHALL obtain `u.pro.services.dependencies.v1`, reconcile its stable `depends_on` and `incompatible_with` relations with the checked-in catalog and current state, and construct a complete transition plan. A disabled dependency SHALL be already enabled or explicitly declared enabled. An enabled incompatible service that must change SHALL be explicitly declared disabled. Apply SHALL order the declared transitions deterministically and SHALL verify the enable/disable API response sets against that plan.

#### Scenario: Required dependency is omitted and disabled
<!-- verification-id: OS-UPM-046 -->
- **WHEN** a desired enablement depends on a disabled service that is neither declared enabled nor already satisfied outside Remotr ownership
- **THEN** Check reports a dependency-plan failure and Apply performs no service mutation

#### Scenario: Enabled incompatible service is omitted
<!-- verification-id: OS-UPM-047 -->
- **WHEN** a desired enablement would cause Canonical to disable an enabled incompatible service not explicitly declared disabled
- **THEN** Check blocks the plan and Apply does not allow the native client to change that omitted service

#### Scenario: Native API changes an undeclared related service
<!-- verification-id: OS-UPM-048 -->
- **WHEN** an enable or disable response includes an enabled or disabled service outside the preflight transition plan
- **THEN** Apply fails, performs applicable best-effort restoration, and reports an unexpected side-effect reason without silently claiming ownership

#### Scenario: Complete dependency and conflict plan is declared
<!-- verification-id: OS-UPM-049 -->
- **WHEN** every required dependency and incompatible transition is already satisfied or explicitly represented in desired state
- **THEN** Apply disables declared conflicts, enables declared dependencies, enables targets, and performs remaining disables in deterministic dependency-safe order

### Requirement: High-impact and exceptional services preserve their native semantics
The service catalog SHALL assign explicit risk, lock, activation, rollback, and evidence requirements to FIPS, FIPS Updates, real-time kernel, Livepatch, USG, Anbox Cloud, and ROS. Enabling USG SHALL mean access to Canonical's compliance tooling and SHALL NOT claim that a CIS or DISA-STIG hardening profile was evaluated or applied. Full FIPS/FIPS Updates and real-time-kernel transitions SHALL use their boot/destructive safety contract and SHALL expose any native reboot requirement. The provider SHALL enforce the cataloged mutual exclusions among Livepatch, FIPS, FIPS Updates, and real-time kernel rather than relying on native automatic changes.

#### Scenario: USG tooling is enabled
<!-- verification-id: OS-UPM-050 -->
- **WHEN** the `usg` service reaches its declared enabled state
- **THEN** Remotr reports only service/tooling enablement and does not report CIS or DISA-STIG compliance or apply a hardening profile

#### Scenario: FIPS stream or real-time kernel is enabled
<!-- verification-id: OS-UPM-051 -->
- **WHEN** a full FIPS, FIPS Updates, or real-time-kernel transition is authorized
- **THEN** the plan identifies boot and package effects, Apply uses the exact qualified tuple, and the result reports best-effort or no-automatic-rollback semantics plus any reboot-required signal

#### Scenario: Mutually exclusive boot services are requested enabled
<!-- verification-id: OS-UPM-052 -->
- **WHEN** desired state simultaneously enables an incompatible pair among Livepatch, FIPS, FIPS Updates, and real-time kernel
- **THEN** validation or Check rejects the impossible state before mutation with a field- or service-specific incompatibility reason

#### Scenario: Purge is explicitly selected
<!-- verification-id: OS-UPM-053 -->
- **WHEN** a declared service uses `state: disabled` and qualified `disableMode: purge`
- **THEN** the plan requires destructive authorization, the typed disable API requests purge, and the result makes no transactional package rollback claim

### Requirement: Attachment and service changes recover honestly
Apply SHALL snapshot prior non-secret attachment and managed-service state, use deterministic dependency-safe ordering, run a second Check, and attempt restoration only to the extent promised by each catalog row. If Apply attached an originally unattached machine and later cannot converge the declared state, it SHALL restore applicable service state and detach that new attachment. If the machine was originally attached, it SHALL restore changed managed-service states in reverse order where the row promises best-effort restoration. Reports SHALL distinguish restored Ubuntu Pro control state from native packages, snaps, repositories, kernels, boot artifacts, or compliance tooling that may remain, and SHALL never upgrade a no-automatic-rollback operation into a transactional claim.

#### Scenario: Entitlement fails after new attachment
<!-- verification-id: OS-UPM-022 -->
- **WHEN** full-token attachment succeeds with auto-enable disabled but the new contract cannot enable a declared service
- **THEN** Apply detaches the newly created attachment, proves the endpoint is unattached, and reports the original entitlement failure plus rollback outcome

#### Scenario: Later service operation fails
<!-- verification-id: OS-UPM-023 -->
- **WHEN** one declared service changes and a later declared service fails
- **THEN** rollback restores previously changed managed services in reverse dependency order where their catalog contracts permit and the result identifies any residual native effects without claiming filesystem-level transactional cleanup

#### Scenario: Repeated Apply follows convergence
<!-- verification-id: OS-UPM-024 -->
- **WHEN** attachment and every declared service already match desired state
- **THEN** a second Check is compliant and no secret resolution or mutating Pro API is invoked

#### Scenario: A no-automatic-rollback transition fails
<!-- verification-id: OS-UPM-057 -->
- **WHEN** purge, FIPS stream replacement, full real-time-kernel installation, or explicit detach fails after native effects begin
- **THEN** Apply stops later transitions, rechecks observable state, reports the operation-specific recovery class and residual effects, and does not attempt an unsafe generic inverse

### Requirement: Detachment is explicit and destructive
`lifecycle: detached` SHALL express an explicit subscription detach request, require destructive authorization, and use `u.pro.detach.v1` without resolving a token. Removing an `ubuntuPro` resource SHALL only relinquish Remotr ownership and SHALL NOT detach the machine or disable services. Explicit detach SHALL have no automatic rollback claim.

#### Scenario: Explicit detach is authorized
<!-- verification-id: OS-UPM-025 -->
- **WHEN** lifecycle is `detached`, the machine is attached, and the exact destructive plan is authorized
- **THEN** Apply invokes the detach API and the next Check reports detached

#### Scenario: Resource disappears from desired state
<!-- verification-id: OS-UPM-026 -->
- **WHEN** an attached resource is removed from the composed artifact
- **THEN** Remotr performs no detach or service mutation and retains no claim of ownership

#### Scenario: Detach fails after partial native work
<!-- verification-id: OS-UPM-027 -->
- **WHEN** the native detach API reports failure or ambiguous post-state
- **THEN** Apply reports failure with no automatic rollback claim and a follow-up Check determines the observable attachment state

### Requirement: Ubuntu Pro operations are serialized and bounded
The resource SHALL acquire mandatory Ubuntu Pro and APT package-manager lock domains plus any cataloged service-specific lock domains, execute with a sanitized noninteractive environment, honor cancellation and bounded timeouts, classify a native Pro lock as contention, and parse only bounded machine-readable output. Unknown JSON fields SHALL be tolerated, while missing, malformed, duplicate, or oversized required data SHALL fail closed.

#### Scenario: Another Pro operation holds the native lock
<!-- verification-id: OS-UPM-028 -->
- **WHEN** the Ubuntu Pro Client reports its operation lock is held
- **THEN** Remotr reports bounded contention and performs no competing mutation

#### Scenario: Status output is malformed
<!-- verification-id: OS-UPM-029 -->
- **WHEN** a Pro API returns malformed, ambiguous, or oversized JSON
- **THEN** Check reports probe failure and Apply does not resolve a token or mutate state

#### Scenario: Apply is cancelled
<!-- verification-id: OS-UPM-030 -->
- **WHEN** cancellation or timeout occurs during a native operation
- **THEN** Remotr stops further transitions, performs the applicable bounded recovery check, and reports cancellation or timeout separately from drift

### Requirement: Ubuntu Pro state is visible and redacted
Fleet state SHALL expose bounded attachment and declared-service state, stable entitlement or contract-health outcomes only when an API operation establishes them, warning presence, last outcome, rollback class, residual-effects class, and reboot-required signals. It SHALL omit subscription/account names, contract identifiers, token material, raw third-party output, and undeclared service details.

#### Scenario: Attached endpoint reports state
<!-- verification-id: OS-UPM-031 -->
- **WHEN** an endpoint checks an attached resource
- **THEN** its state report contains only bounded enums and declared service names/states needed to explain compliance

#### Scenario: Pro API emits sensitive or unbounded diagnostics
<!-- verification-id: OS-UPM-032 -->
- **WHEN** stdout or stderr contains contract metadata, token-like data, localized text, or excessive output
- **THEN** Remotr maps it to a stable bounded reason and does not copy the raw output into fleet, audit, plan, rollback, or test evidence

### Requirement: Reboot needs are signaled but never automatic
When attachment, service enablement or disablement, FIPS or kernel activation, or detachment reports that a reboot is required, the provider SHALL confirm the signal through the typed operation result or `u.pro.security.status.reboot_required.v1`, emit the standard structured `reboot-required` activation signal, and SHALL NOT initiate a reboot.

#### Scenario: Ubuntu Pro transition requires reboot
<!-- verification-id: OS-UPM-033 -->
- **WHEN** a successful Ubuntu Pro API result reports `reboot_required: true`
- **THEN** Apply succeeds with a reboot-required signal for separate maintenance-window coordination

### Requirement: Client settings and one-shot maintenance are separate capabilities
The Ubuntu Pro resource SHALL NOT treat APT News, Pro proxy settings, refresh timers, data-collection policy, CVE or USN fixes, `pro fix`, contract refresh, unattended-upgrade policy, general package upgrades, or reboot execution as persistent subscription-service fields. Those behaviors SHALL require separately typed configuration or event capabilities before Remotr may manage them.

#### Scenario: A client setting is placed in the resource
<!-- verification-id: OS-UPM-058 -->
- **WHEN** an author places proxy, APT News, refresh, telemetry, fix, package-upgrade, or reboot-execution fields in an `ubuntuPro` resource
- **THEN** validation rejects the fields with guidance that they are outside subscription and service lifecycle management

#### Scenario: Service enablement exposes a separate maintenance need
<!-- verification-id: OS-UPM-059 -->
- **WHEN** a service is enabled but package upgrades, a hardening profile, a security fix, or a reboot remains necessary
- **THEN** the resource reports only its bounded service and activation state and does not silently execute or claim the separate maintenance action
