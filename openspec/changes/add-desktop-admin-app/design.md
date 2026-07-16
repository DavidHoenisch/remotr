## Context

Remotr currently ships three Go programs: the `remotr` Admin CLI, `remotr-agent`, and `remotr-server`. Operators author desired state only through the Configuration repository and use the Admin CLI over the mTLS-protected Admin API for Server registry and PKI operations. That separation remains sound, but a CLI-only workflow makes it difficult to scan a fleet, compare Endpoint state, preserve investigation context, and move between related operational records.

The existing `internal/admin`, operator configuration, credential, TLS, and typed model packages already implement the remote contract needed by a desktop application. The server authenticates Operator credentials and authorizes every Admin API request with RBAC. The desktop application therefore does not need a new service or authentication scheme; it needs a deliberately narrow native application boundary over the existing client.

The requested visual references use a restrained infrastructure-console pattern: persistent connection context, a dense navigation rail, sortable tables, compact status tokens, and large focused overlays for resource details and actions. Remotr needs the same information density while retaining its own domain language: Fleet, Endpoint, Drift, Apply failure, Change request, Release ref, and Git sync.

Stakeholders are Fleet operators, security administrators, contributors, release engineers, and reviewers responsible for credential safety. Constraints include the repository's vertical-slice TDD policy, vendored root Go module, Linux GTK/WebKit packaging, the breadth of the existing Admin CLI, and the requirement that private key or secret material never become general frontend state.

## Goals / Non-Goals

**Goals:**

- Ship a dedicated native Remotr desktop executable and release artifact without changing the existing CLI binary.
- Reuse the Admin API, operator configuration, mTLS credentials, RBAC, and domain models rather than reproducing CLI parsing or invoking subprocesses.
- Provide a compact, polished, keyboard-accessible workspace for Fleet and Endpoint visibility and a bounded set of common operator actions.
- Keep the Go backend as the trust boundary and expose only purpose-built, typed view and action methods to the webview.
- Make connection, partial-data, stale-data, authorization, and action progress states explicit and testable.
- Keep routine root-module tests and builds independent of desktop-specific Go, Node, and native WebView dependencies.
- Build, test, package, and support Remotr Desktop on Linux only.
- Make every desktop feature release publish its behavioral parity against the current non-hidden Admin CLI surface and converge on complete parity without regressing already implemented workflows.

**Non-Goals:**

- Hosting a browser Admin UI or adding a remotely reachable desktop-specific service.
- Directly editing server-side desired state, auto-committing or pushing Configuration repository content, or bypassing Git review. Later parity releases may provide the CLI-equivalent local scaffold, validate, discover, render, package, and Hub import workflows inside an operator-selected working tree.
- Reaching complete Admin CLI feature parity in the first desktop release.
- Performing Change request authorization, baseline promotion, Secret lifecycle, app-package upload, RBAC administration, or Operator credential issuance in the first release; these are required later parity slices rather than permanent exclusions.
- Reading secret plaintext through the Admin API or exposing certificate private keys to frontend code.
- Replacing the Admin CLI for automation, scripting, recovery, or headless environments.
- Providing mobile or narrow-window layouts, a terminal emulator, remote shell, or Endpoint log streaming.
- Building, packaging, testing, or advertising macOS or Windows desktop artifacts as part of this change.
- Treating a WebView-rendered test as proof that native Linux packaging works.

## Decisions

### 1. The desktop application is an isolated Wails v2 project

Create `desktop/` as a nested Go module with its own `go.mod`, `go.sum`, `wails.json`, frontend lockfile, build assets, and CI commands. The module path remains beneath `github.com/DavidHoenisch/remotr`, allowing the application to reuse the repository's `internal` packages through a local module replacement during development. The executable and product name are `remotr-desktop` and “Remotr Desktop.” Wails v2 is pinned to the reviewed current release at implementation time rather than using `latest` in repeatable builds.

The frontend uses React with TypeScript and Vite. React is appropriate for the table, filter, overlay, async-state, and keyboard-interaction surface, while TypeScript gives a checked contract around generated Wails bindings. Dependencies remain small and pinned: the framework/runtime, build/test tooling, an icon set, and locally bundled font packages. No runtime assets load from a CDN.

The root Go module does not require Wails, and `make test` continues to exercise the current server, agent, and CLI tree without compiling WebView bindings. New explicit targets run desktop Go tests, frontend checks, development mode, and native builds.

Alternative considered: add `cmd/remotr-desktop` to the root module. Rejected because it would pull Wails, CGO/WebView, and frontend build concerns into the vendored root dependency and routine test surface.

Alternative considered: integrate a GUI mode into `remotr`. Rejected because it violates the requested binary boundary, complicates automation-focused CLI behavior, and couples two different distribution lifecycles.

Alternative considered: host a web dashboard in `remotr-server`. Rejected because it adds a remotely exposed attack surface and conflicts with the native desktop requirement.

### 2. A typed Go application service is the webview security boundary

The Wails-bound object exposes purpose-specific methods such as profile discovery, bootstrap, workspace load, Endpoint detail load, Git sync, label update, upgrade request, diagnostic request, and Endpoint removal. It does not expose `admin.Client`, an arbitrary HTTP fetcher, filesystem access, process execution, or generic method/path arguments.

The application service creates the existing Admin client from a selected profile and maps Admin API responses into desktop view models. Later parity slices may also call existing shared Go packages for local CLI-equivalent workflows such as configuration validation or package building, but they use purpose-specific typed methods and never invoke the CLI process. View models contain only fields required by the interface. PEM certificate bodies, private keys, bootstrap tokens, raw TLS configuration, and unrestricted diagnostic bundle bytes are never returned as ordinary view state. File save and copy operations use narrow native methods with explicit content and lifecycle rules.

Release builds embed the compiled frontend, disallow remote navigation, apply a restrictive content policy compatible with the Wails runtime, and disable developer tooling. Backend methods validate identifiers and action inputs even when the frontend has already validated them. Long-running calls accept application lifetime cancellation; changing profile or closing the app cancels obsolete work.

Alternative considered: let the frontend call the Admin API directly. Rejected because browser networking would require private key access or a new bearer-token design and would duplicate TLS and error handling.

Alternative considered: shell out to the Admin CLI and parse JSON. Rejected because process invocation is an unnecessary boundary, complicates cancellation and error classification, and makes the desktop app depend on a separately installed binary.

### 3. Connection profiles contain references, never credential material

On first launch, the app imports the resolved standard operator configuration as an implicit “Default” profile when available. Operators can add named profiles containing a server URL, Operator state directory, optional CA path, and an optional default Fleet. Profiles are stored in an owner-only desktop settings file. They reference the existing Operator credential layout; certificate and private-key bytes remain in the state directory managed by the existing credential package.

Selecting a profile constructs a fresh authenticated client and verifies identity through `GET /v1/admin/me`. A profile is not considered connected merely because files exist. Switching profiles clears in-memory operational data and transient action outputs before loading the new workspace.

If credentials are absent, the app offers first-operator bootstrap. The token necessarily exists in the focused frontend input while it is being entered, but it is never stored in browser persistence, desktop settings, logs, analytics, error text, or returned results. The Go backend exchanges it, saves the issued credential through the existing protected layout, zeroes/abandons its transient copy, and returns only Operator identity and connection state.

Alternative considered: copy credentials into a desktop-specific store. Rejected because duplicate private keys create ambiguous rotation and revocation behavior.

### 4. The UI consumes purpose-built snapshots and keeps freshness distinct from compliance

The initial workspace loads Operator identity, Fleets, Endpoints, Fleet state summaries, Change requests, and a bounded first page of audit activity. Independent sections report their own errors so an audit permission failure does not hide Endpoint inventory. Requests fan out with a small fixed concurrency bound and share cancellation; high-cardinality detail is loaded lazily when an operator opens a row.

Canonical compliance comes from State report statuses such as compliant, drifted, unsupported, check failed, deferred, apply failed, and no report. Check-in freshness is a separate presentation dimension: recent, stale, or never reported. “Offline” is not inferred from an old timestamp because the current Admin API does not provide a connection-presence signal. The default stale threshold is ten minutes and can be changed in local display preferences without altering server data.

The app auto-refreshes the visible workspace on a 30-second cadence, pauses automatic refresh while the window is hidden, and supports explicit refresh. A failed refresh retains the last successful in-memory snapshot, marks it stale with the failure time and reason, and never presents it as current. Operational snapshots are not persisted to disk in v1.

Alternative considered: derive one green/yellow/red value only from `LastDrift` and `LastApplyFailure` fields. Rejected because it conflates compliance and reachability and can misrepresent historical evidence.

### 5. The visual system is a quiet, dense operations console

The default theme is light and utilitarian: warm near-white content, cool stone navigation, charcoal typography, cobalt primary actions, and restrained mint/amber/red status tokens. IBM Plex Sans and IBM Plex Mono are bundled with the application. Layout uses a compact top connection bar, a persistent approximately 224-pixel navigation rail, an information-dense main table, and a large centered or right-aligned detail surface over a subdued backdrop. One-pixel borders, minimal elevation, tabular numbers, and deliberate whitespace hierarchy replace decorative gradients and oversized dashboard cards.

The main information architecture is Overview; Fleet management (Endpoints and Fleets); Operations (Change requests and Diagnostics); and Activity. Pages share consistent filter, column, refresh, empty, and error patterns. Endpoint details use Overview, State, Schedules, Firewall, and System tabs. Actions remain near the resource title or in an explicit action menu rather than appearing as repeated primary buttons in every row.

Keyboard operation includes predictable tab order, visible focus, Escape to close the top overlay, slash or platform search shortcut to focus filtering, and a refresh shortcut that does not fire while editing text. Status is communicated with text and iconography in addition to color. Motion is brief, disabled under reduced-motion preference, and never blocks data access. The supported application window minimum is 1100 by 720 logical pixels.

Alternative considered: reproduce the reference application's Kubernetes navigation and terminology exactly. Rejected because Remotr must use its own domain model and should not imply unsupported resource types.

### 6. Mutating actions are explicit, typed, server-authorized, and observable

Every action has a dedicated backend method, in-progress state, single-submission guard, structured success result, and classified error. The server remains authoritative for RBAC; frontend visibility or disabled state is usability guidance only. A forbidden response explains that the current Operator is not authorized without suggesting a credential workaround.

Git sync requires a normal confirmation but no typed resource name. Endpoint and Fleet upgrade requests show the exact target and version, and Fleet-wide upgrades require explicit confirmation. Endpoint removal requires the operator to type the exact Endpoint ID, and the Go backend independently checks that confirmation before issuing DELETE. Label edits use the existing label validation contract. Diagnostic requests show collectors and time bounds before submission.

Enrollment tokens are displayed once in a dedicated result panel, support an explicit native clipboard action, and are cleared when the panel closes, the profile changes, or the app exits. They never enter browser storage or logs. Successful actions invalidate and reload only the affected view plus recent activity instead of silently mutating cached rows.

Desired-state authoring controls do not exist in the first release. Later parity releases may expose the Admin CLI's local `init`, validate, discover, render, package, and Hub import behaviors inside an explicitly selected Configuration repository working tree. They do not add a direct server-side editor, auto-commit, auto-push, merge, or Apply path; Git review remains the deployment boundary.

Alternative considered: optimistic row mutation for faster perceived response. Rejected for v1 because server-accepted state and subsequent Endpoint convergence are distinct facts; the interface should show the acknowledged request and then refresh evidence.

### 7. Tests prove public behavior at the Admin API and desktop-user seams

Implementation follows one OpenSpec verification ID and one vertical red→green slice at a time. Backend tests use the real Admin client against a controlled HTTP/TLS boundary or the authenticated in-process Admin API; they do not mock Remotr-owned collaborators or inspect persistence as a side channel. Security tests use synthetic canaries to prove bootstrap tokens, issued private keys, and enrollment tokens do not appear in view models, logs, persisted profiles, or unrelated errors.

Frontend component tests render complete user workflows against an injected Wails bridge boundary and assert accessible roles, visible status text, confirmation behavior, keyboard handling, and error retention. Browser-mode end-to-end tests exercise the compiled frontend with a deterministic bridge fixture; they prove the user flow but not native packaging. The Linux artifact additionally builds and launches a packaged smoke target on a Linux runner. A bounded visual-regression viewport protects table density, overlay layout, and critical states without treating screenshots as the only behavioral assertion.

The desktop workspace becomes an approved public test seam in `docs/testing/public-seams.md`. Existing root `make test` remains required before handoff; desktop-focused tests and native build checks are additive according to risk.

### 8. Linux builds are the only desktop release products

Add `desktop-test`, `desktop-dev`, and `desktop-build` developer targets plus pinned setup documentation. Frontend package installation uses its lockfile, Go dependencies use the nested module sums, and production builds never resolve floating versions. Linux documentation calls out GTK/WebKit requirements and the required WebKit build tag for distributions that ship only the newer ABI.

CI runs Go and frontend checks on every affected pull request. A Linux runner builds and launches each advertised package; a Linux architecture or package format is not advertised until its exact native install/launch/remove smoke check passes. The unsigned DEB remains a short-lived development artifact. Tagged releases add an unsigned Linux/amd64 Flatpak bundle beside the CLI and agent archives, include that bundle in `checksums.txt`, and publish its machine-readable evidence manifest. The Flatpak pins the GNOME runtime/SDK branch, embeds the already-built Wails binary and desktop metadata, and grants only network, display, IPC, and Remotr configuration-directory access.

The change contains no macOS or Windows build metadata, jobs, installers, signing, or release assets. No signed desktop output is configured. The owner-published Flatpak is release-eligible but explicitly unsigned and is not represented as granting downstream redistribution rights; a broader signed distribution channel still requires a reviewed license and signing policy.

Alternative considered: ship macOS and Windows alongside Linux. Rejected because the current product decision is Linux-only and additional platforms would multiply WebView, packaging, signing, and smoke-test scope before the desktop workflows are mature.

### 9. Feature releases converge on behavioral Admin CLI parity

Create a machine-readable parity inventory from the current non-hidden `remotr` command tree. Each entry records the CLI capability, its desktop workflow, status (`implemented`, `planned`, or reviewed `not-applicable`), target feature release, verification IDs/selectors, and any security or interaction differences. Hidden compatibility aliases and presentation-only mechanics such as table/JSON/plain formatting, shell completion, and process exit codes do not require literal GUI equivalents. Every operator-observable behavior does.

The first desktop release establishes the secure Linux shell and the fleet visibility/action slices already specified. Subsequent feature releases close the inventory in coherent vertical groups: remaining Endpoint/Fleet reports and exports; deployment-token and Change-control lifecycles; app-package and Secret lifecycles; RBAC, Operator, and credential administration; local Configuration repository, package, and Hub workflows; and setup/doctor/docs/version/upgrade/AI tooling. The exact release numbering follows delivery evidence rather than being fixed in this design.

A desktop feature release publishes its parity inventory, cannot change an implemented entry back to planned without an approved OpenSpec change, and cannot claim full parity while any applicable CLI capability remains planned. A newly added non-hidden CLI capability must add or update its desktop parity entry in the same change. Reviewed `not-applicable` is limited to interface mechanics with no desktop-user behavior; it cannot excuse a missing security, lifecycle, data, or mutation workflow.

Parity is behavioral, not architectural. Desktop workflows use typed Admin API or shared-package boundaries, preserve or strengthen CLI confirmation/redaction rules, and never shell out to `remotr`. The Admin CLI remains supported for automation, recovery, scripting, and headless environments even after interactive feature parity is reached.

Alternative considered: freeze the desktop at the first-release fleet dashboard. Rejected because it would create a permanently second-class administration surface and force operators to switch interfaces for ordinary supported workflows.

Alternative considered: require every CLI flag and output byte to appear in the GUI. Rejected because transport and presentation mechanics are not user-capability parity and would import terminal constraints into a native interaction model.

## Risks / Trade-offs

- [A WebView increases the local attack surface] → Embed all production assets, forbid remote navigation and arbitrary backend primitives, apply a restrictive content policy, disable release devtools, and keep credential operations in Go.
- [The bootstrap token briefly exists in frontend memory] → Use a dedicated non-persistent input, submit once, clear immediately on completion/cancel, redact all errors, and verify absence with canary tests.
- [CLI and desktop presentation logic diverge] → Treat the Admin API and domain models as the shared contract; keep desktop mapping presentation-specific and avoid copying CLI output parsing.
- [A nested module can drift from the root module] → Pin both dependency graphs, use a local replacement only for the repository checkout, and run compatibility/build checks whenever shared Admin types change.
- [Fleet summary fan-out becomes slow for many Fleets] → Bound concurrency, render independent sections progressively, load per-Endpoint details lazily, measure representative fixtures, and add a bulk API only through a later contract change if evidence requires it.
- [Operators mistake cached data for live state] → Keep snapshots in memory only, show absolute and relative timestamps, mark failed refreshes stale, and never infer “online” from old check-ins.
- [Frontend authorization hints drift from server RBAC] → Keep the server authoritative, handle forbidden results explicitly, and never rely on hidden controls as a security check.
- [Linux WebView and package variants still differ] → Pin Wails and Node dependencies, test each advertised Linux architecture/package format natively, and advertise only evidenced outputs.
- [The dense UI harms readability or accessibility] → Maintain minimum hit targets and contrast, support keyboard and reduced motion, test at the minimum window size, and allow column selection without defaulting to sparse cards.
- [CLI parity creates an unbounded moving target] → Generate a versioned inventory from the non-hidden command tree, require same-change mapping for new commands, deliver coherent feature slices, and gate parity claims on zero applicable gaps.
- [Later parity actions expand into unsafe desired-state editing] → Keep every feature release allowlisted and threat-reviewed, reuse CLI safety rules, limit local Configuration tools to an operator-selected working tree, and preserve Git review as the only deployment path.

## Migration Plan

1. Register the new verification prefixes, generate the initial Admin CLI parity inventory, update domain language to recognize Remotr Desktop, and land the standalone Linux desktop build skeleton without changing existing binaries.
2. Implement profile resolution, connection verification, bootstrap, and the secure typed Wails boundary with credential-canary evidence.
3. Implement the first-release visual shell and read-only Overview, Endpoint, Fleet, Change request, and Activity slices one at a time through their public seams.
4. Add Endpoint/Fleet detail views, deterministic refresh/freshness behavior, keyboard access, and visual-regression coverage.
5. Add the first-release mutating actions as red→green slices, beginning with Git sync and ending with typed-confirmation Endpoint removal.
6. Deliver later feature releases that close the parity inventory for reports/exports, token and Change lifecycles, packages/Secrets, RBAC/Operators, local Configuration/Hub tooling, and setup/support tooling without regressing completed entries.
7. Add Linux build metadata, documentation, CI checks, and native smoke evidence; advertise only Linux architectures and package formats proven by those checks.
8. Publish the evidenced Linux/amd64 Flatpak as an additive, unsigned tagged-release asset with checksums and a release manifest. The Admin CLI remains installed and supported after parity.

Rollback is distribution-only: stop publishing or recommending the desktop artifact while retaining the Admin CLI and unchanged server API. Desktop profiles can be removed without touching Operator credential directories. No server data migration or rollback is required.

## Open Questions

- Which signing identity, redistribution license, additional architectures, and broader Linux distribution channels should follow the first unsigned Linux/amd64 Flatpak? Each added output still requires exact native evidence.
- High-risk Change request authorization is required for eventual parity, but its approval, justification, concurrency, scheduling, and two-Operator semantics still require a dedicated threat review before that feature release is implemented.
