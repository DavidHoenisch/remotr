## MODIFIED Requirements

### Requirement: Attachment consumes a scoped secret without exposure
An attached resource SHALL authorize `tokenRef` only for the authenticated endpoint, the referenced Secret's explicit global, fleet, or endpoint scope, active artifact digest, exact resource address, and `ubuntu-pro-token` purpose. Global scope SHALL permit the same selected token version to serve authorized Ubuntu Pro resources in multiple fleets but SHALL NOT bypass any other authorization condition. The provider SHALL resolve the token only after platform/client preflight and only when the endpoint is unattached, and SHALL pass the bounded token to the versioned full-token attach API through protected stdin with automatic service enablement disabled. Token bytes SHALL NOT appear in argv, environment variables, temporary files, desired state, effective hashes, plans, audit, logs, reports, errors, rollback records, or retained test artifacts.

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

#### Scenario: Multiple fleets attach with one global token
<!-- verification-id: OS-UPM-041 -->
- **WHEN** eligible unattached endpoints in separate fleets apply active artifacts whose Ubuntu Pro resources authorize the same global `tokenRef`
- **THEN** each endpoint may resolve that selected version for purpose `ubuntu-pro-token` and attach without creating fleet-scoped copies of the token

#### Scenario: Global token is referenced for another purpose
<!-- verification-id: OS-UPM-065 -->
- **WHEN** an otherwise eligible endpoint requests the global Ubuntu Pro token from a resource or purpose not authorized by its active artifact
- **THEN** resolution is denied before the provider receives token material and no token or inaccessible-secret metadata is logged
