## ADDED Requirements

### Requirement: Remotr secret scope is explicit and immutable
Every Remotr-managed logical secret SHALL be created with exactly one scope: `global`, `fleet`, or `endpoint`. Global scope SHALL be selected explicitly, SHALL NOT be inferred from a missing scope identifier, and SHALL NOT be the default. All versions of a logical secret SHALL retain its original scope; changing scope SHALL require a distinct logical secret and an explicit configuration migration.

#### Scenario: Operator creates a global secret
<!-- verification-id: OS-LSM-061 -->
- **WHEN** an authorized operator explicitly uploads the first version of a logical secret with scope `global` and no scope identifier
- **THEN** the server creates an inactive globally scoped version and returns safe metadata identifying its scope as `global`

#### Scenario: Scope is omitted
<!-- verification-id: OS-LSM-062 -->
- **WHEN** an operator uploads a secret without selecting `global`, `fleet`, or `endpoint`
- **THEN** the request is rejected rather than defaulting to global scope

#### Scenario: Later version changes scope
<!-- verification-id: OS-LSM-063 -->
- **WHEN** an operator uploads a version whose requested scope differs from the logical secret's existing scope
- **THEN** the server rejects the upload without changing or copying any existing secret material

### Requirement: Global secret lifecycle accounts for every authorized fleet use
Listing, activation, rotation, revocation, deletion, rollout planning, and audit behavior for a global secret SHALL operate on its single logical version history and SHALL account for every active artifact authorized to reference it across fleets. Activation SHALL create rollout work governed by each referencing Resource's risk policy, and deletion safety SHALL retain versions required by any authorized fleet's active rollback reference.

#### Scenario: Global active version rotates across fleets
<!-- verification-id: OS-LSM-064 -->
- **WHEN** a new version of a global secret is activated while active resources in multiple fleets follow `active`
- **THEN** one audited activation generation creates or updates the required rollout work for every referencing resource in every affected fleet

#### Scenario: One fleet retains a rollback reference
<!-- verification-id: OS-LSM-065 -->
- **WHEN** deletion is requested for a global secret version still retained by an unexpired rollback reference in any fleet
- **THEN** deletion is refused unless the existing authorized abandonment policy is satisfied

#### Scenario: Operator lacks global-secret permission
<!-- verification-id: OS-LSM-066 -->
- **WHEN** an operator authorized for one fleet attempts to create, activate, revoke, or delete a global secret
- **THEN** the Admin API rejects the operation without revealing secret bytes or inaccessible usage details

### Requirement: Operators can discover and inspect authorized logical secrets
The Admin API and operator CLI SHALL provide an authorization-filtered logical-secret collection independent of any preselected secret ID. `secret list` SHALL enumerate visible logical secrets with classified safe metadata and SHALL NOT require an ID. `secret show <secret-id>` SHALL display that logical secret's version metadata. When `secret show` omits the ID in an interactive terminal, it SHALL use the standard resource picker over the same authorized collection; non-interactive or structured-output invocation SHALL require an explicit ID and SHALL NOT wait for interactive input.

#### Scenario: Operator lists secrets before knowing an ID
<!-- verification-id: OS-LSM-069 -->
- **WHEN** an authorized operator runs `remotr secret list` without a secret ID
- **THEN** the CLI lists each visible logical secret with only its ID/name, scope, active-version status, and other classified safe summary metadata

#### Scenario: Operator shows one secret explicitly
<!-- verification-id: OS-LSM-070 -->
- **WHEN** an authorized operator runs `remotr secret show <secret-id>`
- **THEN** the CLI displays that logical secret's safe metadata and version history without returning plaintext material

#### Scenario: Interactive show omits the secret ID
<!-- verification-id: OS-LSM-071 -->
- **WHEN** a human runs `remotr secret show` without an ID on an interactive terminal and selects a visible secret
- **THEN** the standard picker identifies choices with safe scope and active-status context and the CLI shows the selected secret

#### Scenario: Non-interactive show omits the secret ID
<!-- verification-id: OS-LSM-072 -->
- **WHEN** `remotr secret show` has no ID and stdin or output is non-interactive, or structured output was requested
- **THEN** the CLI fails promptly with guidance to provide a secret ID and does not open or wait for a picker

#### Scenario: Operator cannot view another scope's inventory
<!-- verification-id: OS-LSM-073 -->
- **WHEN** a Fleet-limited operator lists secrets while global or other-scope secrets exist outside that operator's authority
- **THEN** the server omits inaccessible logical secrets and does not reveal their names, counts, versions, fingerprints, or scopes

## MODIFIED Requirements

### Requirement: Upload and activation are separate operations
Uploading a Remotr secret SHALL create an inactive version. A separate audited activation SHALL first discover the complete current set of authorized `active` consumers, derive their effective identities and risks, create every Change request required by those risks, and validate exactly one rollout binding for every consumer before atomically selecting the active version. If discovery or planning is incomplete, a consumer is omitted or ambiguous, or a high-risk binding lacks a Change request, activation SHALL fail and the prior active version and generation SHALL remain unchanged.

#### Scenario: Network credential activates
<!-- verification-id: OS-LSM-021 -->
- **WHEN** an operator activates a new version referenced by a connectivity-risk Resource
- **THEN** the server creates or updates a high-risk Change request before committing activation or allowing any endpoint to receive the new material

#### Scenario: Inactive version is uploaded
<!-- verification-id: OS-LSM-022 -->
- **WHEN** an operator uploads a version but does not activate it and no resource pins it
- **THEN** endpoint desired state remains unchanged

#### Scenario: High-risk activation planning omits a consumer
<!-- verification-id: OS-LSM-074 -->
- **WHEN** an `active` secret has a current high-risk consumer but planning cannot produce its canonical Change request and exact rollout binding
- **THEN** activation is rejected atomically and the previously active version and generation remain unchanged

#### Scenario: Lower-risk activation has a complete binding
<!-- verification-id: OS-LSM-075 -->
- **WHEN** every current `active` consumer is lower risk and receives an exact validated rollout binding
- **THEN** activation may commit without a Change request and remains audited and bound to those consumers

#### Scenario: Active resolution has no matching binding
<!-- verification-id: OS-LSM-076 -->
- **WHEN** an endpoint requests an `active` reference for a resource and purpose with no exact rollout binding on the selected version
- **THEN** resolution is denied rather than treating the missing binding as unrestricted authorization

#### Scenario: Authorized activation bootstraps its execution lease
<!-- verification-id: OS-LSM-077 -->
- **WHEN** an endpoint has checked the exact current artifact and reports the activated high-risk consumer's canonical hash and ready preflight but cannot acknowledge the artifact until Apply receives an execution lease
- **THEN** authenticated Sync may issue the bound lease from that exact current artifact report without a prior acknowledgement, while stale artifact digests and stale non-empty release acknowledgements remain ineligible

### Requirement: Secret retrieval is scoped
Secret providers SHALL authorize retrieval for authenticated endpoint identity, the referenced Secret's explicit global, Fleet, or endpoint scope, active artifact digest, resource address, and declared purpose. A globally scoped Secret SHALL satisfy only the fleet-membership portion of authorization and SHALL NOT bypass endpoint identity, active-artifact, resource-address, purpose, version-status, or rollout gates. Providers SHALL return bounded material over a protected channel or root-only local source and avoid placing resolved values in argv or environment visible to unrelated processes.

#### Scenario: Endpoint lacks authorization
<!-- verification-id: OS-LSM-015 -->
- **WHEN** an endpoint requests a secret reference it is not authorized to use
- **THEN** Apply fails with an authorization reason and no secret material is returned or logged

#### Scenario: Resource is absent from active artifact
<!-- verification-id: OS-LSM-016 -->
- **WHEN** an endpoint requests a Remotr secret for a resource address or artifact digest that is not currently active for it
- **THEN** the server rejects resolution and audits the denied request

#### Scenario: Two fleets consume one global secret
<!-- verification-id: OS-LSM-067 -->
- **WHEN** authenticated endpoints in two fleets each request the same global reference from their own active artifact at an authorized resource address and purpose
- **THEN** the server resolves the same selected secret version for both requests without copying it into fleet-scoped records

#### Scenario: Global reference is requested outside its declared use
<!-- verification-id: OS-LSM-068 -->
- **WHEN** an authenticated endpoint requests a global secret with a different artifact digest, resource address, or purpose from the authorized consumer
- **THEN** the server denies resolution with a bounded indistinguishable authorization failure and returns no existence, scope, version, or material detail

#### Scenario: Ubuntu Pro token file has a terminal line ending
<!-- verification-id: OS-LSM-078 -->
- **WHEN** an authorized Ubuntu Pro consumer resolves enrollment-token material uploaded from a line-oriented file with exactly one terminal LF or CRLF
- **THEN** the provider removes that line ending before Canonical's protected stdin boundary, preserves every other token byte, and clears the complete resolver-owned buffer after use

#### Scenario: Ubuntu Pro reports unmanaged informational services
<!-- verification-id: OS-LSM-079 -->
- **WHEN** a qualified Ubuntu Pro client includes independent historical, preview, or future service nodes alongside a cataloged service in its versioned dependency response
- **THEN** the provider ignores those unmanaged informational nodes and converges the cataloged service, while an unknown dependency or incompatibility declared by a managed service remains a fail-closed invalid graph
