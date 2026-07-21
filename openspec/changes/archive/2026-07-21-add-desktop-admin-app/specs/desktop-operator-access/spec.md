## ADDED Requirements

### Requirement: Dedicated desktop application artifact
Remotr SHALL build the desktop application as a standalone `remotr-desktop` executable and release artifact whose dependency graph, frontend build, runtime entrypoint, and packaging are distinct from the `remotr` Admin CLI.

<!-- verification-id: OS-DOA-001 -->
#### Scenario: Desktop build leaves the Admin CLI unchanged
- **WHEN** a contributor builds the desktop application from a clean checkout
- **THEN** the build produces the desktop executable without changing the `remotr` command tree, flags, output, or binary entrypoint

<!-- verification-id: OS-DOA-002 -->
#### Scenario: Desktop launch opens embedded application assets
- **WHEN** an operator launches a packaged `remotr-desktop` artifact
- **THEN** it opens a native Wails window using application assets embedded in that artifact and does not require a separately hosted web service

### Requirement: Connection profile discovery and persistence
The desktop application SHALL support named connection profiles containing only a profile name, server URL, Operator state-directory reference, optional CA path, and optional default Fleet, and SHALL import the resolved standard Operator configuration as an implicit Default profile when it exists.

<!-- verification-id: OS-DOA-003 -->
#### Scenario: Existing Operator configuration becomes the default profile
- **WHEN** the application starts with a valid standard Operator configuration and no desktop profiles
- **THEN** it offers a Default profile using the resolved server URL, state directory, CA, and default Fleet without copying credential material

<!-- verification-id: OS-DOA-004 -->
#### Scenario: Named profile persists non-secret connection references
- **WHEN** an operator saves a valid named connection profile
- **THEN** the application writes only the allowed non-secret fields to an owner-only settings file and never writes certificate, private-key, or token bytes into that file

<!-- verification-id: OS-DOA-005 -->
#### Scenario: Invalid profile is rejected before connection
- **WHEN** a profile has a missing or non-HTTPS server URL, a relative state directory, or an empty name
- **THEN** the application rejects the profile with field-specific guidance and sends no Admin API request

### Requirement: Profile switching isolates operational state
The desktop application SHALL create a fresh authenticated Admin client for the selected profile and SHALL clear or cancel data and work associated with the previously selected profile before presenting the new workspace.

<!-- verification-id: OS-DOA-006 -->
#### Scenario: Switching profile clears the previous workspace
- **WHEN** an operator switches from one connected profile to another
- **THEN** the application cancels obsolete requests, clears the prior Operator identity, snapshots, selections, overlays, and transient action results, and loads data only from the newly selected server

<!-- verification-id: OS-DOA-007 -->
#### Scenario: Failed profile switch does not mix servers
- **WHEN** connection verification for the newly selected profile fails
- **THEN** the application shows that profile's connection error and does not retain operational rows or action outputs from the previously connected profile

### Requirement: Operator identity is verified through mTLS
The desktop application SHALL consider a profile connected only after the existing Admin client loads the referenced Operator credential, establishes server trust, and successfully retrieves the current Operator identity from the Admin API.

<!-- verification-id: OS-DOA-008 -->
#### Scenario: Valid Operator credential connects
- **WHEN** the selected profile references a trusted, unexpired Operator credential authorized to call the identity endpoint
- **THEN** the application displays the returned Operator identity and roles and begins loading the workspace

<!-- verification-id: OS-DOA-009 -->
#### Scenario: Missing credentials offer bootstrap
- **WHEN** the selected state directory has no complete Operator credential
- **THEN** the application reports that credentials are missing, offers first-Operator bootstrap, and does not downgrade to an unauthenticated Admin API session

<!-- verification-id: OS-DOA-010 -->
#### Scenario: TLS and authorization failures remain distinct
- **WHEN** connection verification fails because of an unknown CA, expired or revoked credential, unreachable server, or forbidden identity request
- **THEN** the application presents the corresponding classified failure and safe corrective guidance without including PEM data, token values, or private-key paths in the error

### Requirement: First-Operator bootstrap is transient and compatible
The desktop application SHALL exchange a one-time bootstrap token through the existing bootstrap API and SHALL persist the issued Operator credential through the existing protected Operator credential layout.

<!-- verification-id: OS-DOA-011 -->
#### Scenario: Successful bootstrap establishes the selected profile
- **WHEN** an operator submits a valid bootstrap token for a trusted Remotr server and writable state directory
- **THEN** the backend saves the returned Operator certificate, private key, and CA with the existing protected permissions, clears the submitted token, verifies the new Operator identity, and returns only non-secret connection state to the frontend

<!-- verification-id: OS-DOA-012 -->
#### Scenario: Failed bootstrap leaves no credential fragments
- **WHEN** the bootstrap API rejects the token or credential persistence fails
- **THEN** the application clears the token input, reports a redacted error, and leaves no partial credential set that could be mistaken for a valid Operator credential

<!-- verification-id: OS-DOA-013 -->
#### Scenario: Bootstrap secret is not retained
- **WHEN** bootstrap succeeds, fails, is canceled, the profile changes, or the application exits
- **THEN** the bootstrap token is absent from browser persistence, desktop settings, logs, analytics, later view models, and unrelated error messages

### Requirement: Typed desktop bridge limits frontend authority
The Go backend SHALL expose only purpose-specific typed desktop methods and SHALL not expose an arbitrary HTTP client, filesystem browser, command runner, raw Admin client, generic Admin API method/path pair, or unrestricted credential reader to frontend code.

<!-- verification-id: OS-DOA-014 -->
#### Scenario: Frontend loads a safe workspace model
- **WHEN** the frontend requests workspace or detail data through a bound method
- **THEN** the backend returns only the documented view model and excludes private keys, PEM bodies, bootstrap tokens, TLS objects, and unrestricted diagnostic bytes

<!-- verification-id: OS-DOA-015 -->
#### Scenario: Release frontend cannot navigate to remote content
- **WHEN** a release build renders its application shell or receives an attempted external navigation
- **THEN** it serves only embedded allowlisted application assets, blocks in-window remote navigation, and does not execute remotely hosted scripts, styles, fonts, or frames

### Requirement: Server authorization remains authoritative
The desktop application SHALL use Operator identity and known roles to improve action presentation but SHALL rely on the server's RBAC decision for every Admin API request and SHALL treat forbidden results as authorization failures rather than connection failures.

<!-- verification-id: OS-DOA-016 -->
#### Scenario: Forbidden action is explained without bypass
- **WHEN** a connected Operator invokes an action that the server rejects as forbidden
- **THEN** the application keeps the session connected, reports that the current Operator lacks authorization for that action, and does not retry with another identity or weaker authentication

<!-- verification-id: OS-DOA-017 -->
#### Scenario: Hidden control is not an authorization boundary
- **WHEN** frontend state incorrectly exposes or invokes an action that role hints marked unavailable
- **THEN** the backend still validates the typed request and the server's RBAC decision determines whether the operation occurs

### Requirement: Linux delivery and versioned CLI parity are evidence-backed
The project SHALL build, test, package, and advertise Remotr Desktop only for Linux in this change. Every desktop feature release SHALL publish a machine-readable behavioral parity inventory against every current non-hidden Admin CLI capability, SHALL preserve previously implemented entries, and SHALL claim complete parity only when no applicable capability remains planned.

<!-- verification-id: OS-DOA-018 -->
#### Scenario: Supported Linux artifact passes native smoke
- **WHEN** a Linux architecture or package format is marked supported for a release
- **THEN** that exact Linux artifact has passed its native build and launch smoke checks with the expected embedded version and application identity, and the release contains no macOS or Windows desktop artifact

<!-- verification-id: OS-DOA-019 -->
#### Scenario: Feature release reports CLI parity truthfully
- **WHEN** a desktop feature release is published against the current non-hidden Admin CLI command tree
- **THEN** its parity inventory maps every operator-observable capability to an implemented desktop workflow, a target feature release, or a reviewed interface-only non-applicable reason, preserves previously implemented entries, and reports complete parity only when every applicable entry has passing evidence
