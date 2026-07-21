# Linux Security and Secret Management Specification

## Purpose

Define safe Linux security-policy, secret, certificate, trust, and logging management with bounded redacted evidence.

## Requirements

### Requirement: Mandatory access control state is provider-specific
The security contract SHALL distinguish SELinux and AppArmor as separate providers and SHALL reject policy operations not supported by the active kernel/distribution. This change SHALL advertise only the AppArmor provider; the SELinux contract remains roadmap guidance until the future RPM-family change implements and tests it.

#### Scenario: SELinux resource targets AppArmor host
<!-- verification-id: OS-LSM-001 -->
- **WHEN** an SELinux-specific resource resolves on an AppArmor-only endpoint
- **THEN** Check returns `unsupported` and does not approximate the policy with file permissions

### Requirement: SELinux resources cover effective policy objects
When a future RPM-family change advertises SELinux, resources SHALL manage global mode, booleans, file-context declarations, port mappings, policy modules, users, and logins as separately owned objects with runtime/persistent semantics where applicable. SELinux is roadmap-only and SHALL remain unadvertised by this implementation change.

#### Scenario: Boolean persistence differs
<!-- verification-id: OS-LSM-002 -->
- **WHEN** a managed SELinux boolean has the desired runtime value but a different persistent value
- **THEN** Check reports persistent drift and Apply corrects persistence

### Requirement: AppArmor profiles are validated before activation
AppArmor resources SHALL manage named profile content and enforce, complain, or disabled mode, and SHALL parse/validate a staged profile before loading it.

#### Scenario: Profile syntax is invalid
<!-- verification-id: OS-LSM-003 -->
- **WHEN** staged AppArmor policy fails parser validation
- **THEN** the active profile and mode remain unchanged

### Requirement: Audit policy is first-class
Audit resources SHALL manage named audit rule fragments and loading state, validate the complete effective ruleset, and distinguish reboot-required immutable mode from live-loadable changes.

#### Scenario: Immutable audit rules prevent reload
<!-- verification-id: OS-LSM-004 -->
- **WHEN** rules differ and the active audit subsystem is immutable
- **THEN** Apply writes valid persistent state, returns `reboot-required`, and does not claim live convergence

### Requirement: Secret material is reference-only in Git
Secret values, private keys, password hashes, and repository credentials SHALL not appear in authored or composed Git artifacts. Resources SHALL reference an authorized secret source identifier.

#### Scenario: Inline private key is authored
<!-- verification-id: OS-LSM-005 -->
- **WHEN** validation encounters private-key material in a secret-valued field
- **THEN** validation rejects it and directs the author to use a secret reference

### Requirement: Initial secret providers have clear boundaries
The system SHALL provide `local-file` for root-readable material provisioned independently of Remotr and a server-backed `remotr` provider for production distribution. The Remotr provider SHALL store versioned application-encrypted records in the server registry while keeping its encryption master key outside Postgres. The provider SHALL NOT expose a general plaintext-read workflow to operators.

#### Scenario: Local file is externally provisioned
<!-- verification-id: OS-LSM-006 -->
- **WHEN** a resource references `local-file` material with correct root-only access
- **THEN** the agent resolves it locally without copying the value into the artifact or server registry

#### Scenario: Operator uploads a Remotr secret
<!-- verification-id: OS-LSM-007 -->
- **WHEN** an authorized operator supplies secret bytes through stdin or a protected input file
- **THEN** the server creates an encrypted version and returns only safe name, version, fingerprint, scope, and audit metadata

#### Scenario: Operator attempts plaintext retrieval
<!-- verification-id: OS-LSM-008 -->
- **WHEN** an operator requests the stored secret value through the Admin API or CLI
- **THEN** the provider refuses plaintext readback while allowing authorized metadata inspection, rotation, and revocation

### Requirement: Remotr secrets use envelope encryption
Every Remotr-managed secret version SHALL use a fresh random 256-bit DEK and AES-256-GCM authenticated encryption with random nonces. The DEK SHALL be wrapped by an identified KEK from a versioned external keyring. Postgres SHALL contain ciphertext, wrapped DEK, cryptographic format metadata, and authenticated non-secret scope metadata but SHALL NOT contain the KEK.

#### Scenario: Two versions contain identical plaintext
<!-- verification-id: OS-LSM-009 -->
- **WHEN** an operator uploads identical bytes as two secret versions
- **THEN** independently random DEKs and nonces produce independent ciphertext records

#### Scenario: Production keyring is missing
<!-- verification-id: OS-LSM-010 -->
- **WHEN** the Remotr provider is enabled in production without a configured external KEK
- **THEN** the server fails closed and does not silently generate or store a replacement key in Postgres

### Requirement: Master-key lifecycle distinguishes rotation from compromise
The keyring SHALL support one active encryption KEK and decrypt-only historical KEKs. Routine rotation SHALL rewrap stored DEKs under a new KEK. Compromise recovery SHALL generate new DEKs and re-encrypt secret ciphertext. Tooling SHALL prevent removal of a KEK still referenced by stored records.

#### Scenario: Routine master-key rotation
<!-- verification-id: OS-LSM-011 -->
- **WHEN** a new active KEK is installed and routine rewrap completes
- **THEN** secret ciphertext remains unchanged, DEKs are wrapped by the new key, and the old key can be removed only after no references remain

#### Scenario: Master key may be compromised
<!-- verification-id: OS-LSM-012 -->
- **WHEN** an operator invokes compromise rekey
- **THEN** the server generates new DEKs, re-encrypts every affected secret version, and records the security event

### Requirement: Secret backups require the external keyring
Backup and recovery documentation and diagnostics SHALL state that encrypted database records are recoverable only with the corresponding external KEK keyring and SHALL verify key coverage without exposing key material.

#### Scenario: Restore lacks historical KEK
<!-- verification-id: OS-LSM-013 -->
- **WHEN** a restored database contains versions wrapped by a missing KEK
- **THEN** diagnostics identify affected key IDs and secret metadata while refusing resolution

### Requirement: Key wrapping is provider-extensible
The Remotr secret store SHALL use a key-encryption-provider contract so a later KMS or HSM can wrap DEKs without changing consumer resource schemas or stored secret version semantics.

#### Scenario: KMS wrapping provider is introduced
<!-- verification-id: OS-LSM-014 -->
- **WHEN** a deployment switches new encryption to a supported KMS provider
- **THEN** resources continue using the same secret references while rotation migrates wrapped DEKs according to policy

### Requirement: Secret retrieval is scoped
Secret providers SHALL authorize retrieval for authenticated endpoint identity, Fleet/endpoint scope, active artifact digest, resource address, and declared purpose, return bounded material over a protected channel or root-only local source, and avoid placing resolved values in argv or environment visible to unrelated processes.

#### Scenario: Endpoint lacks authorization
<!-- verification-id: OS-LSM-015 -->
- **WHEN** an endpoint requests a secret reference it is not authorized to use
- **THEN** Apply fails with an authorization reason and no secret material is returned or logged

#### Scenario: Resource is absent from active artifact
<!-- verification-id: OS-LSM-016 -->
- **WHEN** an endpoint requests a Remotr secret for a resource address or artifact digest that is not currently active for it
- **THEN** the server rejects resolution and audits the denied request

### Requirement: Secret providers are extensible without changing resource schemas
Secret-backed resource schemas SHALL use a provider-neutral reference and resolution context. Later server-side providers SHALL implement version metadata, authorization, resolution, redaction, and audit contracts without changing each consuming resource kind.

#### Scenario: External vault adapter is added
<!-- verification-id: OS-LSM-017 -->
- **WHEN** a later provider resolves the same secret reference contract from an external manager
- **THEN** certificate, account, repository, network, and service resources consume it without provider-specific secret fields

### Requirement: Secret version selection is explicit
Every Secret reference SHALL explicitly select an exact provider version or the provider's `active` version and SHALL declare its purpose. The system SHALL NOT provide an implicit latest selector.

#### Scenario: Resource pins a secret version
<!-- verification-id: OS-LSM-018 -->
- **WHEN** a resource selects exact version `7`
- **THEN** provider activation of version `8` does not change that resource's effective desired state

#### Scenario: Resource follows active version
<!-- verification-id: OS-LSM-019 -->
- **WHEN** a resource selects `active` and version `8` becomes active
- **THEN** its effective desired state changes to version `8` through an audited rollout

#### Scenario: Version selector is omitted
<!-- verification-id: OS-LSM-020 -->
- **WHEN** a Secret reference has no exact or active selector
- **THEN** validation rejects the reference rather than assuming latest

### Requirement: Upload and activation are separate operations
Uploading a Remotr secret SHALL create an inactive version. A separate audited activation SHALL select the active version and SHALL create rollout work governed by every referencing Resource's risk policy.

#### Scenario: Network credential activates
<!-- verification-id: OS-LSM-021 -->
- **WHEN** an operator activates a new version referenced by a connectivity-risk Resource
- **THEN** the server creates or updates a high-risk Change request before any endpoint receives the new material

#### Scenario: Inactive version is uploaded
<!-- verification-id: OS-LSM-022 -->
- **WHEN** an operator uploads a version but does not activate it and no resource pins it
- **THEN** endpoint desired state remains unchanged

### Requirement: Effective hashes identify secret versions without values
The effective Resource hash SHALL include provider, logical secret name, selected version identity, activation generation, and purpose but SHALL NOT include secret bytes. A version change SHALL invalidate authorization bound to the prior effective hash.

#### Scenario: Active version changes after approval
<!-- verification-id: OS-LSM-023 -->
- **WHEN** a high-risk resource was approved with one active secret version and another version activates
- **THEN** the prior approval no longer matches the effective Resource hash

### Requirement: Revocation does not claim remote erasure
Revoking a Secret version SHALL prevent future resolution but SHALL NOT report previously installed endpoint copies removed unless desired state explicitly rotates or removes them and Check verifies that outcome.

#### Scenario: Installed credential version is revoked
<!-- verification-id: OS-LSM-024 -->
- **WHEN** an endpoint already contains a credential whose provider version is revoked
- **THEN** reporting identifies required rotation/removal rather than claiming revocation erased the endpoint copy

### Requirement: Certificates and keys converge safely
Certificate resources SHALL manage certificate/key presence, chain, subject/SAN expectations, safe fingerprint, expiry threshold, owner/group/mode, provider source, renewal policy, and activation notification. Private key bytes SHALL never be reported.

#### Scenario: Certificate nears expiry
<!-- verification-id: OS-LSM-025 -->
- **WHEN** a certificate is within its managed renewal threshold
- **THEN** Check reports actionable drift using expiry and fingerprint metadata and Apply obtains renewed material through its provider

#### Scenario: Key and certificate do not match
<!-- verification-id: OS-LSM-026 -->
- **WHEN** staged certificate and private key are not a pair
- **THEN** Apply fails validation before replacing active files

### Requirement: Trust-store entries are scoped and refreshable
CA trust resources SHALL manage named trust anchors with fingerprint and presence, use distribution-supported trust directories, and trigger trust-store refresh only after verified changes.

#### Scenario: Trust anchor is removed
<!-- verification-id: OS-LSM-027 -->
- **WHEN** a named CA declares absent
- **THEN** Apply removes only its owned anchor and refreshes the trust store

### Requirement: Secret reporting and rollback do not leak values
Logs, state reports, diagnostics, backups, errors, and rollback metadata SHALL omit secret bytes. Secret rollback SHALL use provider version references or protected encrypted storage and SHALL report when rollback is unavailable.

#### Scenario: Secret activation fails
<!-- verification-id: OS-LSM-028 -->
- **WHEN** a service fails after new secret material is installed
- **THEN** rollback uses a protected prior version when supported and reports only version/fingerprint metadata

### Requirement: Journald policy is structured
When logging policy is advertised, journald resources SHALL manage storage mode, retention/disk limits, rate limits, and forwarding settings with validation and activation reporting.

#### Scenario: Journald limit changes
<!-- verification-id: OS-LSM-029 -->
- **WHEN** a managed limit differs
- **THEN** Apply changes a Remotr-owned drop-in, validates it, and emits the required service activation

### Requirement: Log rotation is structurally validated
Logrotate resources SHALL manage named fragments including paths, cadence, retention, compression, ownership/creation, and safe scripts, and SHALL validate the complete configuration before activation.

#### Scenario: Logrotate fragment is invalid
<!-- verification-id: OS-LSM-030 -->
- **WHEN** a staged fragment fails effective configuration validation
- **THEN** the existing fragment remains active and Apply reports validation failure

### Requirement: Security reports are bounded and redacted
Security resources SHALL report provider, mode, object identity, safe fingerprint/expiry, activation, and validation outcomes with bounded diagnostics and no policy-injected secret expansion.

#### Scenario: Policy command diagnostic contains secret-like input
<!-- verification-id: OS-LSM-031 -->
- **WHEN** a backend diagnostic includes resolved sensitive material
- **THEN** typed redaction removes the value before persistence or transmission
