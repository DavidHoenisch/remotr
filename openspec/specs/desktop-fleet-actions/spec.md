# Desktop Fleet Actions Specification

## Purpose

Define safe, typed, server-authoritative fleet and endpoint actions exposed by the Linux desktop application.

## Requirements

### Requirement: Actions use a consistent observable lifecycle
Every desktop mutation SHALL use a purpose-specific backend method with validated input, one active submission per target and action, visible progress, a structured server-acknowledged result, classified failure, and an explicit retry path that does not silently duplicate a pending request. The first release SHALL apply this contract to its bounded action set; later feature releases SHALL expand the same contract to every applicable Admin CLI mutation until parity is complete.

<!-- verification-id: OS-DFA-001 -->
#### Scenario: Repeated activation submits once
- **WHEN** an operator repeatedly activates an action while its first request is pending
- **THEN** the application sends one Admin API request, keeps the action visibly pending, and prevents an accidental duplicate submission

<!-- verification-id: OS-DFA-002 -->
#### Scenario: Validation failure sends no request
- **WHEN** an action contains a missing, malformed, or out-of-bounds required field
- **THEN** the backend returns field-specific validation guidance and sends no Admin API mutation

<!-- verification-id: OS-DFA-003 -->
#### Scenario: Server failure preserves safe context
- **WHEN** the Admin API rejects an action or its network request fails
- **THEN** the application keeps the relevant resource detail and non-secret input visible, clears any secret input required by its dedicated rule, and offers an explicit retry or cancel path

<!-- verification-id: OS-DFA-004 -->
#### Scenario: Successful action refreshes affected evidence
- **WHEN** the Admin API acknowledges an action
- **THEN** the application displays exactly what the server accepted, refreshes the affected resource and recent Activity, and does not claim that an Endpoint has converged before later evidence reports it

### Requirement: Git sync is an explicit server operation
The desktop application SHALL allow an authorized Operator to request the existing server Git sync after a normal confirmation that identifies the active connection profile and explains that the request may advance the Release ref.

<!-- verification-id: OS-DFA-005 -->
#### Scenario: Confirmed Git sync is requested once
- **WHEN** an authorized Operator confirms Git sync for the active profile
- **THEN** the application invokes the typed Git sync method once, reports that the server accepted the request, and refreshes Release ref and Activity evidence

<!-- verification-id: OS-DFA-006 -->
#### Scenario: Canceled Git sync changes nothing
- **WHEN** an operator cancels the Git sync confirmation
- **THEN** no Admin API request is sent and the current workspace remains unchanged

<!-- verification-id: OS-DFA-007 -->
#### Scenario: Git sync failure does not imply Release ref advancement
- **WHEN** the server rejects or cannot complete the Git sync request
- **THEN** the application reports the classified failure and continues showing the last observed Release ref with its original evidence timestamp

### Requirement: Enrollment token creation handles a one-time secret
The desktop application SHALL allow an authorized Operator to create an Endpoint enrollment token for an exact existing Fleet and positive server-supported TTL, SHALL display the returned token only in a transient result surface, and SHALL never persist or log the token.

<!-- verification-id: OS-DFA-008 -->
#### Scenario: Enrollment token is shown once with scope and expiry
- **WHEN** the server successfully creates an enrollment token
- **THEN** the result surface displays the exact Fleet, expiry, and token, identifies it as one-time sensitive material, and offers an explicit native clipboard copy action

<!-- verification-id: OS-DFA-009 -->
#### Scenario: Closing token result clears application state
- **WHEN** the operator closes the result, switches profile, or exits the application
- **THEN** the token is cleared from frontend and backend application state and is absent from browser persistence, desktop settings, logs, Activity detail, and later error messages

<!-- verification-id: OS-DFA-010 -->
#### Scenario: Explicit clipboard copy does not create hidden persistence
- **WHEN** the operator activates Copy token
- **THEN** the native clipboard receives the token only as the direct result of that action, the interface warns that clipboard contents are outside Remotr's persistence boundary, and no second copy is stored by the application

<!-- verification-id: OS-DFA-011 -->
#### Scenario: Invalid enrollment scope is rejected locally
- **WHEN** the selected Fleet is empty or unknown in the current workspace or the TTL is not positive
- **THEN** the application requires a valid scope before enabling submission and the backend independently rejects the invalid request without contacting the mutation endpoint

### Requirement: Endpoint Labels are edited through the existing contract
The desktop application SHALL allow an authorized Operator to add, replace, or remove Endpoint Labels using the existing label-key and label-value validation rules and the current Endpoint identity.

<!-- verification-id: OS-DFA-012 -->
#### Scenario: Label set refreshes the selected Endpoint
- **WHEN** an operator submits a valid key and value for an existing Endpoint
- **THEN** the application sends the typed set-label request, reports whether the server added or replaced the Label, and refreshes the selected Endpoint and matching inventory columns

<!-- verification-id: OS-DFA-013 -->
#### Scenario: Invalid Label is rejected at both desktop layers
- **WHEN** a Label key starts with a dot, contains forbidden whitespace or equals, exceeds the key limit, or the value exceeds its limit
- **THEN** frontend guidance and backend validation reject the same input and no Admin API request is sent

<!-- verification-id: OS-DFA-014 -->
#### Scenario: Label removal identifies the exact key
- **WHEN** an operator confirms removal of one existing Label key from an Endpoint
- **THEN** only that Endpoint and key are sent to the remove-label API and the refreshed Endpoint retains every other Label

### Requirement: Agent upgrades distinguish request from completion
The desktop application SHALL allow an authorized Operator to request an exact agent version for one Endpoint or one Fleet and SHALL present the response as an upgrade request that applies on a later Sync, not as an installed version.

<!-- verification-id: OS-DFA-015 -->
#### Scenario: Endpoint upgrade identifies target and version
- **WHEN** an operator confirms an Endpoint upgrade with a non-empty version
- **THEN** the application sends that Endpoint ID and version once, labels the result requested, and refreshes desired and reported version evidence separately

<!-- verification-id: OS-DFA-016 -->
#### Scenario: Fleet upgrade requires scope confirmation
- **WHEN** an operator requests an agent upgrade for a Fleet
- **THEN** the confirmation names the Fleet, requested version, and current member count, and only an explicit confirmation invokes the Fleet upgrade API

<!-- verification-id: OS-DFA-017 -->
#### Scenario: Fleet upgrade result uses the server count
- **WHEN** the Fleet upgrade API succeeds
- **THEN** the application reports the number of Endpoints accepted by the server and does not substitute the locally cached Fleet count or claim that those Endpoints have completed the upgrade

### Requirement: Diagnostic collection is scoped and trackable
The desktop application SHALL allow an authorized Operator to request supported diagnostic collectors for one Endpoint over a valid time interval, display the resulting request identity and lifecycle, and save a ready bundle only through a native file-save operation.

<!-- verification-id: OS-DFA-018 -->
#### Scenario: Diagnostic request preview is exact
- **WHEN** an operator prepares diagnostic collection
- **THEN** the confirmation names the Endpoint, every selected collector, and the absolute since and until timestamps that will be sent

<!-- verification-id: OS-DFA-019 -->
#### Scenario: Invalid diagnostic interval sends no request
- **WHEN** no collector is selected, the until time is not after the since time, or the interval exceeds the server-supported bound
- **THEN** the application shows field-specific guidance and sends no diagnostic collection request

<!-- verification-id: OS-DFA-020 -->
#### Scenario: Active diagnostic conflict is explicit
- **WHEN** the server reports that the Endpoint already has an active diagnostic request
- **THEN** the application presents the conflict as the existing-work condition, does not retry automatically, and offers to inspect the known request when its identity is available

<!-- verification-id: OS-DFA-021 -->
#### Scenario: Ready diagnostic bundle is saved natively
- **WHEN** a diagnostic request is ready and the operator chooses a destination through the native save dialog
- **THEN** the backend downloads the bundle directly to the chosen file, verifies available size and digest metadata, and returns only save status and path metadata rather than bundle bytes to ordinary frontend state

<!-- verification-id: OS-DFA-022 -->
#### Scenario: Diagnostic bundle is not saved before ready
- **WHEN** the operator requests a download for a pending, failed, expired, or missing diagnostic request
- **THEN** the application does not create a misleading destination file and reports the exact lifecycle condition

### Requirement: Endpoint removal requires typed identity confirmation
The desktop application SHALL require an operator to type the exact Endpoint ID before removal and the Go backend SHALL independently compare the confirmation to the target before issuing the existing delete request.

<!-- verification-id: OS-DFA-023 -->
#### Scenario: Exact Endpoint ID permits removal request
- **WHEN** an authorized Operator enters the exact case-sensitive Endpoint ID and confirms removal
- **THEN** the backend issues one delete request for that Endpoint, closes its detail surface after success, removes it from refreshed inventory, and reports that its credential is no longer enrolled

<!-- verification-id: OS-DFA-024 -->
#### Scenario: Mismatched Endpoint ID blocks removal
- **WHEN** the confirmation is empty or differs from the exact target ID
- **THEN** both the interface and backend refuse the operation and no delete request is sent

<!-- verification-id: OS-DFA-025 -->
#### Scenario: Removal failure preserves the Endpoint
- **WHEN** the server rejects Endpoint removal or the request fails
- **THEN** the detail surface stays open, the Endpoint remains in inventory, the confirmation text is cleared, and the classified failure is displayed without implying revocation

### Requirement: Server RBAC and audit remain authoritative for actions
Every desktop action SHALL use the connected Operator credential, SHALL preserve server forbidden responses, and SHALL rely on existing server audit generation rather than writing an unauthenticated or client-authored audit substitute.

<!-- verification-id: OS-DFA-026 -->
#### Scenario: Forbidden action leaves state unchanged
- **WHEN** the server rejects a desktop action as forbidden
- **THEN** the application reports the authorization failure, keeps the session connected, and makes no local change that suggests the action succeeded

<!-- verification-id: OS-DFA-027 -->
#### Scenario: Action Activity comes from the server
- **WHEN** an action succeeds and the subsequent Activity refresh includes its audit event
- **THEN** the application displays the server-provided actor, action, resource, request identity, time, and status and does not fabricate a duplicate client event

### Requirement: Desired-state parity preserves the Git boundary
The desktop application SHALL NOT provide a direct server-side YAML editor, direct Deployable artifact mutation, automatic stage/commit/push/merge, or action that bypasses Configuration repository review. Later parity releases SHALL implement the Admin CLI's local repository scaffold, package scaffold/build, Hub import, validate, discover, and render behaviors through purpose-specific methods scoped to an operator-selected working tree.

<!-- verification-id: OS-DFA-028 -->
#### Scenario: CLI-equivalent local authoring stays inside the working tree
- **WHEN** a parity release scaffolds a Configuration repository or package, imports a Hub snippet, validates, discovers, or renders Configuration content
- **THEN** it reads or writes only the operator-selected local paths declared by that workflow, never stages, commits, pushes, merges, or directly applies the content, and explains that Git review remains required before server Git sync can advance the Release ref

<!-- verification-id: OS-DFA-029 -->
#### Scenario: Git sync does not become configuration editing
- **WHEN** an operator requests Git sync from the desktop application
- **THEN** the application invokes only the existing server sync operation and does not create, modify, stage, commit, or push repository files
