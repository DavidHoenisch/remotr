## ADDED Requirements

### Requirement: Mandatory access control state is provider-specific
The security contract SHALL provide distinct SELinux and AppArmor providers and SHALL reject policy operations not supported by the active kernel/distribution.

#### Scenario: SELinux resource targets AppArmor host
- **WHEN** an SELinux-specific resource resolves on an AppArmor-only endpoint
- **THEN** Check returns `unsupported` and does not approximate the policy with file permissions

### Requirement: SELinux resources cover effective policy objects
When SELinux is advertised, resources SHALL manage global mode, booleans, file-context declarations, port mappings, policy modules, users, and logins as separately owned objects with runtime/persistent semantics where applicable.

#### Scenario: Boolean persistence differs
- **WHEN** a managed SELinux boolean has the desired runtime value but a different persistent value
- **THEN** Check reports persistent drift and Apply corrects persistence

### Requirement: AppArmor profiles are validated before activation
AppArmor resources SHALL manage named profile content and enforce, complain, or disabled mode, and SHALL parse/validate a staged profile before loading it.

#### Scenario: Profile syntax is invalid
- **WHEN** staged AppArmor policy fails parser validation
- **THEN** the active profile and mode remain unchanged

### Requirement: Audit policy is first-class
Audit resources SHALL manage named audit rule fragments and loading state, validate the complete effective ruleset, and distinguish reboot-required immutable mode from live-loadable changes.

#### Scenario: Immutable audit rules prevent reload
- **WHEN** rules differ and the active audit subsystem is immutable
- **THEN** Apply writes valid persistent state, returns `reboot-required`, and does not claim live convergence

### Requirement: Secret material is reference-only in Git
Secret values, private keys, password hashes, and repository credentials SHALL not appear in authored or composed Git artifacts. Resources SHALL reference an authorized secret source identifier.

#### Scenario: Inline private key is authored
- **WHEN** validation encounters private-key material in a secret-valued field
- **THEN** validation rejects it and directs the author to use a secret reference

### Requirement: Secret retrieval is scoped
Secret providers SHALL authorize retrieval for the endpoint, resource address, and purpose, return bounded material over a protected channel or root-only local source, and avoid placing resolved values in argv or environment visible to unrelated processes.

#### Scenario: Endpoint lacks authorization
- **WHEN** an endpoint requests a secret reference it is not authorized to use
- **THEN** Apply fails with an authorization reason and no secret material is returned or logged

### Requirement: Certificates and keys converge safely
Certificate resources SHALL manage certificate/key presence, chain, subject/SAN expectations, safe fingerprint, expiry threshold, owner/group/mode, provider source, renewal policy, and activation notification. Private key bytes SHALL never be reported.

#### Scenario: Certificate nears expiry
- **WHEN** a certificate is within its managed renewal threshold
- **THEN** Check reports actionable drift using expiry and fingerprint metadata and Apply obtains renewed material through its provider

#### Scenario: Key and certificate do not match
- **WHEN** staged certificate and private key are not a pair
- **THEN** Apply fails validation before replacing active files

### Requirement: Trust-store entries are scoped and refreshable
CA trust resources SHALL manage named trust anchors with fingerprint and presence, use distribution-supported trust directories, and trigger trust-store refresh only after verified changes.

#### Scenario: Trust anchor is removed
- **WHEN** a named CA declares absent
- **THEN** Apply removes only its owned anchor and refreshes the trust store

### Requirement: Secret reporting and rollback do not leak values
Logs, state reports, diagnostics, backups, errors, and rollback metadata SHALL omit secret bytes. Secret rollback SHALL use provider version references or protected encrypted storage and SHALL report when rollback is unavailable.

#### Scenario: Secret activation fails
- **WHEN** a service fails after new secret material is installed
- **THEN** rollback uses a protected prior version when supported and reports only version/fingerprint metadata

### Requirement: Journald policy is structured
When logging policy is advertised, journald resources SHALL manage storage mode, retention/disk limits, rate limits, and forwarding settings with validation and activation reporting.

#### Scenario: Journald limit changes
- **WHEN** a managed limit differs
- **THEN** Apply changes a Remotr-owned drop-in, validates it, and emits the required service activation

### Requirement: Log rotation is structurally validated
Logrotate resources SHALL manage named fragments including paths, cadence, retention, compression, ownership/creation, and safe scripts, and SHALL validate the complete configuration before activation.

#### Scenario: Logrotate fragment is invalid
- **WHEN** a staged fragment fails effective configuration validation
- **THEN** the existing fragment remains active and Apply reports validation failure

### Requirement: Security reports are bounded and redacted
Security resources SHALL report provider, mode, object identity, safe fingerprint/expiry, activation, and validation outcomes with bounded diagnostics and no policy-injected secret expansion.

#### Scenario: Policy command diagnostic contains secret-like input
- **WHEN** a backend diagnostic includes resolved sensitive material
- **THEN** typed redaction removes the value before persistence or transmission

