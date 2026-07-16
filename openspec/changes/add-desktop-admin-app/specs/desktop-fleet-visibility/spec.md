## ADDED Requirements

### Requirement: Persistent operations workspace
The desktop application SHALL present a compact persistent application shell with active connection context, Fleet scope, grouped Remotr navigation, page title and summary, filtering controls, and a main data surface sized for desktop fleet operations.

<!-- verification-id: OS-DFV-001 -->
#### Scenario: Connected shell preserves operational context
- **WHEN** an operator navigates among Overview, Endpoints, Fleets, Change requests, Diagnostics, and Activity
- **THEN** the active connection profile and Fleet scope remain visible while the main page changes without opening separate application windows

<!-- verification-id: OS-DFV-002 -->
#### Scenario: Minimum supported window remains usable
- **WHEN** the application is resized to 1100 by 720 logical pixels
- **THEN** primary navigation, the active page, row actions, filters, and overlay close controls remain keyboard and pointer accessible without overlapping or hiding critical content

### Requirement: Overview summarizes independently loaded fleet evidence
The Overview SHALL display Endpoint totals, compliance distribution, check-in freshness, pending or active Change requests, Fleet counts, and recent activity using existing Admin API evidence, and SHALL retain successful sections when a separately authorized or fetched section fails.

<!-- verification-id: OS-DFV-003 -->
#### Scenario: Complete overview snapshot is internally consistent
- **WHEN** all Overview Admin API requests succeed
- **THEN** the displayed totals and status counts match the returned Fleet, Endpoint, State report, Change request, and audit records without double-counting an Endpoint

<!-- verification-id: OS-DFV-004 -->
#### Scenario: One forbidden section does not blank the overview
- **WHEN** Endpoint and Fleet data load but audit activity is forbidden for the current Operator
- **THEN** the Overview displays the available fleet evidence, marks Activity unavailable due to authorization, and does not present the whole connection as failed

<!-- verification-id: OS-DFV-005 -->
#### Scenario: Overview counts link to their source rows
- **WHEN** an operator activates a compliance, freshness, Fleet, or Change request summary count
- **THEN** the application opens the corresponding page with an equivalent filter applied

### Requirement: Endpoint inventory uses canonical compliance state
The Endpoint inventory SHALL show Endpoint ID, Fleet, State report status, check-in freshness, reported and desired agent versions, Release ref, selected Labels, and last evidence time, with canonical compliance derived from the current State report rather than inferred only from historical summary fields.

<!-- verification-id: OS-DFV-006 -->
#### Scenario: Endpoint statuses preserve Admin API distinctions
- **WHEN** State reports contain compliant, drifted, unsupported, check-failed, deferred, apply-failed, and no-report statuses
- **THEN** the table renders those statuses as distinct text labels and does not collapse them into a single healthy or unhealthy boolean

<!-- verification-id: OS-DFV-007 -->
#### Scenario: Endpoint without a State report is not declared healthy
- **WHEN** an enrolled Endpoint has check-in metadata but no State report
- **THEN** the inventory labels its compliance as not reported and shows the independent check-in freshness

<!-- verification-id: OS-DFV-008 -->
#### Scenario: Selected labels are deterministic
- **WHEN** Endpoints report different Label keys
- **THEN** the default Label columns use a deterministic configured key order and the operator can inspect all remaining key-value pairs in Endpoint detail

### Requirement: Check-in freshness is not connection presence
The application SHALL classify the latest Endpoint check-in as recent, stale, or never reported using one injectable clock and a locally configured threshold that defaults to ten minutes, and SHALL not label an Endpoint online or offline solely from that timestamp.

<!-- verification-id: OS-DFV-009 -->
#### Scenario: Freshness boundary is deterministic
- **WHEN** an Endpoint's latest check-in age is exactly the configured threshold
- **THEN** it is recent, and it becomes stale only when the age exceeds that threshold

<!-- verification-id: OS-DFV-010 -->
#### Scenario: Endpoint has never checked in
- **WHEN** an Endpoint has no latest check-in timestamp
- **THEN** the application displays never reported without inventing a last-seen time or an offline duration

<!-- verification-id: OS-DFV-011 -->
#### Scenario: Compliance and freshness remain independent
- **WHEN** an Endpoint has a compliant State report but its latest check-in is stale
- **THEN** the interface shows both compliant and stale rather than changing compliance to failed or claiming the Endpoint is offline

### Requirement: Inventory filtering and sorting are predictable
Endpoint and Fleet tables SHALL support case-insensitive text search, explicit Fleet and status filters, sortable documented columns, clear-all behavior, and a visible result count without changing server state.

<!-- verification-id: OS-DFV-012 -->
#### Scenario: Endpoint search matches operator-visible fields
- **WHEN** an operator searches by a substring from Endpoint ID, Fleet, username, Label key, or Label value
- **THEN** the table shows exactly the rows containing that substring case-insensitively in at least one of those fields and displays the filtered and total counts

<!-- verification-id: OS-DFV-013 -->
#### Scenario: Combined filters use intersection semantics
- **WHEN** an operator applies Fleet, compliance, freshness, and text filters together
- **THEN** a row remains visible only when it satisfies every active filter

<!-- verification-id: OS-DFV-014 -->
#### Scenario: Stable sort resolves equal values by identity
- **WHEN** multiple rows have equal values in the selected sort column
- **THEN** the application orders equal rows by Endpoint or Fleet identity so refresh does not arbitrarily reorder them

<!-- verification-id: OS-DFV-015 -->
#### Scenario: Keyboard shortcut focuses current-page search
- **WHEN** focus is not in an editor or modal and the operator presses slash or the documented platform search shortcut
- **THEN** focus moves to the current page search input without entering the shortcut character as query text

### Requirement: Endpoint detail is a focused investigation surface
Activating an Endpoint row SHALL open a large focused detail surface that lazily loads the Endpoint record and its State, schedule, firewall, and system evidence, preserves the underlying inventory context, and separates each evidence class into a labeled tab.

<!-- verification-id: OS-DFV-016 -->
#### Scenario: Endpoint overview identifies the selected resource
- **WHEN** an operator opens Endpoint detail
- **THEN** the detail header shows the exact Endpoint ID, Fleet, compliance, freshness, agent version, Release ref, and available Labels before any mutating control

<!-- verification-id: OS-DFV-017 -->
#### Scenario: Detail tabs preserve partial evidence
- **WHEN** State and system information load but schedule or firewall evidence is unavailable or forbidden
- **THEN** the available tabs remain usable and each unavailable tab shows its own classified state without closing the detail surface

<!-- verification-id: OS-DFV-018 -->
#### Scenario: Closing detail restores inventory context
- **WHEN** the operator presses Escape or activates the visible close control on the topmost Endpoint detail surface
- **THEN** the surface closes, focus returns to the originating row, and the previous search, filters, sort, and scroll position remain intact

<!-- verification-id: OS-DFV-019 -->
#### Scenario: New selection cancels obsolete detail work
- **WHEN** the operator selects a different Endpoint before the prior detail requests finish
- **THEN** the application cancels or ignores the obsolete responses and never renders prior-Endpoint evidence under the newly selected identity

### Requirement: Fleet detail summarizes member evidence
The Fleets page SHALL show each Fleet's Endpoint count and current State report distribution, and Fleet detail SHALL list member Endpoints, agent-version distribution, freshness distribution, and applicable Fleet-level actions using the same status definitions as Endpoint inventory.

<!-- verification-id: OS-DFV-020 -->
#### Scenario: Fleet summary matches member rows
- **WHEN** a Fleet contains Endpoints with multiple compliance and freshness states
- **THEN** Fleet counts equal the visible member evidence and selecting a count filters the member table to that exact state

<!-- verification-id: OS-DFV-021 -->
#### Scenario: Empty Fleet is explicit
- **WHEN** the Admin API returns a Fleet with no enrolled Endpoints
- **THEN** the Fleet remains visible with zero counts and detail explains that no Endpoints are enrolled rather than treating the Fleet as missing

### Requirement: Change request visibility begins read-only and advances to parity
The first desktop release SHALL list and inspect existing Change requests, rollout state, target Fleet and Endpoint scope, risk, approvals, windows, progress, and outcomes without exposing authorization, pause, resume, revoke, baseline, or adoption mutations. Later feature releases SHALL add those Admin CLI-equivalent mutations as dedicated safety-reviewed action slices before the desktop can claim CLI parity.

<!-- verification-id: OS-DFV-022 -->
#### Scenario: Change request list preserves risk and lifecycle
- **WHEN** the Admin API returns Change requests in different risk and lifecycle states
- **THEN** the list renders the exact server-provided states, target counts, and most recent update without inventing an approval or completion state

<!-- verification-id: OS-DFV-023 -->
#### Scenario: Change request detail has no mutation controls
- **WHEN** an operator opens Change request detail in v1
- **THEN** the application displays available plan, approval, authorization, execution, and outcome evidence but offers no control that invokes a Change request lifecycle or baseline mutation

### Requirement: Activity is a bounded audit view
The Activity page SHALL use the existing audit-event API to show a bounded page of event time, actor, action, resource, status, and request identity, with server-backed cursor pagination and supported filters.

<!-- verification-id: OS-DFV-024 -->
#### Scenario: Next activity page uses the server cursor
- **WHEN** an audit response includes a next cursor and the operator requests more activity
- **THEN** the application sends that cursor with the active filters, appends no duplicate event IDs, and preserves the server's event order

<!-- verification-id: OS-DFV-025 -->
#### Scenario: Audit detail is safely formatted
- **WHEN** an audit event includes structured details
- **THEN** the application renders bounded non-secret fields as formatted data and does not reinterpret them as executable markup, links, or frontend code

<!-- verification-id: OS-DFV-026 -->
#### Scenario: Audit authorization failure is local to Activity
- **WHEN** the current Operator cannot list audit events
- **THEN** Activity displays an authorization-specific empty state while Endpoint, Fleet, and other permitted pages remain usable

### Requirement: Refresh preserves truthfulness and context
The application SHALL support explicit refresh and a visible-workspace automatic refresh cadence of thirty seconds, SHALL pause automatic refresh while hidden, and SHALL retain the last successful in-memory snapshot with a stale-data warning when a later refresh fails.

<!-- verification-id: OS-DFV-027 -->
#### Scenario: Successful refresh replaces a snapshot atomically
- **WHEN** a refresh returns a complete new result for a page section
- **THEN** the application replaces that section with the new rows and timestamps without briefly combining counts from different snapshots

<!-- verification-id: OS-DFV-028 -->
#### Scenario: Failed refresh retains visibly stale data
- **WHEN** a page has a successful snapshot and the next refresh fails
- **THEN** the application retains the last successful rows, displays when they were loaded and when refresh failed, and does not label them current

<!-- verification-id: OS-DFV-029 -->
#### Scenario: Hidden window does not poll
- **WHEN** the application window is hidden or suspended across one or more automatic refresh intervals
- **THEN** no scheduled workspace refresh begins until the window becomes visible, at which point one immediate refresh may run

<!-- verification-id: OS-DFV-030 -->
#### Scenario: Refresh shortcut respects editing context
- **WHEN** the operator invokes the documented refresh shortcut outside an editor
- **THEN** one refresh begins, while the same keystroke inside a text or code input leaves the input untouched and does not refresh

### Requirement: Loading, empty, and error states preserve orientation
Every data surface SHALL provide labeled loading, empty, partial, stale, authorization-failed, connection-failed, and unexpected-error states inside the persistent shell, and SHALL provide a relevant recovery action when recovery is possible.

<!-- verification-id: OS-DFV-031 -->
#### Scenario: Empty inventory is not a connection failure
- **WHEN** the authenticated Admin API returns zero Endpoints
- **THEN** the Endpoints page confirms that the connection succeeded, explains that no Endpoints are enrolled, and offers the permitted enrollment-token workflow rather than a generic error

<!-- verification-id: OS-DFV-032 -->
#### Scenario: Initial connection failure replaces operational content
- **WHEN** no successful workspace snapshot exists and connection verification or initial inventory load fails
- **THEN** the application shows the selected profile and classified connection recovery actions without showing demo or fabricated fleet rows

### Requirement: Dense presentation remains accessible
The desktop interface SHALL communicate status with text and iconography in addition to color, maintain visible keyboard focus and labeled controls, meet the project's contrast target, honor reduced-motion preference, and use semantic table and dialog behavior for the supported window sizes.

<!-- verification-id: OS-DFV-033 -->
#### Scenario: Keyboard-only Endpoint investigation
- **WHEN** an operator uses only the keyboard to filter Endpoints, select a row, traverse detail tabs, and close the detail surface
- **THEN** every step has visible focus, the active tab and dialog semantics are announced, and focus returns to the originating row

<!-- verification-id: OS-DFV-034 -->
#### Scenario: Status remains understandable without color
- **WHEN** color is unavailable or cannot be distinguished
- **THEN** compliance, freshness, progress, warning, and error states remain identifiable from their text labels and icons

<!-- verification-id: OS-DFV-035 -->
#### Scenario: Reduced motion disables nonessential transitions
- **WHEN** the operating system requests reduced motion
- **THEN** page and overlay transitions complete without animated movement while loading, focus, and progress feedback remain perceivable
