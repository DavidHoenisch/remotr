## ADDED Requirements

### Requirement: Operators reserve endpoint IDs before enrollment
An authorized operator SHALL be able to reserve a valid endpoint ID in an existing fleet before any machine enrolls with that ID. Reservation SHALL create the endpoint in the `reserved` enrollment state with no client certificate, and SHALL return a single-use enrollment token bound to exactly that fleet and endpoint ID. Reservation SHALL use the existing endpoint ID format and SHALL be rejected for unauthenticated or unauthorized callers.

#### Scenario: Operator reserves a new endpoint ID
<!-- verification-id: OS-ENR-001 -->
- **WHEN** an authorized operator reserves endpoint ID `gitlab-runner-01` in fleet `ubuntu-servers`
- **THEN** the server records `gitlab-runner-01` as a `reserved` endpoint in `ubuntu-servers` and returns a single-use enrollment token bound to that fleet and ID

#### Scenario: Reservation input is invalid
<!-- verification-id: OS-ENR-002 -->
- **WHEN** an operator reserves an ID that violates the endpoint ID format, or names a fleet that does not exist
- **THEN** the request is rejected and no endpoint or token is created

#### Scenario: Reserved ID is already enrolled
<!-- verification-id: OS-ENR-003 -->
- **WHEN** an operator reserves an ID that belongs to an enrolled endpoint
- **THEN** the request is rejected as a conflict and the enrolled endpoint and its credentials are unchanged

#### Scenario: Caller is not authorized
<!-- verification-id: OS-ENR-004 -->
- **WHEN** an unauthenticated caller, or an operator without enrollment administration permission, requests a reservation
- **THEN** the request is denied and no endpoint or token is created

### Requirement: Bound enrollment claims the reserved endpoint
Enrollment with a reservation-bound token SHALL claim the reserved endpoint: the server SHALL assign the bound endpoint ID and fleet, SHALL transition the endpoint to `enrolled`, SHALL ignore any endpoint ID supplied by the agent, and SHALL return the bound ID in the enrollment response. A bound token SHALL be redeemable at most once and only before it expires or is revoked.

#### Scenario: Agent enrolls with a bound token
<!-- verification-id: OS-ENR-005 -->
- **WHEN** an agent enrolls with the bound token for `gitlab-runner-01` and supplies its default hostname-derived endpoint ID
- **THEN** the endpoint is enrolled as `gitlab-runner-01` in the reserved fleet, the response returns `gitlab-runner-01`, and the agent authenticates subsequent Syncs as `gitlab-runner-01`

#### Scenario: Bound token is reused, expired, or revoked
<!-- verification-id: OS-ENR-006 -->
- **WHEN** an agent presents a bound token that was already redeemed, has expired, or was revoked
- **THEN** enrollment is rejected and the reserved or enrolled endpoint's state and credentials are unchanged

### Requirement: Reserved IDs are claimable only through their bound token
An endpoint ID in the `reserved` state SHALL NOT be claimable by enrollment with an unbound enrollment token, a deployment token, or a token bound to a different endpoint. Hostnames and agent-reported labels SHALL NOT select an endpoint ID or an override path.

#### Scenario: Unbound credential requests a reserved ID
<!-- verification-id: OS-ENR-007 -->
- **WHEN** an agent enrolls with an unbound enrollment token or a deployment token and requests endpoint ID `gitlab-runner-01` while it is reserved
- **THEN** enrollment is rejected and the reservation remains claimable by its bound token

### Requirement: Overrides compose for reserved endpoints
Git sync SHALL compose and store endpoint override artifacts for every `endpoints/<endpoint-id>/` directory whose ID is enrolled or reserved, so an override can be released before its endpoint enrolls. The first Sync after a bound enrollment SHALL deliver the endpoint override artifact rather than the fleet artifact. Composition of an override whose ID is neither enrolled nor reserved SHALL fail with an error naming the endpoint ID and stating that it must be reserved or enrolled.

#### Scenario: Override merges before the machine exists
<!-- verification-id: OS-ENR-008 -->
- **WHEN** `gitlab-runner-01` is reserved, its override is merged and Git sync advances the release, and an agent then enrolls with the bound token and syncs
- **THEN** Git sync succeeds and the agent's first Sync receives the composed `gitlab-runner-01` override artifact

#### Scenario: Override names an unknown endpoint
<!-- verification-id: OS-ENR-009 -->
- **WHEN** Git sync composes `endpoints/unknown-host-01/` and that ID is neither enrolled nor reserved
- **THEN** release advance is blocked and the reported error names `unknown-host-01` and the reserve-or-enroll remedy

#### Scenario: Reserved endpoint cannot Sync
<!-- verification-id: OS-ENR-010 -->
- **WHEN** a client attempts Sync as an endpoint that is still `reserved`
- **THEN** the request is rejected because the endpoint has no enrolled client certificate

### Requirement: Reservation lifecycle is operator-managed and visible
Reserving an ID that is already `reserved` in the same fleet SHALL revoke its outstanding bound token and issue a new one. An operator SHALL be able to release an unclaimed reservation; releasing SHALL revoke its bound token and remove the reserved endpoint, and SHALL be rejected for enrolled endpoints. When the current release contains a compiled override for the reserved ID, releasing SHALL be rejected with an explicit error that names the endpoint ID, the release, the `endpoints/<endpoint-id>/` directory, the consequence that Git sync will be blocked until the directory is removed, and the `--force` option, unless the operator explicitly forces it. Endpoint listing in the Admin API and operator CLI SHALL show each endpoint's enrollment state. Bound tokens and reservation secrets SHALL NOT appear in listings, logs, or audit records.

#### Scenario: Operator re-issues a reservation token
<!-- verification-id: OS-ENR-011 -->
- **WHEN** an operator reserves `gitlab-runner-01` again while it is reserved in the same fleet
- **THEN** the previous bound token can no longer enroll and the newly returned token can

#### Scenario: Operator reserves an existing reservation in a different fleet
<!-- verification-id: OS-ENR-012 -->
- **WHEN** an operator reserves `gitlab-runner-01` in a fleet other than the one it is reserved in
- **THEN** the request is rejected as a conflict and the existing reservation is unchanged

#### Scenario: Operator releases a reservation
<!-- verification-id: OS-ENR-013 -->
- **WHEN** an operator unreserves `gitlab-runner-01` while it is reserved
- **THEN** the reserved endpoint is removed and its bound token can no longer enroll

#### Scenario: Release conflicts with a released override
<!-- verification-id: OS-ENR-017 -->
- **WHEN** an operator unreserves `gitlab-runner-01` without force while the current release has a compiled override for it
- **THEN** the request is rejected, the reservation is unchanged, and the Admin API error and CLI output name `gitlab-runner-01`, the release, `endpoints/gitlab-runner-01/`, the blocked-Git-sync consequence, and `--force`

#### Scenario: Operator forces release despite a released override
<!-- verification-id: OS-ENR-018 -->
- **WHEN** an operator unreserves `gitlab-runner-01` with force while the current release has a compiled override for it
- **THEN** the reservation is removed, its bound token can no longer enroll, the CLI warns that Git sync is blocked until `endpoints/gitlab-runner-01/` is removed, and the forced release is audited

#### Scenario: Operator tries to release an enrolled endpoint
<!-- verification-id: OS-ENR-014 -->
- **WHEN** an operator unreserves an enrolled endpoint
- **THEN** the request is rejected and the endpoint is unchanged

#### Scenario: Operator lists endpoints
<!-- verification-id: OS-ENR-015 -->
- **WHEN** an operator lists endpoints in table or JSON output
- **THEN** each endpoint shows `reserved` or `enrolled`, and no bound token value appears in the output, server logs, or audit records

### Requirement: Existing enrollment and override behavior is preserved
Unbound enrollment tokens, deployment tokens, agent-chosen endpoint IDs for unreserved IDs, and overrides for enrolled endpoints SHALL behave as before this change.

#### Scenario: Unreserved enrollment is unchanged
<!-- verification-id: OS-ENR-016 -->
- **WHEN** an agent enrolls with an unbound enrollment token or deployment token and an unreserved endpoint ID
- **THEN** the endpoint enrolls with that ID in the token's fleet exactly as before, and an existing override for an enrolled endpoint is still selected ahead of its fleet artifact
