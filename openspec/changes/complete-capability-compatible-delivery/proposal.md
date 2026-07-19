## Why

The applicator umbrella specifies current-Sync endpoint capability documents and capability-compatible artifact delivery, but the public protocol still has no implemented capability document, bounded variant selection, or `capability_blocked` delivery outcome. Provider qualification and mixed-fleet safety cannot be truthful until the server bases delivery on authenticated current endpoint evidence without silently dropping desired state.

## What Changes

- Add a versioned, bounded endpoint capability document to every modern authenticated Sync request, including schema versions, capability contract revisions, normalized provider facts, agent version metadata, and a canonical document digest.
- Validate and persist the latest document for readiness and reporting while using only the current authenticated Sync document for artifact selection.
- Compute explicit artifact requirement sets and persist only bounded canonical schema variants; never remove resources or fields to manufacture compatibility.
- Serve the highest compatible artifact variant, retain an existing endpoint's last processed artifact when the target Release is incompatible, and keep a new incompatible endpoint explicitly unmanaged.
- Add structured `capability_blocked` protocol and reporting state with global target Release, endpoint active Release, schema version, and exact missing capabilities.
- Preserve a conservative, server-maintained legacy-version profile for known pre-capability agents and fail closed for modern or unknown agents that omit valid evidence.
- Add deterministic mixed-version/mixed-capability, downgrade, reconnect, persistence, load, and malformed-document evidence for OS-AEC-016 through OS-AEC-026.
- Close umbrella task 3.11 only after the behavior and evidence in this change are accepted.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `applicator-execution-contract`: Refine provider-capability negotiation, bounded artifact variants, compatibility selection, blocked delivery, and observable Release-state requirements into a complete public protocol contract.

## Impact

- Agent fact collection and authenticated Sync request/response schemas.
- Server artifact composition/cache persistence, selection, endpoint registry state, Release advancement, and upgrade instruction behavior.
- Endpoint/fleet state reporting, Admin API/CLI output, desktop consumers, migration fixtures, and mixed-version compatibility.
- Traceability, authenticated load workloads, and provider advertisement gates that depend on current capability evidence.
- This child remains linked to the active umbrella and is not archived ahead of the umbrella capability baseline.
