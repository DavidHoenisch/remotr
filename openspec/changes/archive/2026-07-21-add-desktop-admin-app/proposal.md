## Why

Remotr's Admin CLI exposes the fleet-management primitives operators need, but it makes fleet-wide health, endpoint comparison, and investigation slower than a persistent visual workspace. A dedicated desktop application can make those workflows easier to scan and operate without adding a browser-hosted control plane or coupling desktop concerns to the existing CLI binary.

## What Changes

- Add a standalone Remotr desktop application, built with Wails, as a distinct executable and release artifact from `remotr`, `remotr-agent`, and `remotr-server`.
- Build, test, package, and support the desktop application for Linux only. macOS and Windows artifacts are outside this change.
- Reuse the existing authenticated Admin API and operator credential model directly from the Go backend; the desktop frontend does not shell out to or embed the Admin CLI.
- Add a compact operator workspace with overview, Fleet, Endpoint, Change request, and activity views; searchable tables; health/status indicators; and focused detail overlays inspired by the supplied dense infrastructure-console references.
- Support existing operator configuration and mTLS credentials, first-operator bootstrap, explicit connection errors, and RBAC-aware action availability without exposing private keys or secret values to the webview.
- Add a bounded v1 action set for Git sync, enrollment-token creation, Endpoint labels, agent upgrades, diagnostic collection, and Endpoint removal, with confirmation and transient-secret handling appropriate to each action.
- Establish a versioned feature-parity program: the first release may use the bounded action set, but subsequent desktop feature releases close the gap against every non-hidden Admin CLI capability until behavioral parity is reached and maintained.
- Preserve Git as the only desired-state deployment surface. The first release does not author Configuration content; later parity releases may match the CLI's local scaffold, validate, discover, render, package, and Hub import workflows inside an operator-selected working tree, but never bypass Git review or directly apply uncommitted content.
- Add desktop-specific tests, developer commands, packaging metadata, and release checks independently of the existing CLI build.

## Capabilities

### New Capabilities

- `desktop-operator-access`: Linux-only standalone application startup and delivery, connection profiles, bootstrap, operator mTLS credential reuse, authorization-aware behavior, secure desktop/frontend boundaries, and truthful versioned Admin CLI parity reporting.
- `desktop-fleet-visibility`: Overview, Fleet and Endpoint inventory, health summaries, search/filter behavior, detail overlays, Change request visibility, activity visibility, refresh, stale/not-reported presentation, and progressive read-only Admin CLI parity.
- `desktop-fleet-actions`: Explicit, RBAC-authorized desktop workflows for the bounded first-release action set followed by versioned expansion to behavioral parity with Admin CLI mutations, including confirmation, progress, result, and sensitive-output rules.

### Modified Capabilities

None. No archived main capability spec currently defines an administration UI, and the initial desktop release uses existing Admin API behavior without changing its contracts.

## Impact

- A new isolated Linux desktop project and executable in the monorepo, with Wails v2, a TypeScript frontend, generated Go/TypeScript bindings, Linux packaging metadata, and desktop-specific dependency/build files.
- Reuse of `internal/admin`, operator configuration, credential persistence, TLS configuration, and typed domain models through a small desktop application-service boundary.
- A machine-readable parity inventory mapping every non-hidden Admin CLI capability to its desktop equivalent, target feature release, verification evidence, or reviewed non-applicable disposition. Presentation-only CLI mechanics such as JSON/plain output modes, exit codes, and shell completion do not require literal GUI equivalents.
- Make targets and CI/release jobs for frontend checks, desktop Go tests, Wails Linux builds, and Linux artifacts; routine server, agent, and CLI builds remain unchanged.
- Updates to architecture, operator, build, testing-seam, and domain-language documentation that currently describe the Admin CLI as the only administration surface.
- No new remotely hosted service, database table, endpoint credential role, or desired-state editing path.
