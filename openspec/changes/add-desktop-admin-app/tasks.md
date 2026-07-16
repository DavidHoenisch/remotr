## 1. Register the Desktop Contract

- [x] 1.1 Add truthful planned traceability entries for OS-DOA-001 through OS-DOA-019, OS-DFV-001 through OS-DFV-035, and OS-DFA-001 through OS-DFA-029 with initial verification classes and implementation-dependent selector dispositions.
- [x] 1.2 Update `docs/testing/public-seams.md` to define the desktop user/Wails bridge seam, its relationship to the authenticated Admin API seam, and examples of acceptable versus implementation-coupled evidence.
- [x] 1.3 Update `CONTEXT.md` domain language to define Remotr Desktop, replace the obsolete “Admin UI not planned” statement, preserve Admin CLI terminology, and list the fourth binary without calling the app a hosted Admin UI.
- [x] 1.4 Add the desktop boundary and additive release artifact to the architecture and operator-overview documentation before implementation claims it is supported.
- [x] 1.5 Generate and commit a machine-readable behavioral parity inventory from the current non-hidden `remotr` command tree, mapping every operator workflow to implemented, planned, or reviewed not-applicable status; include target feature release, OpenSpec verification IDs, passing selectors when implemented, and any deliberate desktop safety difference.
- [x] 1.6 Add a parity-drift check that fails when a non-hidden CLI capability is unmapped, an implemented capability loses evidence, a planned target is omitted, or a new CLI capability lands without a desktop disposition in the same change; treat shell completion, output formatting, flag spelling, and exit-code mechanics as reviewable interface differences rather than automatic desktop gaps.

## 2. Establish the Isolated Desktop Build

- [x] 2.1 For OS-DOA-001 and OS-DOA-002, add and run a focused failing build-layout check proving the standalone desktop module/artifact and embedded native entrypoint are not yet present; record the intended red result.
- [x] 2.2 Create the minimal `desktop/` nested Go module, pin the reviewed Wails v2 release, add `wails.json`, application metadata, embedded frontend assets, and a `remotr-desktop` main entrypoint to make the focused build-layout check green.
- [x] 2.3 Add the pinned React, TypeScript, Vite, icon, IBM Plex font, Vitest, Testing Library, and browser-test dependencies with committed lockfiles and no remote runtime asset dependency.
- [x] 2.4 Configure the native window title, application identity, minimum 1100-by-720 size, production asset embedding, release developer-tool policy, and remote-navigation guard.
- [x] 2.5 Add a typed frontend adapter around generated Wails bindings plus an injectable deterministic bridge fixture for component and browser-mode tests.
- [x] 2.6 Add desktop build-output and local-cache ignores while retaining desktop source, sums, lockfiles, generated metadata required for repeatable builds, and Linux application assets.
- [x] 2.7 Prove the root Go module and existing `remotr` CLI test/build surface remain independent of Wails and frontend dependencies, and prove desktop Go tests run explicitly from the nested module.

## 3. Implement Operator Profiles and Authentication

- [x] 3.1 For OS-DOA-003 through OS-DOA-005, add and run one focused failing profile test for standard-config import, allowlisted persistence, owner-only permissions, and invalid profile rejection through the desktop service seam.
- [x] 3.2 Implement the profile model/store and standard Operator-config import needed to make that profile slice green, including absolute-path and HTTPS validation and atomic owner-only writes.
- [x] 3.3 For OS-DOA-006 and OS-DOA-007, add and run a focused failing profile-switch test that uses controlled cancellation and independently known server identities.
- [x] 3.4 Implement the session manager that cancels obsolete work and clears Operator identity, snapshots, selections, overlays, and transient results before connecting the new profile.
- [x] 3.5 For OS-DOA-008 through OS-DOA-010, add and run focused failing connection tests through the real Admin client at a controlled network/TLS boundary for success, missing credentials, unknown CA, expired credential, unreachable server, and forbidden identity.
- [x] 3.6 Implement authenticated client creation, `GET /v1/admin/me` verification, safe error classification, and non-secret connection view models to make the connection slice green.
- [x] 3.7 For OS-DOA-011 through OS-DOA-013, add and run a failing bootstrap seam test with synthetic token/private-key canaries covering success, API rejection, persistence failure, cancellation, and partial-file cleanup.
- [x] 3.8 Implement typed bootstrap exchange and protected existing-layout persistence, clear transient token state on every terminal path, and make the bootstrap canary tests green.
- [x] 3.9 For OS-DOA-014 and OS-DOA-015, add and run a failing bridge-security test that inventories bound methods, scans view models for canaries, and attempts release remote navigation/content loading.
- [x] 3.10 Implement the purpose-specific Wails binding allowlist, safe view-model mappings, embedded-only release content policy, and external-link handoff needed to make the bridge-security slice green.
- [x] 3.11 For OS-DOA-016 and OS-DOA-017, add and run a failing authorization-behavior test proving a forbidden action leaves the authenticated session connected and cannot be bypassed through frontend state.
- [x] 3.12 Implement Operator identity/role presentation and server-authoritative forbidden handling without adding a client-side authorization bypass or alternate identity retry.

## 4. Build the Desktop Application Service

- [x] 4.1 Define safe typed desktop models for Operator identity, section results, Endpoint rows, Fleet summaries, State evidence, Change request summaries, audit activity, action results, classified errors, and snapshot timestamps without leaking raw credential or diagnostic content.
- [x] 4.2 For OS-DFV-003 and OS-DFV-004, add and run a focused failing workspace-load test through the real Admin client and controlled network boundary for complete results and one independently forbidden section.
- [x] 4.3 Implement bounded concurrent loading of identity, Fleets, Endpoints, Fleet State reports, Change requests, and the first audit page with shared cancellation and per-section errors to make the workspace slice green.
- [x] 4.4 For OS-DFV-006 through OS-DFV-011, add and run table-driven failing tests for every State report status, no-report behavior, deterministic Label ordering, and recent/stale/never freshness at the exact ten-minute boundary using an injected clock.
- [x] 4.5 Implement canonical compliance mapping, independent freshness mapping, stable Label projection, and local freshness preferences to make the status slice green without using “online” or “offline.”
- [x] 4.6 For OS-DFV-016 through OS-DFV-019, add and run a failing Endpoint-detail service test for partial State/schedule/firewall/system evidence and cancellation of an obsolete selection.
- [x] 4.7 Implement lazy Endpoint-detail loading with per-tab results, identity checks, bounded requests, and stale-response suppression to make the detail-service slice green.
- [ ] 4.8 For OS-DFV-020 and OS-DFV-021, add and run a failing Fleet-detail service test for mixed-status members and an empty Fleet, then implement consistent member/version/freshness aggregation to green.
- [ ] 4.9 For OS-DFV-022 through OS-DFV-026, add and run a failing read-only Change request and cursor-audit service test, then implement bounded safe mappings and cursor pagination without any Change lifecycle mutation binding.
- [ ] 4.10 Add a native Go benchmark with allocation reporting for workspace composition at representative 10, 100, 500, and 1,000 Endpoint fixtures and record the controlled baseline without deriving correctness expectations from benchmark output.

## 5. Create the Visual Shell and Overview

- [ ] 5.1 Establish the quiet operations-console design tokens, bundled typography, spacing, border, status, elevation, focus, motion, and minimum-window rules in one documented frontend theme.
- [ ] 5.2 For OS-DFV-001 and OS-DFV-002, add and run a failing semantic component test for persistent profile/Fleet context, grouped navigation, minimum-size access, and page changes.
- [ ] 5.3 Implement the top connection bar, approximately 224-pixel grouped navigation rail, page header, content frame, overlay layer, and responsive minimum-size behavior to make the shell test green.
- [ ] 5.4 For OS-DFV-003 through OS-DFV-005, add and run a failing Overview component test for consistent counts, a forbidden Activity section, and summary-to-filter navigation.
- [ ] 5.5 Implement the information-dense Overview summary strip, compliance/freshness visualization, Fleet and Change request summaries, recent Activity, progressive section states, and linked filters to make the Overview slice green.
- [ ] 5.6 Add loading, empty, partial, stale, authorization, connection, and unexpected-error primitives that preserve shell context and expose safe recovery controls.

## 6. Implement Inventory and Investigation Views

- [ ] 6.1 For OS-DFV-006 through OS-DFV-008, add and run a failing Endpoint-table component test covering all canonical status labels, independent freshness, versions, Release ref, selected Labels, and no-report evidence.
- [ ] 6.2 Implement the dense semantic Endpoint table, status tokens with text/icons, column selection, result count, stable row identity, and zero-Endpoint state to make the inventory rendering slice green.
- [ ] 6.3 For OS-DFV-012 through OS-DFV-015, add and run a failing filtering test for case-insensitive visible-field search, intersected filters, stable identity tie-breaking, clear-all, and search-focus shortcuts.
- [ ] 6.4 Implement reusable table search/filter/sort state and keyboard focus behavior while preserving the current page's Fleet scope and result count.
- [ ] 6.5 For OS-DFV-016 through OS-DFV-019, add and run a failing user-flow test for opening Endpoint detail, partial tab evidence, changing selection mid-load, Escape/close behavior, and focus/filter/scroll restoration.
- [ ] 6.6 Implement the large focused Endpoint overlay with Overview, State, Schedules, Firewall, and System tabs, per-tab states, safe structured fields, cancellation, and origin-focus restoration.
- [ ] 6.7 For OS-DFV-020 and OS-DFV-021, add and run a failing Fleet list/detail component test, then implement Fleet status distributions, member filters, agent-version/freshness summaries, and the explicit empty-Fleet state.
- [ ] 6.8 For OS-DFV-022 and OS-DFV-023, add and run a failing Change request list/detail component test, then implement exact read-only lifecycle/risk/approval/window/progress/outcome presentation with no first-release mutation controls.
- [ ] 6.9 For OS-DFV-024 through OS-DFV-026, add and run a failing Activity test for cursor pagination, deduplication, filter preservation, safe structured-detail rendering, and authorization-local failure.
- [ ] 6.10 Implement the Activity table and detail surface with bounded pages, exact server order, safe formatting, and no executable audit markup.

## 7. Add Refresh, Keyboard, Accessibility, and Visual Evidence

- [ ] 7.1 For OS-DFV-027 through OS-DFV-030, add and run deterministic failing refresh tests with injected clock/visibility for atomic replacement, stale retention, hidden-window pause, immediate resume, and editing-safe shortcuts without wall-clock sleeps.
- [ ] 7.2 Implement the visible-workspace 30-second refresh controller, one-request guard, section-level atomic replacement, stale banners, hidden pause, and editing-aware shortcut handling to make the refresh slice green.
- [ ] 7.3 For OS-DFV-031 and OS-DFV-032, add and run a failing state-orientation flow for zero Endpoints and initial connection failure, then implement specific recovery copy/actions without demo or fabricated rows.
- [ ] 7.4 For OS-DFV-033 through OS-DFV-035, add and run automated keyboard, semantic role/name, focus-return, non-color status, contrast, and reduced-motion checks for the shell, inventory, and topmost overlay.
- [ ] 7.5 Fix the UI and interaction semantics until the accessibility slice passes at the default and minimum supported window sizes.
- [ ] 7.6 Add bounded visual-regression coverage at representative 1440-by-900 and 1100-by-720 viewports for populated inventory, partial Overview, Endpoint detail, connection failure, and destructive confirmation without using screenshots as the sole assertion.

## 8. Implement the Common Action Contract and Git Sync

- [ ] 8.1 For OS-DFA-001 through OS-DFA-004, add and run a failing action-controller test for single submission, backend validation, safe error context, exact acknowledged result, affected refresh, and no premature convergence claim.
- [ ] 8.2 Implement the reusable frontend action state machine and typed backend error/result envelope needed to make the common action slice green.
- [ ] 8.3 For OS-DFA-005 through OS-DFA-007 and OS-DFA-029, add and run a failing Git sync user-flow test for confirm, cancel, failure, accepted refresh, and absence of repository writes.
- [ ] 8.4 Implement the typed Git sync backend method, active-profile confirmation UI, server-accepted result, Release ref/Activity refresh, and failure retention without any local Git or Configuration mutation.

## 9. Implement Enrollment and Label Actions

- [ ] 9.1 For OS-DFA-008 through OS-DFA-011, add and run a failing enrollment-token workflow with a synthetic token canary for Fleet/TTL validation, one-time display, explicit clipboard copy, close/profile/exit clearing, and persistence/log scans.
- [ ] 9.2 Implement typed enrollment-token creation, transient result ownership, explicit native clipboard copy, external-clipboard warning, and every required clearing path to make the secret-canary slice green.
- [ ] 9.3 For OS-DFA-012 through OS-DFA-014, add and run a failing Label workflow for add, replace, remove, exact Endpoint/key targeting, and all current key/value validation boundaries.
- [ ] 9.4 Implement Label editor UI and typed set/remove backend methods using the existing validation package, then refresh only the selected Endpoint, affected columns, and Activity.

## 10. Implement Upgrade and Diagnostic Actions

- [ ] 10.1 For OS-DFA-015, add and run a failing Endpoint-upgrade flow proving exact version/target submission and the distinction among requested, desired, reported, and completed states.
- [ ] 10.2 Implement the Endpoint upgrade confirmation, typed request, requested result, and evidence refresh needed to make the Endpoint slice green.
- [ ] 10.3 For OS-DFA-016 and OS-DFA-017, add and run a failing Fleet-upgrade flow for exact Fleet/version/member-count confirmation and server-returned accepted count.
- [ ] 10.4 Implement Fleet upgrade confirmation and typed request without substituting cached member count for the result or claiming completion.
- [ ] 10.5 For OS-DFA-018 through OS-DFA-020, add and run a failing diagnostic-collection flow covering collector preview, absolute time bounds, empty/invalid intervals, server-supported limits, and active-request conflict.
- [ ] 10.6 Implement typed diagnostic request validation, confirmation, lifecycle presentation, and conflict handling without automatic duplicate submission.
- [ ] 10.7 For OS-DFA-021 and OS-DFA-022, add and run a failing native-save test for a ready digest/size-described bundle and pending, failed, expired, or missing requests, including interrupted-write cleanup.
- [ ] 10.8 Implement direct backend download to a native-selected temporary destination, size/digest verification when present, atomic final placement, cleanup on failure, and a metadata-only frontend result.

## 11. Implement Endpoint Removal and Action Authority

- [ ] 11.1 For OS-DFA-023 through OS-DFA-025, add and run a failing removal flow for exact case-sensitive typed confirmation, backend revalidation, successful inventory removal, mismatch no-request, and failure retention/confirmation clearing.
- [ ] 11.2 Implement the destructive confirmation surface and typed backend Endpoint removal method, then refresh inventory/Activity and restore focus only after server success.
- [ ] 11.3 For OS-DFA-026 and OS-DFA-027, add and run a failing cross-action authorization/audit test proving forbidden actions leave state unchanged and successful Activity rows come only from server audit events.
- [ ] 11.4 Implement consistent forbidden handling and post-action server Activity refresh across every action without fabricating client-authored audit records.
- [ ] 11.5 For OS-DFA-028, add and run a UI/binding inventory test proving no desired-state editor, repository writer, artifact mutation, Change authorization, Secret, package-upload, RBAC, or Operator-issuance action exists in the first release; mark every applicable deferred workflow as planned in the parity inventory rather than permanently unsupported.
- [ ] 11.6 Add the Git-only desired-state explanation at relevant empty/detail surfaces and keep the first-release backend action allowlist free of Configuration repository write primitives.

## 12. Close Behavioral CLI Parity in Versioned Slices

- [ ] 12.1 For every parity slice below, name the mapped CLI workflows and OpenSpec verification IDs, add one focused failing public-seam test, record the intended red result, implement only enough typed desktop behavior to make it green, and update the parity inventory and release target without weakening the CLI or desktop assertion.
- [ ] 12.2 Complete remaining read and export parity for Inventory save, Endpoint/Fleet State and cron reports, Firewall logs/report/export, audit log list/export information, and diagnostics lifecycle evidence, using native save destinations and bounded structured views instead of CLI stdout formats.
- [ ] 12.3 Complete reusable deployment-token create/list/show/revoke parity with one-time secret handling, protected persistence where selected, destructive confirmation, redaction canaries, and server-authoritative results.
- [ ] 12.4 Complete Change request watch, authorize, pause, resume, revoke, baseline-promote, and baseline-adopt parity only after a focused threat review; preserve bounded rollout controls, justification, exact resource confirmation, exception acknowledgement, audit evidence, and deterministic polling without wall-clock sleeps.
- [ ] 12.5 Complete application-package validate/publish/list/show/delete and local package create/build/publish parity with safe native file selection, integrity evidence, explicit object-deletion confirmation, and no credential exposure to the frontend.
- [ ] 12.6 Complete encrypted Secret upload/list/activate/revoke parity with protected native input, Fleet/Endpoint scope validation, version metadata only in view models, rollout-planning semantics, destructive confirmation, cleanup, and secret-canary evidence.
- [ ] 12.7 Complete canonical RBAC role list/create/show/delete, rule add/remove, Operator list/set-roles, and admin credential-stamp parity with server-authoritative authorization, exact destructive confirmation, protected key output, role validation, and audit refresh; hidden compatibility aliases remain outside the parity contract.
- [ ] 12.8 Complete setup and operator-maintenance parity for configuration show/path/init, bootstrap and enrollment variants, doctor diagnostics, documentation access, version display, and desktop self-update/check behavior appropriate to Linux releases.
- [ ] 12.9 Complete Configuration repository init/validate/discover/render and Hub snippet-import parity through shared Go packages inside an explicitly selected local working tree; never shell out to `remotr`, directly mutate server desired state, or automatically stage, commit, push, merge, or apply generated content.
- [ ] 12.10 Complete AI integration setup/list/upgrade parity for supported agent runtimes with explicit user-selected scope, version and replacement controls, filesystem-boundary tests, and safe recovery when an external runtime is absent.
- [ ] 12.11 Before each desktop feature release, run the parity-drift gate and publish its inventory: do not claim full CLI parity until every current non-hidden applicable workflow is implemented with passing public-seam evidence or has an approved not-applicable rationale limited to interface mechanics.

## 13. Finish Linux Delivery and Documentation

- [ ] 13.1 Add root `desktop-test`, `desktop-dev`, and `desktop-build` targets with pinned setup, lockfile, nested-module, frontend, and Linux GTK/WebKit prerequisite behavior.
- [ ] 13.2 Add affected-pull-request Linux CI for desktop Go tests, frontend type/lint/unit/browser checks, bridge-security canaries, visual regression, production frontend build, and parity drift while retaining root `make test`.
- [ ] 13.3 Add versioned Wails application icons and Linux-only packaging metadata that consistently identify Remotr Desktop without copying the reference application's branding; add no macOS or Windows jobs, metadata, installers, or release assets.
- [ ] 13.4 For OS-DOA-018, add native Linux build/launch smoke evidence and gate every Linux artifact advertisement on its result.
- [ ] 13.5 Add native build/launch/install/remove smoke evidence for every advertised Linux architecture and package format, clearly distinguishing unsigned development snapshots from signed release output.
- [ ] 13.6 Add a release-manifest check that rejects macOS or Windows desktop artifacts and rejects any Linux package format or architecture without matching native evidence.
- [ ] 13.7 Document developer setup, Linux prerequisites, profile and credential reuse, bootstrap handling, Linux-only support, Git-bound desired-state workflows, current parity status, troubleshooting, and CLI fallback/recovery.
- [ ] 13.8 Update release documentation and automation so desktop publication is additive and Linux-only, evidence and signing policy are truthful, and stopping desktop publication requires no server or credential migration.

## 14. Verify the Complete Desktop Change

- [ ] 14.1 Run strict OpenSpec validation, traceability lint, and the CLI parity-drift gate; resolve every missing, duplicate, malformed, orphaned, stale, or selector-less desktop verification record.
- [ ] 14.2 Run all focused desktop Go, frontend component, browser-mode, security-canary, accessibility, visual, and benchmark checks from a clean dependency state.
- [ ] 14.3 Run root fuzz seed corpora and `make test` to prove the existing server, agent, and Admin CLI behavior remains green and independent of the desktop dependency graph.
- [ ] 14.4 Build the production Linux desktop artifact and record Wails doctor/dependency output, build metadata, embedded version, install/launch smoke evidence, and the advertised architecture/package format.
- [ ] 14.5 Review every OS-DOA, OS-DFV, and OS-DFA scenario against passing selectors or an approved evidence exception, and do not advertise incomplete first-release capabilities, parity status, Linux architectures, or package formats.
- [ ] 14.6 Perform a final credential/secret canary scan of logs, persisted profiles, built frontend assets, view-model fixtures, screenshots, browser storage, and failure artifacts before release handoff.
